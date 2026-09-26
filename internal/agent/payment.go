package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"reflect"
	"strings"
	"time"

	"Decision402/internal/probe"
	x402 "github.com/x402-foundation/x402/go/v2"
	xhttp "github.com/x402-foundation/x402/go/v2/http"
	nethttpmw "github.com/x402-foundation/x402/go/v2/http/nethttp"
	buyer "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/client"
	seller "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/server"
	signers "github.com/x402-foundation/x402/go/v2/signers/evm"
	"github.com/x402-foundation/x402/go/v2/types"
)

func noRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func Quote(ctx context.Context, s Service) (*types.PaymentRequirements, error) {
	req, _ := http.NewRequestWithContext(ctx, "GET", s.URL, nil)
	resp, err := noRedirectClient(20 * time.Second).Do(req)
	if err != nil {
		return nil, errors.New("Could not fetch the service offer")
	}
	defer resp.Body.Close()
	if resp.StatusCode != 402 {
		return nil, errors.New("Service did not return HTTP 402")
	}
	encoded := resp.Header.Get("PAYMENT-REQUIRED")
	if len(encoded) > 64000 {
		return nil, errors.New("Offer is too large")
	}
	b, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		return nil, errors.New("Offer encoding is invalid")
	}
	var offer types.PaymentRequired
	if json.Unmarshal(b, &offer) != nil || offer.X402Version != 2 || len(offer.Accepts) != 1 {
		return nil, errors.New("Offer format or number of payment options is invalid")
	}
	q := offer.Accepts[0]
	if err := CheckQuote(q, s); err != nil {
		return nil, err
	}
	return &q, nil
}

func sampleData(id string) map[string]any {
	return map[string]any{"provider": id, "city": "Tokyo", "temperature_c": 23, "sample_only": true, "description": "Static demo weather, not current weather"}
}
func SellerHandler(services []Service) http.Handler {
	routes := xhttp.RoutesConfig{}
	mux := http.NewServeMux()
	for _, s := range services {
		path := "/services/" + s.ID + "/data"
		prices := map[string]string{"A": "$0.005", "B": "$0.02", "C": "$0.01", "D": "$0.05"}
		routes["GET "+path] = xhttp.RouteConfig{Accepts: xhttp.PaymentOptions{{Scheme: "exact", Price: prices[s.ID], Network: Network, PayTo: s.PayTo}}, Description: "Static Tokyo demo weather dataset", MimeType: "application/json"}
		id := s.ID
		mux.HandleFunc("GET "+path, func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			json.NewEncoder(w).Encode(sampleData(id))
		})
	}
	return nethttpmw.X402Payment(nethttpmw.Config{Routes: routes, Facilitator: xhttp.NewHTTPFacilitatorClient(&xhttp.FacilitatorConfig{URL: "https://x402.org/facilitator"}), Schemes: []nethttpmw.SchemeConfig{{Network: x402.Network(Network), Server: seller.NewExactEvmScheme()}}, Timeout: 30 * time.Second})(mux)
}

type PaymentOutcome struct {
	Transaction string          `json:"transaction,omitempty"`
	Network     string          `json:"network"`
	Data        json.RawMessage `json:"data,omitempty"`
	Signed      bool            `json:"signed"`
	Settled     bool            `json:"settled"`
	Error       string          `json:"error,omitempty"`
}

// Pay performs one attempt. beforeSign durably reserves the task; it must fail
// closed if the journal cannot be synced. No retry/recovery hooks are installed.
func Pay(ctx context.Context, c Candidate, p Policy, keyFile string, scan interface {
	Screen(context.Context, string) probe.Decision
}, beforeSign func() error, emit func(string, any)) PaymentOutcome {
	outcome := PaymentOutcome{Network: Network}
	fail := func(msg string) PaymentOutcome { outcome.Error = msg; return outcome }
	info, err := os.Stat(keyFile)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0 {
		return fail("Test wallet file is missing or its permissions are not 600")
	}
	b, err := os.ReadFile(keyFile)
	if err != nil {
		return fail("Could not read the test wallet")
	}
	signer, err := signers.NewClientSignerFromPrivateKey(strings.TrimSpace(string(b)))
	if err != nil {
		return fail("Test wallet private key is invalid")
	}
	client := x402.Newx402Client(x402.WithSpendControls(x402.SpendControls{MaxAmountPerPayment: "$" + p.PerPayment}))
	consumed := false
	client.OnBeforePaymentCreation(func(pc x402.PaymentCreationContext) (*x402.BeforePaymentCreationHookResult, error) {
		abort := func(msg string) (*x402.BeforePaymentCreationHookResult, error) {
			return &x402.BeforePaymentCreationHookResult{Abort: true, Reason: msg}, nil
		}
		if consumed {
			return abort("This task already used its payment attempt; retry blocked")
		}
		q, ok := pc.SelectedRequirements.(types.PaymentRequirements)
		if !ok {
			return abort("Unexpected offer version")
		}
		if CheckQuote(q, c.Service) != nil || c.Quote == nil || !reflect.DeepEqual(q, *c.Quote) {
			return abort("Final offer changed; payment stopped")
		}
		d := scan.Screen(pc.Ctx, q.PayTo)
		emit("final_risk", d)
		if d.Action != "allow" || (d.Level != 0 && d.Level != 1) || d.ToxicScore == nil || *d.ToxicScore != 0 || !strings.EqualFold(d.Address, q.PayTo) || (d.Level == 0 && len(d.Traits) != 0) || (d.Level == 1 && len(d.Traits) == 0) {
			return abort("Final risk check failed")
		}
		selected := c
		selected.Level = d.Level
		selected.Quote = &q
		_, winner := Rank(p, []Candidate{selected})
		if winner == nil {
			return abort("Final offer exceeds the owner's authorization")
		}
		if pc.Ctx.Err() != nil {
			return abort("Task timed out")
		}
		if err := beforeSign(); err != nil {
			return abort("Could not durably record payment authorization; stopped")
		}
		consumed = true
		return nil, nil
	})
	client.OnAfterPaymentCreation(func(x402.PaymentCreatedContext) error { outcome.Signed = true; return nil })
	client.Register(x402.Network(Network), buyer.NewExactEvmScheme(signer, nil))
	protocol := xhttp.Newx402HTTPClient(client)
	httpClient := xhttp.WrapHTTPClientWithPayment(noRedirectClient(55*time.Second), protocol)
	req, _ := http.NewRequestWithContext(ctx, "GET", c.Service.URL, nil)
	resp, err := httpClient.Do(req)
	if err != nil {
		return fail("Payment request failed; inspect the final check. If authorization was signed, settlement is unknown and will not be retried automatically")
	}
	defer resp.Body.Close()
	// Verify settlement before reading data: response-body failure must not hide payment.
	settlement, err := protocol.GetPaymentSettleResponse(map[string]string{"PAYMENT-RESPONSE": resp.Header.Get("PAYMENT-RESPONSE")})
	if err != nil || settlement == nil || !settlement.Success || settlement.Network != Network || settlement.Transaction == "" || !outcome.Signed {
		return fail("No valid settlement receipt confirmed; no automatic retry")
	}
	outcome.Settled = true
	outcome.Transaction = settlement.Transaction
	data, err := io.ReadAll(io.LimitReader(resp.Body, 65537))
	if err != nil || len(data) > 65536 || !json.Valid(data) || resp.StatusCode != 200 {
		return fail("Payment settled, but service data could not be read; do not pay again")
	}
	outcome.Data = data
	return outcome
}
