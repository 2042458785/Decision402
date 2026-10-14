package agent

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"reflect"
	"strings"
	"time"

	"Decision402/internal/probe"
	x402 "github.com/x402-foundation/x402/go/v2"
	xhttp "github.com/x402-foundation/x402/go/v2/http"
	buyer "github.com/x402-foundation/x402/go/v2/mechanisms/evm/exact/client"
	"github.com/x402-foundation/x402/go/v2/types"
)

func noRedirectClient(timeout time.Duration) *http.Client {
	return &http.Client{Timeout: timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
}
func Quote(ctx context.Context, s Service) (*types.PaymentRequirements, error) {
	req, err := serviceRequest(ctx, s)
	if err != nil {
		return nil, errors.New("Could not construct the service request")
	}
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
	if json.Unmarshal(b, &offer) != nil || offer.X402Version != 2 || len(offer.Accepts) == 0 {
		return nil, errors.New("Offer format or number of payment options is invalid")
	}
	var match *types.PaymentRequirements
	for _, q := range offer.Accepts {
		if CheckQuote(q, s) == nil {
			if match != nil {
				return nil, errors.New("Offer has duplicate matching payment options")
			}
			selected := q
			match = &selected
		}
	}
	if match == nil {
		return nil, errors.New("No verified Base Sepolia pay-per-request USDC offer")
	}
	return match, nil
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
func Pay(ctx context.Context, c Candidate, p Policy, signer *walletSigner, scan interface {
	Screen(context.Context, string) probe.Decision
}, beforeSign func() error, emit func(string, any)) PaymentOutcome {
	outcome := PaymentOutcome{Network: Network}
	fail := func(msg string) PaymentOutcome { outcome.Error = msg; return outcome }
	if signer == nil || signer.grant == nil || !signer.grant.valid(signer.grant.session) {
		return fail(errWalletLocked.Error())
	}
	signer.beforeSign = beforeSign
	if c.Quote == nil {
		return fail("No approved quote")
	}
	client := x402.Newx402Client(
		x402.WithSpendControls(x402.SpendControls{MaxAmountPerPayment: "$" + p.PerPayment}),
		x402.WithPolicy(func(options []x402.PaymentRequirementsView) []x402.PaymentRequirementsView {
			approved := make([]x402.PaymentRequirementsView, 0, 1)
			for _, option := range options {
				q, ok := option.(types.PaymentRequirements)
				if ok && reflect.DeepEqual(q, *c.Quote) {
					approved = append(approved, option)
				}
			}
			return approved
		}),
	)
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
		consumed = true
		return nil, nil
	})
	client.OnAfterPaymentCreation(func(x402.PaymentCreatedContext) error {
		outcome.Signed = true
		emit("payment_signed", map[string]string{"service": c.Service.ID})
		return nil
	})
	client.Register(x402.Network(Network), buyer.NewExactEvmScheme(signer, nil))
	protocol := xhttp.Newx402HTTPClient(client)
	httpClient := xhttp.WrapHTTPClientWithPayment(noRedirectClient(55*time.Second), protocol)
	req, err := serviceRequest(ctx, c.Service)
	if err != nil {
		return fail("Could not construct the service request")
	}
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
	checked, err := checkedServiceData(c.Service, data)
	if err != nil {
		return fail("Payment settled, but the service data was invalid; do not pay again")
	}
	outcome.Data = checked
	return outcome
}
