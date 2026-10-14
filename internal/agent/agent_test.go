package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"Decision402/internal/probe"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/x402-foundation/x402/go/v2/types"
)

const testPayTo = "0x1111111111111111111111111111111111111111"
const testRisk = "0x2222222222222222222222222222222222222222"

func testQuote(amount string) *types.PaymentRequirements {
	return &types.PaymentRequirements{Scheme: "exact", Network: Network, Asset: Asset, Amount: amount, PayTo: testPayTo, MaxTimeoutSeconds: 300, Extra: map[string]interface{}{"name": "USDC", "version": "2"}}
}
func TestPolicyPreferencesAndLimits(t *testing.T) {
	cs := []Candidate{{Service: Service{ID: "A"}, Level: 2, Quote: testQuote("5000")}, {Service: Service{ID: "C"}, Level: 1, Quote: testQuote("10000")}, {Service: Service{ID: "D"}, Level: 0, Quote: testQuote("50000")}, {Service: Service{ID: "U"}, Level: -1, Quote: testQuote("1000")}}
	for _, tc := range []struct {
		risk               int
		pref, budget, want string
	}{{1, "price", "0.1", "C"}, {1, "risk", "0.1", "D"}, {0, "price", "0.1", "D"}, {0, "risk", "0.02", ""}, {1, "price", "0.005", ""}} {
		p := Policy{PerPayment: "0.1", TaskBudget: tc.budget, MaxRisk: tc.risk, Preference: tc.pref}
		out, winner := Rank(p, cs)
		got := ""
		if winner != nil {
			got = winner.Service.ID
		}
		if got != tc.want {
			t.Fatalf("%+v selected %s", tc, got)
		}
		if out[0].Eligible || out[3].Eligible {
			t.Fatal("high/unknown risk became eligible")
		}
	}
	for _, s := range []string{"NaN", "0.0000001", "-1", "1e-2", "999999999999999999999", "0.1 USD"} {
		if _, err := Money(s); err == nil {
			t.Fatalf("accepted %s", s)
		}
	}
	n, err := Money("0.01")
	if err != nil || n != 10000 {
		t.Fatal(n, err)
	}
}

type scannerStub struct {
	action   string
	level    int
	calls    int
	onScreen func()
}

func (s *scannerStub) Screen(_ context.Context, address string) probe.Decision {
	s.calls++
	if s.onScreen != nil {
		s.onScreen()
	}
	score := 0
	traits := []probe.Trait{}
	if s.action == "deny" {
		score = 100
		s.level = 2
	} else if s.level == 1 {
		risk := 10
		traits = []probe.Trait{{Risk: &risk, Name: "reviewed_advisory", Description: "test advisory"}}
	}
	return probe.Decision{Action: s.action, Level: s.level, Address: address, ToxicScore: &score, Traits: traits}
}
func TestPayFinalGateAndReceipt(t *testing.T) {
	for _, name := range []string{"allow", "low_allowed", "low_not_allowed", "risk", "changed_quote", "journal_error", "locked", "lock_during_risk"} {
		t.Run(name, func(t *testing.T) {
			q := testQuote("10000")
			signingRequests := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Header.Get("PAYMENT-SIGNATURE") == "" {
					offer := *q
					if name == "changed_quote" {
						offer.Amount = "10001"
					}
					b, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{offer}})
					w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(b))
					w.WriteHeader(402)
					fmt.Fprint(w, "{}")
					return
				}
				signingRequests++
				b, _ := json.Marshal(map[string]any{"success": true, "network": Network, "transaction": "0x" + strings.Repeat("a", 64)})
				w.Header().Set("PAYMENT-RESPONSE", base64.StdEncoding.EncodeToString(b))
				fmt.Fprint(w, `{"sample_only":true}`)
			}))
			defer server.Close()
			key, _ := crypto.GenerateKey()
			grant, err := newGrant(key, nil, "test", time.Now().Add(time.Minute))
			if err != nil {
				t.Fatal(err)
			}
			defer grant.lock()
			signer := &walletSigner{grant: grant, address: crypto.PubkeyToAddress(key.PublicKey).Hex()}
			wipeKey(key)
			scanner := &scannerStub{action: "allow"}
			if name == "locked" {
				grant.lock()
			}
			if name == "lock_during_risk" {
				scanner.onScreen = grant.lock
			}
			if name == "risk" {
				scanner.action = "deny"
			}
			maxRisk := 0
			if name == "low_allowed" || name == "low_not_allowed" {
				scanner.level = 1
			}
			if name == "low_allowed" {
				maxRisk = 1
			}
			reserved := 0
			c := Candidate{Service: Service{ID: "C", URL: server.URL, PayTo: testPayTo, Amount: "10000"}, Quote: q, Level: 0}
			out := Pay(context.Background(), c, Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: maxRisk, Preference: "price"}, signer, scanner, func() error {
				reserved++
				if name == "journal_error" {
					return errors.New("disk failure")
				}
				return nil
			}, func(string, any) {})
			if name == "allow" || name == "low_allowed" {
				if !out.Signed || !out.Settled || signingRequests != 1 || reserved != 1 {
					t.Fatalf("expected one test signature and mock receipt: %+v requests=%d reserve=%d", out, signingRequests, reserved)
				}
			} else {
				if (name == "locked" || name == "lock_during_risk") && reserved != 0 {
					t.Fatal("locked wallet reserved payment")
				}
				if out.Signed || out.Settled || signingRequests != 0 || out.Error == "" {
					t.Fatalf("gate bypass %+v", out)
				}
			}
		})
	}
}
func TestJournalAndRequestIsolation(t *testing.T) {
	dir := t.TempDir()
	a, err := NewApp("127.0.0.1:8080", dir, NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{})
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "test-task-00000001", Instruction: "Check Base mainnet USDC balance for 0x3333333333333333333333333333333333333333", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
	task := &Task{Request: request, Status: "running", PaymentAttempted: true}
	if a.persist(task) != nil {
		t.Fatal("persist failed")
	}
	recovered, err := NewApp(a.host, dir, a.model, a.scanner)
	if err != nil {
		t.Fatal(err)
	}
	if recovered.tasks[request.ID].Status != "held" || !recovered.tasks[request.ID].PaymentAttempted {
		t.Fatal("restart replayed/reset old authorization")
	}
	var empty struct{}
	if strictJSON([]byte(`{"budget":"9999"}`), &empty) == nil {
		t.Fatal("tool accepted override")
	}
	if strictJSON([]byte(`{} {}`), &empty) == nil {
		t.Fatal("tool accepted trailing JSON")
	}
}

func TestAgentLoopSimulationAndDuplicateExecute(t *testing.T) {
	var requests atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		turn := requests.Add(1)
		var msg any
		if turn == 1 {
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "a", "type": "function", "function": map[string]string{"name": "find_services", "arguments": `{"address":"0x3333333333333333333333333333333333333333"}`}}}}
		} else if turn == 2 {
			calls := []any{}
			for _, id := range []string{"b", "c"} {
				calls = append(calls, map[string]any{"id": id, "type": "function", "function": map[string]string{"name": "execute_purchase", "arguments": "{}"}})
			}
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": calls}
		} else {
			msg = map[string]string{"role": "assistant", "content": "模拟选择 C，未付款。"}
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": msg}}})
	}))
	defer modelServer.Close()
	scanner := &scannerStub{action: "allow"}
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", modelServer.URL), scanner)
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "test-task-00000002", Instruction: "Check Base mainnet USDC balance for 0x3333333333333333333333333333333333333333", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
	a.tasks[request.ID] = &Task{Request: request, Status: "queued"}
	a.run(request)
	task := a.tasks[request.ID]
	if task.Status != "previewed" || task.Selected.Service.ID != "quicknode" || task.PaymentAttempted || scanner.calls != 0 || !strings.Contains(task.Summary, "Policy simulation") || !strings.Contains(task.Summary, "Screening:") || requests.Load() != 2 {
		t.Fatalf("unexpected simulated execution %+v", task)
	}
	count := 0
	for _, e := range task.Events {
		if e.Kind == "selection" {
			count++
		}
	}
	if count != 1 {
		t.Fatal("repeated tool call re-executed selection")
	}
}

func TestModelErrorDoesNotExposeCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(401); fmt.Fprint(w, "test-secret") }))
	defer server.Close()
	model := NewModel("test-secret", "test", server.URL)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := model.Complete(ctx, []Message{})
	if err == nil || strings.Contains(err.Error(), "test-secret") {
		t.Fatal("credential leaked or error missing")
	}
}

func TestDuplicateTaskAndChangedAuthorization(t *testing.T) {
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{})
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "test-task-00000003", Instruction: "Check Base mainnet USDC balance for 0x3333333333333333333333333333333333333333", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
	a.tasks[request.ID] = &Task{Request: request, Status: "previewed", Summary: "existing result"}
	for _, change := range []bool{false, true} {
		input := request
		if change {
			input.Policy.MaxRisk = 0
		}
		b, _ := json.Marshal(input)
		r := httptest.NewRequest("POST", "http://127.0.0.1:8080/api/tasks", strings.NewReader(string(b)))
		w := httptest.NewRecorder()
		a.createTask(w, r)
		want := 200
		if change {
			want = 409
		}
		if w.Code != want {
			t.Fatalf("changed=%v status=%d", change, w.Code)
		}
	}
	if len(a.tasks) != 1 || a.tasks[request.ID].Status != "previewed" {
		t.Fatal("duplicate task was executed again")
	}
}

func TestQuickNodeIsOnlyConfiguredService(t *testing.T) {
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.services) != 1 || a.services[0].ID != "quicknode" || a.services[0].URL != quickNodeURL || a.services[0].Amount != quickNodePrice {
		t.Fatalf("unexpected services: %+v", a.services)
	}
}

func TestUnsupportedTaskIsRejectedBeforeRunning(t *testing.T) {
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{})
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "unsupported-task-001", Instruction: "Check this address", Mode: "preview", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 0, Preference: "price"}}
	data, _ := json.Marshal(request)
	w := httptest.NewRecorder()
	a.createTask(w, httptest.NewRequest("POST", "/api/tasks", strings.NewReader(string(data))))
	if w.Code != 400 || len(a.tasks) != 0 {
		t.Fatalf("unsupported task should be rejected without starting: status=%d tasks=%d", w.Code, len(a.tasks))
	}
}

type fixtureScanner struct {
	decisions map[string]probe.Decision
}

func (s *fixtureScanner) Screen(_ context.Context, address string) probe.Decision {
	return s.decisions[address]
}

func TestLivePreviewReadsPOSTQuoteAndScreensRecipient(t *testing.T) {
	zero := 0
	scanner := &fixtureScanner{decisions: map[string]probe.Decision{
		quickNodePayTo: {Action: "allow", Level: 0, Address: quickNodePayTo, ToxicScore: &zero},
	}}
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), scanner)
	if err != nil {
		t.Fatal(err)
	}
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Errorf("wrong request: %s %s", r.Method, r.Header.Get("Content-Type"))
		}
		var body struct {
			Method string            `json:"method"`
			Params []json.RawMessage `json:"params"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil || body.Method != "eth_call" || len(body.Params) != 2 {
			t.Errorf("wrong JSON-RPC body: %+v", body)
		}
		large := *testQuote("1000000")
		large.PayTo = quickNodePayTo
		exact := *testQuote(quickNodePrice)
		exact.PayTo = quickNodePayTo
		gateway := *testQuote("100")
		gateway.PayTo = quickNodePayTo
		gateway.Extra = map[string]interface{}{"name": "GatewayWalletBatched", "version": "1"}
		b, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{large, exact, gateway}})
		w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(b))
		w.WriteHeader(402)
	}))
	defer server.Close()
	a.services[0].URL = server.URL
	id := "test-live-preview-0001"
	a.tasks[id] = &Task{Request: TaskRequest{ID: id}, Status: "running"}
	policy := Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 0, Preference: "price"}
	candidates, selected := a.discover(context.Background(), id, "preview", policy, "0x3333333333333333333333333333333333333333")
	if selected == nil || selected.Service.ID != "quicknode" || selected.Quote.Amount != quickNodePrice || len(candidates) != 1 || candidates[0].Level != 0 || requests.Load() != 1 {
		t.Fatalf("unexpected preview: selected=%+v candidates=%+v requests=%d", selected, candidates, requests.Load())
	}
}

func TestModelCannotSubstituteWalletAddress(t *testing.T) {
	var turns atomic.Int32
	modelServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if turns.Add(1) == 1 {
			json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "tool_calls": []any{map[string]any{"id": "a", "type": "function", "function": map[string]string{"name": "find_services", "arguments": `{"address":"0x4444444444444444444444444444444444444444"}`}}}}}}})
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]any{"role": "assistant", "content": "Unsupported address."}}}})
	}))
	defer modelServer.Close()
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", modelServer.URL), &scannerStub{action: "allow"})
	if err != nil {
		t.Fatal(err)
	}
	req := TaskRequest{ID: "model-address-test-01", Instruction: "Check Base mainnet USDC balance for 0x3333333333333333333333333333333333333333", Mode: "simulate", Policy: Policy{PerPayment: "0.01", TaskBudget: "0.01", MaxRisk: 0, Preference: "price"}}
	a.tasks[req.ID] = &Task{Request: req, Status: "queued"}
	a.run(req)
	result := a.tasks[req.ID]
	if len(result.Candidates) != 0 || result.PaymentAttempted || result.Selected != nil {
		t.Fatalf("model changed target: %+v", result)
	}
}
