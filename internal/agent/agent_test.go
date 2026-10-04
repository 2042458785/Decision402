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
	a, err := NewApp("127.0.0.1:8080", dir, NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{}, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "test-task-00000001", Instruction: "Tokyo weather", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
	task := &Task{Request: request, Status: "running", PaymentAttempted: true}
	if a.persist(task) != nil {
		t.Fatal("persist failed")
	}
	recovered, err := NewApp(a.host, dir, a.model, a.scanner, testPayTo, testRisk)
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
			msg = map[string]any{"role": "assistant", "content": "", "tool_calls": []any{map[string]any{"id": "a", "type": "function", "function": map[string]string{"name": "find_services", "arguments": `{"city":"Tokyo"}`}}}}
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
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", modelServer.URL), scanner, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "test-task-00000002", Instruction: "Tokyo weather", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
	a.tasks[request.ID] = &Task{Request: request, Status: "queued"}
	a.run(request)
	task := a.tasks[request.ID]
	if task.Status != "previewed" || task.Selected.Service.ID != "C" || task.PaymentAttempted || scanner.calls != 0 || !strings.Contains(task.Summary, "Policy simulation") || !strings.Contains(task.Summary, "Screening:") || requests.Load() != 2 {
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
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{}, testPayTo, testRisk)
	if err != nil {
		t.Fatal(err)
	}
	request := TaskRequest{ID: "test-task-00000003", Instruction: "Tokyo weather", Mode: "simulate", Policy: Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: "price"}}
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

func TestOptionalLowRiskRecipientIsDistinct(t *testing.T) {
	low := "0x3333333333333333333333333333333333333333"
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{}, testPayTo, testRisk, low)
	if err != nil {
		t.Fatal(err)
	}
	if a.services[2].PayTo != low || a.services[3].PayTo != testPayTo {
		t.Fatalf("C/D recipients not separated: %+v", a.services)
	}
	if _, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), &scannerStub{}, testPayTo, testRisk, testRisk); err == nil {
		t.Fatal("reused risk fixture must fail")
	}
}

type fixtureScanner struct {
	decisions map[string]probe.Decision
}

func (s *fixtureScanner) Screen(_ context.Context, address string) probe.Decision {
	return s.decisions[address]
}

func TestLivePreviewRanksScreenedRecipients(t *testing.T) {
	low := "0x3333333333333333333333333333333333333333"
	zero, ten, hundred := 0, 10, 100
	scanner := &fixtureScanner{decisions: map[string]probe.Decision{
		testRisk:  {Action: "deny", Level: 2, Address: testRisk, ToxicScore: &hundred, Reason: "known malicious"},
		low:       {Action: "allow", Level: 1, Address: low, ToxicScore: &zero, Traits: []probe.Trait{{Risk: &ten, Name: "reviewed_advisory", Description: "minor exposure"}}},
		testPayTo: {Action: "allow", Level: 0, Address: testPayTo, ToxicScore: &zero},
	}}
	a, err := NewApp("127.0.0.1:8080", t.TempDir(), NewModel("test", "test", "https://api.deepseek.com"), scanner, testPayTo, testRisk, low)
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]Service{}
	for _, service := range a.services {
		byID[service.ID] = service
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := strings.TrimPrefix(strings.TrimSuffix(r.URL.Path, "/data"), "/services/")
		service, ok := byID[id]
		if !ok {
			w.WriteHeader(404)
			return
		}
		q := *testQuote(service.Amount)
		q.PayTo = service.PayTo
		body, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{q}})
		w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(body))
		w.WriteHeader(402)
	}))
	defer server.Close()
	for i := range a.services {
		a.services[i].URL = server.URL + "/services/" + a.services[i].ID + "/data"
	}
	id := "test-live-preview-0001"
	a.tasks[id] = &Task{Request: TaskRequest{ID: id}, Status: "running"}
	for _, tc := range []struct{ preference, want string }{{"price", "C"}, {"risk", "D"}} {
		policy := Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 1, Preference: tc.preference}
		candidates, selected := a.discover(context.Background(), id, "preview", policy)
		if selected == nil || selected.Service.ID != tc.want || candidates[0].Level != 2 || candidates[1].Level != 2 || candidates[2].Level != 1 || candidates[3].Level != 0 {
			t.Fatalf("preference=%s selected=%+v candidates=%+v", tc.preference, selected, candidates)
		}
	}
}
