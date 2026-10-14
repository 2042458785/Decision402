package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/crypto"
	"github.com/x402-foundation/x402/go/v2/types"
)

const target = "0x3333333333333333333333333333333333333333"

func TestRequestedAddress(t *testing.T) {
	for _, input := range []string{"weather in Tokyo", "Base USDC balance 0x123", "Base USDC balance " + target + " and 0x4444444444444444444444444444444444444444", "Base USDC balance " + target + "f"} {
		if _, err := requestedAddress(input); err == nil {
			t.Errorf("accepted %q", input)
		}
	}
	got, err := requestedAddress("Check Base mainnet USDC balance for " + target)
	if err != nil || got != target {
		t.Fatalf("got %q: %v", got, err)
	}
	got, err = requestedAddress("请通过 QuickNode 查询 Base 主网钱包的 USDC 余额，钱包地址：" + target)
	if err != nil || got != target {
		t.Fatalf("Chinese task got %q: %v", got, err)
	}
}

func TestQuickNodeRequestAndBalance(t *testing.T) {
	s := quickNodeService(target)
	req, err := serviceRequest(context.Background(), s)
	if err != nil || req.Method != http.MethodPost || req.URL.String() != quickNodeURL {
		t.Fatalf("request: %+v, %v", req, err)
	}
	var body struct {
		Method string            `json:"method"`
		Params []json.RawMessage `json:"params"`
	}
	if json.NewDecoder(req.Body).Decode(&body) != nil || body.Method != "eth_call" || len(body.Params) != 2 || !strings.Contains(string(body.Params[0]), baseMainnetUSDC) || !strings.Contains(string(body.Params[0]), strings.ToLower(target[2:])) {
		t.Fatalf("wrong eth_call body: %+v", body)
	}
	good := []byte(fmt.Sprintf(`{"jsonrpc":"2.0","id":1,"result":"0x%064x"}`, 1234567))
	checked, err := checkedServiceData(s, good)
	if err != nil || !strings.Contains(string(checked), `"balance_usdc":"1.234567"`) {
		t.Fatalf("result %s: %v", checked, err)
	}
	for _, bad := range []string{`{"jsonrpc":"2.0","id":1,"error":{"code":-1}}`, `{"jsonrpc":"2.0","id":1,"result":"0xzz"}`, `{"jsonrpc":"2.0","id":2,"result":"0x0"}`} {
		if _, err := checkedServiceData(s, []byte(bad)); err == nil {
			t.Errorf("accepted bad RPC response %s", bad)
		}
	}
}

func TestQuickNodePaymentUsesApprovedOptionAndReplaysPOST(t *testing.T) {
	s := quickNodeService(target)
	exact := types.PaymentRequirements{Scheme: "exact", Network: Network, Asset: Asset, Amount: quickNodePrice, PayTo: quickNodePayTo, MaxTimeoutSeconds: 60, Extra: map[string]interface{}{"name": "USDC", "version": "2"}}
	credit := exact
	credit.Amount = "1000000"
	gateway := exact
	gateway.Amount = "100"
	gateway.Extra = map[string]interface{}{"name": "GatewayWalletBatched", "version": "1"}
	var original string
	var paid int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		data, _ := io.ReadAll(r.Body)
		if original == "" {
			original = string(data)
		} else if original != string(data) {
			t.Error("paid retry changed the JSON-RPC request")
		}
		if r.Header.Get("PAYMENT-SIGNATURE") == "" {
			offer, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{credit, exact, gateway}})
			w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(offer))
			w.WriteHeader(402)
			return
		}
		paid++
		receipt, _ := json.Marshal(map[string]any{"success": true, "network": Network, "transaction": "0x" + strings.Repeat("a", 64)})
		w.Header().Set("PAYMENT-RESPONSE", base64.StdEncoding.EncodeToString(receipt))
		fmt.Fprintf(w, `{"jsonrpc":"2.0","id":1,"result":"0x%064x"}`, 1234567)
	}))
	defer server.Close()
	s.URL = server.URL
	quote, err := Quote(context.Background(), s)
	if err != nil || quote.Amount != quickNodePrice {
		t.Fatalf("wrong quote %+v: %v", quote, err)
	}
	key, _ := crypto.GenerateKey()
	grant, err := newGrant(key, nil, "test", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer grant.lock()
	signer := &walletSigner{grant: grant, address: crypto.PubkeyToAddress(key.PublicKey).Hex()}
	wipeKey(key)
	out := Pay(context.Background(), Candidate{Service: s, Quote: quote, Level: 0}, Policy{PerPayment: "0.01", TaskBudget: "0.01", MaxRisk: 0, Preference: "price"}, signer, &scannerStub{action: "allow"}, func() error { return nil }, func(string, any) {})
	if !out.Signed || !out.Settled || out.Error != "" || paid != 1 || !strings.Contains(string(out.Data), `"balance_usdc":"1.234567"`) {
		t.Fatalf("payment failed or wrong data: %+v paid=%d", out, paid)
	}
}

func TestPaidRPCErrorDoesNotClaimBalance(t *testing.T) {
	s := quickNodeService(target)
	q := types.PaymentRequirements{Scheme: "exact", Network: Network, Asset: Asset, Amount: quickNodePrice, PayTo: quickNodePayTo, MaxTimeoutSeconds: 60, Extra: map[string]interface{}{"name": "USDC", "version": "2"}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("PAYMENT-SIGNATURE") == "" {
			offer, _ := json.Marshal(types.PaymentRequired{X402Version: 2, Accepts: []types.PaymentRequirements{q}})
			w.Header().Set("PAYMENT-REQUIRED", base64.StdEncoding.EncodeToString(offer))
			w.WriteHeader(402)
			return
		}
		receipt, _ := json.Marshal(map[string]any{"success": true, "network": Network, "transaction": "0x" + strings.Repeat("a", 64)})
		w.Header().Set("PAYMENT-RESPONSE", base64.StdEncoding.EncodeToString(receipt))
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"error":{"code":-32000,"message":"unavailable"}}`)
	}))
	defer server.Close()
	s.URL = server.URL
	key, _ := crypto.GenerateKey()
	grant, err := newGrant(key, nil, "test", time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	defer grant.lock()
	signer := &walletSigner{grant: grant, address: crypto.PubkeyToAddress(key.PublicKey).Hex()}
	wipeKey(key)
	out := Pay(context.Background(), Candidate{Service: s, Quote: &q, Level: 0}, Policy{PerPayment: "0.01", TaskBudget: "0.01", MaxRisk: 0, Preference: "price"}, signer, &scannerStub{action: "allow"}, func() error { return nil }, func(string, any) {})
	if !out.Settled || out.Error == "" || len(out.Data) != 0 {
		t.Fatalf("RPC error misreported as balance: %+v", out)
	}
}
