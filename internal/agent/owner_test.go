package agent

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/x402-foundation/x402/go/v2/types"

	"github.com/ethereum/go-ethereum/accounts/keystore"
	"github.com/ethereum/go-ethereum/crypto"
)

func TestOwnerSignInAgentVaultAndTaskBoundary(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "tasks")
	a, err := NewApp("127.0.0.1:8080", dir, NewModel("server-key", "deepseek-flash", "https://api.deepseek.com"), &scannerStub{action: "allow"}, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	a.scryptN, a.scryptP = keystore.LightScryptN, keystore.LightScryptP
	handler := a.Handler(t.TempDir())
	call := func(method, path, body string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "http://127.0.0.1:8080"+path, strings.NewReader(body))
		if method == "POST" {
			r.Header.Set("X-Decision402", "local-ui")
			r.Header.Set("Content-Type", "application/json")
		}
		if cookie != nil {
			r.AddCookie(cookie)
		}
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)
		return w
	}
	key, err := crypto.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	owner := crypto.PubkeyToAddress(key.PublicKey).Hex()
	challengeResponse := call("GET", "/api/auth/challenge?address="+owner, "", nil)
	if challengeResponse.Code != 200 {
		t.Fatal(challengeResponse.Body.String())
	}
	var challenge struct {
		Nonce   string `json:"nonce"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(challengeResponse.Body.Bytes(), &challenge); err != nil {
		t.Fatal(err)
	}
	messageHash := crypto.Keccak256([]byte("\x19Ethereum Signed Message:\n" + strconv.Itoa(len(challenge.Message)) + challenge.Message))
	sig, err := crypto.Sign(messageHash, key)
	if err != nil {
		t.Fatal(err)
	}
	sig[64] += 27
	login, _ := json.Marshal(map[string]string{"address": owner, "nonce": challenge.Nonce, "signature": "0x" + hex.EncodeToString(sig)})
	if bad := call("POST", "/api/auth/session", string(login), nil); bad.Code != 200 {
		t.Fatalf("login: %d %s", bad.Code, bad.Body.String())
	}
	if reused := call("POST", "/api/auth/session", string(login), nil); reused.Code != 401 {
		t.Fatal("reused challenge was accepted")
	}
	loginReply := call("POST", "/api/auth/session", `{"address":"wrong"}`, nil)
	if loginReply.Code == 200 {
		t.Fatal("invalid login was accepted")
	}
	// The original successful response above is needed for its HttpOnly session cookie.
	// Re-authenticate because the challenge is intentionally one-use.
	challengeResponse = call("GET", "/api/auth/challenge?address="+owner, "", nil)
	json.Unmarshal(challengeResponse.Body.Bytes(), &challenge)
	messageHash = crypto.Keccak256([]byte("\x19Ethereum Signed Message:\n" + strconv.Itoa(len(challenge.Message)) + challenge.Message))
	sig, _ = crypto.Sign(messageHash, key)
	sig[64] += 27
	login, _ = json.Marshal(map[string]string{"address": owner, "nonce": challenge.Nonce, "signature": "0x" + hex.EncodeToString(sig)})
	sessionReply := call("POST", "/api/auth/session", string(login), nil)
	if sessionReply.Code != 200 || len(sessionReply.Result().Cookies()) == 0 {
		t.Fatal("missing session cookie")
	}
	cookie := sessionReply.Result().Cookies()[0]
	if !cookie.HttpOnly || cookie.SameSite != http.SameSiteStrictMode {
		t.Fatal("unsafe session cookie")
	}
	createBody := `{"wallet_kind":"legacy","name":"Tokyo buyer","model_url":"https://api.deepseek.com","model_name":"deepseek-flash","model_api_key":"agent-model-secret","password":"test-wallet-password"}`
	if got := call("POST", "/api/agents", createBody, nil); got.Code != 401 {
		t.Fatal("agent creation without owner session")
	}
	created := call("POST", "/api/agents", createBody, cookie)
	if created.Code != 201 || strings.Contains(created.Body.String(), "agent-model-secret") {
		t.Fatalf("agent response leaked key or failed: %s", created.Body.String())
	}
	var view agentView
	if err := json.Unmarshal(created.Body.Bytes(), &view); err != nil || view.Owner != owner || !addressPattern.MatchString(view.Wallet) {
		t.Fatal("invalid agent view")
	}
	for _, ext := range []string{".json"} {
		file := filepath.Join(filepath.Dir(dir), "agents", view.ID+ext)
		info, err := os.Stat(file)
		if err != nil || info.Mode().Perm() != 0600 {
			t.Fatalf("vault file %s not 0600: %v", ext, err)
		}
	}
	if noCookie := call("GET", "/api/agents", "", nil); noCookie.Code != 401 {
		t.Fatal("agent list accessible without owner")
	}
	list := call("GET", "/api/agents", "", cookie)
	if list.Code != 200 || strings.Contains(list.Body.String(), "agent-model-secret") {
		t.Fatal("agent list leaked key")
	}
	reloaded, err := NewApp(a.host, dir, a.model, a.scanner, testPayTo, testRisk)
	if err != nil || reloaded.agents[view.ID].Wallet != view.Wallet {
		t.Fatalf("wallet did not survive restart: %v", err)
	}
	request := TaskRequest{ID: "test-agent-task-0001", AgentID: view.ID, Instruction: "Tokyo weather", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
	requestJSON, _ := json.Marshal(request)
	if unauthorized := call("POST", "/api/tasks", string(requestJSON), nil); unauthorized.Code != 403 {
		t.Fatal("task accepted without wallet session")
	}
	a.tasks[request.ID] = &Task{Request: request, Status: "previewed", Events: []Event{}, Candidates: []Candidate{}}
	if got := call("GET", "/api/tasks/"+request.ID, "", nil); got.Code != 403 {
		t.Fatal("agent task visible without wallet session")
	}
	if got := call("GET", "/api/tasks/"+request.ID, "", cookie); got.Code != 200 {
		t.Fatal("agent task hidden from owner")
	}
	// Exercise the new wallet through the real x402 client against a local
	// mock seller. No onchain transaction or external model call occurs here.
	var turns atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		name := "find_services"
		args := `{"city":"Tokyo"}`
		if turns.Add(1) > 1 {
			name, args = "execute_purchase", `{}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "tool-1", "type": "function", "function": map[string]string{"name": name, "arguments": args}}}}}}})
	}))
	defer modelServer.Close()
	var paid atomic.Int32
	services := map[string]Service{}
	for _, s := range a.services {
		services[s.ID] = s
	}
	sellerServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/data"), "/services/")
		s, ok := services[id]
		if !ok {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("PAYMENT-SIGNATURE") == "" {
			q := *testQuote(s.Amount)
			q.PayTo = s.PayTo
			b, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{q}})
			w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(b))
			w.WriteHeader(402)
			return
		}
		payload, err := base64.StdEncoding.DecodeString(r.Header.Get("PAYMENT-SIGNATURE"))
		if err != nil || !strings.Contains(strings.ToLower(string(payload)), strings.ToLower(view.Wallet)) {
			t.Errorf("payment did not use generated agent wallet")
		}
		paid.Add(1)
		b, _ := json.Marshal(map[string]any{"success": true, "network": Network, "transaction": "0x" + strings.Repeat("a", 64)})
		w.Header().Set("PAYMENT-RESPONSE", base64.StdEncoding.EncodeToString(b))
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"sample_only":true}`))
	}))
	defer sellerServer.Close()
	for i := range a.services {
		a.services[i].URL = sellerServer.URL + "/services/" + a.services[i].ID + "/data"
	}
	configured := a.agents[view.ID]
	configured.ModelURL = modelServer.URL
	a.agents[view.ID] = configured
	payRequest := TaskRequest{ID: "test-agent-payment-01", AgentID: view.ID, Instruction: "Tokyo weather", Mode: "pay", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 0, Preference: "price"}}
	a.tasks[payRequest.ID] = &Task{Request: payRequest, Status: "queued", Events: []Event{}, Candidates: []Candidate{}}
	unlocked := call("POST", "/api/agents/"+view.ID+"/unlock", `{"password":"test-wallet-password"}`, cookie)
	if unlocked.Code != 200 {
		t.Fatalf("unlock failed: %s", unlocked.Body.String())
	}
	a.run(payRequest, a.grants[view.ID])
	a.lockWallet(view.ID)
	result := a.tasks[payRequest.ID]
	if result.Status != "settled" || result.Payment == nil || !result.Payment.Settled || paid.Load() != 1 {
		t.Fatalf("agent payment did not settle with generated wallet: status=%s paid=%d summary=%s error=%s payment=%+v", result.Status, paid.Load(), result.Summary, result.Error, result.Payment)
	}
}
