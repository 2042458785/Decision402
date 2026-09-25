package probe

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const normalFixture = "0x1111111111111111111111111111111111111111"
const riskFixture = "0x2222222222222222222222222222222222222222"

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// Only tests rewrite requests to a local server. Production has a fixed HTTPS
// origin. These fixtures test transport/report behavior, not Intercepta verdicts.
func localClient(t *testing.T, handler http.HandlerFunc, timeout time.Duration) *Client {
	t.Helper()
	s := httptest.NewServer(handler)
	t.Cleanup(s.Close)
	u, _ := url.Parse(s.URL)
	c := NewClient("test-only-key", timeout)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	c.http.Transport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Host != "api.web3antivirus.io" || req.Header.Get("X-API-KEY") != "test-only-key" {
			t.Error("wrong official origin or authentication header")
		}
		clone := req.Clone(req.Context())
		clone.URL.Scheme, clone.URL.Host = u.Scheme, u.Host
		return transport.RoundTrip(clone)
	})
	return c
}

func TestCollectBothResponsesWithoutInventingVerdicts(t *testing.T) {
	calls := 0
	c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		if r.Method != "GET" || !strings.HasSuffix(r.URL.Path, "/quick-scan") || r.Header.Get("Accept") != "application/json" {
			t.Error("incorrect request shape")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, normalFixture) {
			io.WriteString(w, `{"test_signal":false,"large":9007199254740993,"empty":[],"a/b":null}`)
		} else {
			io.WriteString(w, `{"test_signal":true,"test_reasons":["synthetic test fixture"]}`)
		}
	}, time.Second)
	dir, err := Run(context.Background(), Config{NormalAddress: normalFixture, RiskAddress: riskFixture, AddressSource: "unit test only"}, c, t.TempDir(), io.Discard)
	if err != nil {
		t.Fatal(err)
	}
	body, err := os.ReadFile(filepath.Join(dir, "report.json"))
	if err != nil {
		t.Fatal(err)
	}
	var report Report
	if err := json.Unmarshal(body, &report); err != nil {
		t.Fatal(err)
	}
	if calls != 2 || len(report.Results) != 2 || !report.CollectionComplete || report.VerdictsConfirmed {
		t.Fatalf("incorrect completion evidence: calls=%d report=%+v", calls, report)
	}
	for _, r := range report.Results {
		if r.RiskInterpretation != "unreviewed" || r.DurationMS <= 0 || !r.HTTPOK {
			t.Fatalf("bad result: %+v", r)
		}
		stored, err := os.ReadFile(filepath.Join(dir, r.ResponseFile))
		if err != nil || !json.Valid(stored) {
			t.Fatalf("raw body not retained: %v", err)
		}
		info, _ := os.Stat(filepath.Join(dir, r.ResponseFile))
		if info.Mode().Perm() != 0600 {
			t.Fatal("response permissions must be 0600")
		}
	}
	if !strings.Contains(string(body), "9007199254740993") || !strings.Contains(string(body), "/a~1b") {
		t.Fatal("field inventory lost integer precision or JSON Pointer escaping")
	}
}

func TestHTTPFailuresAndMalformedResponses(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status int
		body   string
		valid  bool
	}{
		{"unauthorized", 401, `{"error":"bad credentials"}`, true},
		{"forbidden", 403, `{"error":"forbidden"}`, true},
		{"rate limited", 429, `{"error":"rate limit"}`, true},
		{"server failure", 503, `{"error":"unavailable"}`, true},
		{"invalid JSON", 200, `<html>upstream error</html>`, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := localClient(t, func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(tc.status); io.WriteString(w, tc.body) }, time.Second)
			r := c.Scan(context.Background(), "risk", riskFixture)
			if r.Error == "" || r.HTTPStatus != tc.status || r.ValidJSON != tc.valid || string(r.body) != tc.body || r.RiskInterpretation != "unreviewed" {
				t.Fatalf("failure was not retained correctly: %+v", r)
			}
		})
	}
}

func TestTimeoutAndInvalidAddress(t *testing.T) {
	c := localClient(t, func(w http.ResponseWriter, r *http.Request) { <-r.Context().Done() }, 30*time.Millisecond)
	r := c.Scan(context.Background(), "normal", normalFixture)
	if r.Error == "" || r.HTTPOK || r.DurationMS < 20 {
		t.Fatalf("timeout not captured: %+v", r)
	}
	r = c.Scan(context.Background(), "risk", "../../other")
	if r.Error == "" || r.URL != "" {
		t.Fatal("invalid address reached transport")
	}
}

func TestRedirectIsNotFollowed(t *testing.T) {
	calls := 0
	c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		calls++
		http.Redirect(w, r, "https://example.invalid/collect-key", http.StatusFound)
	}, time.Second)
	r := c.Scan(context.Background(), "normal", normalFixture)
	if calls != 1 || r.HTTPStatus != 302 || r.Error == "" {
		t.Fatalf("redirect behavior: %+v calls=%d", r, calls)
	}
}

func TestBodyLimitAndSecretRedaction(t *testing.T) {
	t.Run("limit", func(t *testing.T) {
		c := localClient(t, func(w http.ResponseWriter, _ *http.Request) {
			io.WriteString(w, strings.Repeat("x", maxResponseBytes+20))
		}, time.Second)
		r := c.Scan(context.Background(), "normal", normalFixture)
		if !r.Truncated || r.Error == "" || r.ValidJSON || len(r.body) != maxResponseBytes {
			t.Fatalf("body limit failed: %+v", r)
		}
	})
	t.Run("redaction", func(t *testing.T) {
		c := localClient(t, func(w http.ResponseWriter, _ *http.Request) { io.WriteString(w, `{"echo":"test-only-key"}`) }, time.Second)
		r := c.Scan(context.Background(), "normal", normalFixture)
		if !r.SecretRedacted || strings.Contains(string(r.body), "test-only-key") || !r.ValidJSON {
			t.Fatal("key must not be written into the report")
		}
	})
}

func TestSecondFailureRetainsFirstResponse(t *testing.T) {
	c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, riskFixture) {
			w.WriteHeader(429)
		}
		io.WriteString(w, `{"synthetic":true}`)
	}, time.Second)
	dir, err := Run(context.Background(), Config{NormalAddress: normalFixture, RiskAddress: riskFixture}, c, t.TempDir(), io.Discard)
	if err == nil {
		t.Fatal("failed call must fail the run")
	}
	body, readErr := os.ReadFile(filepath.Join(dir, "report.json"))
	if readErr != nil {
		t.Fatal(readErr)
	}
	var report Report
	json.Unmarshal(body, &report)
	if report.CollectionComplete || len(report.Results) != 2 || report.Results[0].HTTPStatus != 200 || report.Results[1].HTTPStatus != 429 {
		t.Fatalf("partial evidence lost: %+v", report)
	}
}

func TestConfigValidationAndPrecedence(t *testing.T) {
	for _, name := range []string{"INTERCEPTA_API_KEY", "INTERCEPTA_NORMAL_ADDRESS", "INTERCEPTA_RISK_ADDRESS", "INTERCEPTA_ADDRESS_SOURCE"} {
		value, exists := os.LookupEnv(name)
		os.Unsetenv(name)
		t.Cleanup(func() {
			if exists {
				os.Setenv(name, value)
			} else {
				os.Unsetenv(name)
			}
		})
	}
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("missing key must fail")
	}
	content := "# local fixture\nINTERCEPTA_API_KEY='file-key'\nINTERCEPTA_NORMAL_ADDRESS=" + normalFixture + "\nINTERCEPTA_RISK_ADDRESS=" + riskFixture + "\n"
	os.WriteFile(path, []byte(content), 0600)
	cfg, err := LoadConfig(path)
	if err != nil || cfg.APIKey != "file-key" {
		t.Fatalf("dotenv: %v", err)
	}
	t.Setenv("INTERCEPTA_API_KEY", "environment-key")
	cfg, err = LoadConfig(path)
	if err != nil || cfg.APIKey != "environment-key" {
		t.Fatal("environment must override file")
	}
	t.Setenv("INTERCEPTA_RISK_ADDRESS", normalFixture)
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("duplicate addresses must fail")
	}
	t.Setenv("INTERCEPTA_RISK_ADDRESS", "invalid")
	if _, err := LoadConfig(path); err == nil {
		t.Fatal("invalid address must fail")
	}
	os.WriteFile(path, []byte("super-secret-without-equals"), 0600)
	if _, err := LoadConfig(path); err == nil || strings.Contains(err.Error(), "super-secret") {
		t.Fatal("parse error must not echo secret lines")
	}
}
