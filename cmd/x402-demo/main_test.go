package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	x402 "github.com/x402-foundation/x402/go/v2"
)

const demoPayTo = "0x1111111111111111111111111111111111111111"

type fakeRequirements struct {
	scheme, network, asset, amount, payTo string
	extra                                 map[string]interface{}
}

func (r fakeRequirements) GetScheme() string                { return r.scheme }
func (r fakeRequirements) GetNetwork() string               { return r.network }
func (r fakeRequirements) GetAsset() string                 { return r.asset }
func (r fakeRequirements) GetAmount() string                { return r.amount }
func (r fakeRequirements) GetPayTo() string                 { return r.payTo }
func (r fakeRequirements) GetMaxTimeoutSeconds() int        { return 60 }
func (r fakeRequirements) GetExtra() map[string]interface{} { return r.extra }

func TestOfferGuardBeforeSigning(t *testing.T) {
	base := fakeRequirements{scheme: "exact", network: baseSepolia, asset: baseSepoliaUSDC, amount: "1000", payTo: demoPayTo}
	if err := checkOffer(base, demoPayTo); err != nil {
		t.Fatalf("valid testnet quote rejected: %v", err)
	}
	checks := []struct {
		name string
		edit func(*fakeRequirements)
	}{
		{"changed recipient", func(r *fakeRequirements) { r.payTo = "0x2222222222222222222222222222222222222222" }},
		{"fake token", func(r *fakeRequirements) { r.asset = "0x2222222222222222222222222222222222222222" }},
		{"mainnet", func(r *fakeRequirements) { r.network = "eip155:8453" }},
		{"over budget", func(r *fakeRequirements) { r.amount = "1001" }},
		{"zero amount", func(r *fakeRequirements) { r.amount = "0" }},
		{"unparseable amount", func(r *fakeRequirements) { r.amount = "1.5" }},
		{"different scheme", func(r *fakeRequirements) { r.scheme = "upto" }},
		{"permit2 method", func(r *fakeRequirements) { r.extra = map[string]interface{}{"assetTransferMethod": "permit2"} }},
	}
	for _, tc := range checks {
		t.Run(tc.name, func(t *testing.T) {
			r := base
			tc.edit(&r)
			if err := checkOffer(r, demoPayTo); err == nil {
				t.Fatal("changed offer would reach signing")
			}
		})
	}
	var absent x402.PaymentRequirementsView
	if err := checkOffer(absent, demoPayTo); err == nil {
		t.Fatal("missing offer must be rejected")
	}
}

func TestTestWalletKeyFilePermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wallet.key")
	if _, err := readTestKey(path); err == nil {
		t.Fatal("missing key must fail")
	}
	if err := os.WriteFile(path, []byte("test-only-key\n"), 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := readTestKey(path); err == nil {
		t.Fatal("world-readable key must fail")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	key, err := readTestKey(path)
	if err != nil || key != "test-only-key" {
		t.Fatalf("private key file not read correctly: %v", err)
	}
}

func TestDemoURLAndSellerRequireConfiguration(t *testing.T) {
	for _, raw := range []string{"https://example.com/data", "http://example.com/data", "http://127.0.0.1:4021/other", "http://127.0.0.1:4021/data?x=1"} {
		if err := request(raw, "", ""); err == nil || !strings.Contains(err.Error(), "local") {
			t.Fatalf("outside demo scope: %q err=%v", raw, err)
		}
	}
	if err := serve("127.0.0.1:4021", ""); err == nil {
		t.Fatal("seller missing receiving address")
	}
}
