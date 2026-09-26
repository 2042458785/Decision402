package main

import (
	"context"
	"os"
	"testing"
	"time"

	"Decision402/internal/probe"
	x402 "github.com/x402-foundation/x402/go/v2"
	"github.com/x402-foundation/x402/go/v2/types"
)

type stubScanner struct {
	action, scannedAddress, resultAddress string
	calls                                 int
}

func (s *stubScanner) Screen(_ context.Context, address string) probe.Decision {
	s.calls++
	s.scannedAddress = address
	returned := address
	if s.resultAddress != "" {
		returned = s.resultAddress
	}
	return probe.Decision{Action: s.action, Address: returned, Reason: "test decision"}
}

// A spy replaces the payment scheme: tests never read a wallet key or sign/pay.
type payloadSpy struct{ calls int }

func (s *payloadSpy) Scheme() string { return "exact" }
func (s *payloadSpy) CreatePaymentPayload(context.Context, types.PaymentRequirements, x402.PaymentPayloadContext) (types.PaymentPayload, error) {
	s.calls++
	return types.PaymentPayload{X402Version: 2, Payload: map[string]interface{}{}}, nil
}

func exerciseGate(ctx context.Context, scanner riskScanner, payTo, expected, amount string) (int, error) {
	spy := &payloadSpy{}
	client := x402.Newx402Client()
	client.Register(x402.Network(baseSepolia), spy)
	client.OnBeforePaymentCreation(paymentGate(scanner, expected))
	_, err := client.CreatePaymentPayload(ctx, types.PaymentRequirements{
		Scheme: "exact", Network: baseSepolia, Asset: baseSepoliaUSDC, Amount: amount, PayTo: payTo, MaxTimeoutSeconds: 300,
	}, nil, nil)
	return spy.calls, err
}

func TestSDKGatePreventsPayloadCreation(t *testing.T) {
	for _, action := range []string{"allow", "deny", "hold", "unknown", ""} {
		t.Run(action, func(t *testing.T) {
			scanner := &stubScanner{action: action}
			calls, err := exerciseGate(context.Background(), scanner, demoPayTo, demoPayTo, "1000")
			if scanner.calls != 1 || scanner.scannedAddress != demoPayTo {
				t.Fatal("must scan exact quote payTo")
			}
			if action == "allow" {
				if calls != 1 || err != nil {
					t.Fatalf("allow calls=%d err=%v", calls, err)
				}
			} else if calls != 0 || err == nil {
				t.Fatalf("unsafe payload creation: calls=%d err=%v", calls, err)
			}
		})
	}
	scanner := &stubScanner{action: "allow"}
	calls, err := exerciseGate(context.Background(), scanner, demoPayTo, demoPayTo, "1001")
	if calls != 0 || scanner.calls != 0 || err == nil {
		t.Fatal("over-budget quote reached scan or payload creation")
	}
	scanner.resultAddress = "0x2222222222222222222222222222222222222222"
	calls, err = exerciseGate(context.Background(), scanner, demoPayTo, demoPayTo, "1000")
	if calls != 0 || err == nil {
		t.Fatal("mismatched evidence permitted signing")
	}
	scanner.resultAddress = ""
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls, err = exerciseGate(ctx, scanner, demoPayTo, demoPayTo, "1000")
	if calls != 0 || err == nil {
		t.Fatal("canceled request permitted signing")
	}
}

// Opt-in live API test. Uses two real scans but a spy payment scheme, so there
// is no wallet access, signature, facilitator request, or token transfer.
type recordingScanner struct {
	riskScanner
	last probe.Decision
}

func (s *recordingScanner) Screen(ctx context.Context, address string) probe.Decision {
	s.last = s.riskScanner.Screen(ctx, address)
	return s.last
}

func TestLiveInterceptaGate(t *testing.T) {
	if os.Getenv("DECISION402_LIVE_TEST") != "1" {
		t.Skip("set DECISION402_LIVE_TEST=1 to consume two API calls without paying")
	}
	cfg, err := probe.LoadConfig("../../.env")
	if err != nil {
		t.Fatal(err)
	}
	scanner := &recordingScanner{riskScanner: probe.NewClient(cfg.APIKey, 15*time.Second)}
	for _, tc := range []struct {
		name, address string
		allow         bool
	}{
		{"normal", cfg.NormalAddress, true}, {"risk", cfg.RiskAddress, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls, err := exerciseGate(context.Background(), scanner, tc.address, tc.address, "1000")
			if tc.allow && (err != nil || calls != 1) {
				t.Fatalf("expected allow: calls=%d err=%v", calls, err)
			}
			if !tc.allow && (err == nil || calls != 0) {
				t.Fatalf("expected block: calls=%d err=%v", calls, err)
			}
			expectedAction := "deny"
			if tc.allow {
				expectedAction = "allow"
			}
			if scanner.last.Action != expectedAction {
				t.Fatalf("expected %s, got %+v", expectedAction, scanner.last)
			}
			t.Logf("payload_spy_calls=%d; real signatures=0; payments=0", calls)
		})
	}
}
