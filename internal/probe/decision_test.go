package probe

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestScreenPolicy(t *testing.T) {
	for _, tc := range []struct {
		name, body, action string
		status             int
		level              int
		lowTraits          []string
	}{
		{"zero", `{"toxicScore":0,"traits":[]}`, "allow", 200, 0, nil},
		{"risk", `{"toxicScore":100,"traits":[{"risk":100,"name":"known_scammer","description":"reported malicious activity"}]}`, "deny", 200, 2, nil},
		{"positive score", `{"toxicScore":1,"traits":[]}`, "deny", 200, 2, nil},
		{"unknown trait", `{"toxicScore":0,"traits":[{"risk":0,"name":"unknown_trait","description":"review"}]}`, "hold", 200, -1, nil},
		{"reviewed advisory without configuration", `{"toxicScore":0,"traits":[{"risk":10,"name":"reviewed_advisory","description":"minor exposure"}]}`, "hold", 200, -1, nil},
		{"reviewed advisory configured", `{"toxicScore":0,"traits":[{"risk":10,"name":"reviewed_advisory","description":"minor exposure"}]}`, "allow", 200, 1, []string{"reviewed_advisory"}},
		{"advisory score increased", `{"toxicScore":1,"traits":[{"risk":10,"name":"reviewed_advisory","description":"minor exposure"}]}`, "deny", 200, 2, []string{"reviewed_advisory"}},
		{"advisory trait risk increased", `{"toxicScore":0,"traits":[{"risk":21,"name":"reviewed_advisory","description":"exposure"}]}`, "deny", 200, 2, []string{"reviewed_advisory"}},
		{"severe name cannot be allowlisted", `{"toxicScore":0,"traits":[{"risk":0,"name":"sanctioned_address","description":"exposure"}]}`, "deny", 200, 2, []string{"sanctioned_address"}},
		{"mixed traits held", `{"toxicScore":0,"traits":[{"risk":1,"name":"reviewed_advisory","description":"minor"},{"risk":1,"name":"unknown_trait","description":"review"}]}`, "hold", 200, -1, []string{"reviewed_advisory"}},
		{"empty", `{}`, "hold", 200, -1, nil},
		{"null", `null`, "hold", 200, -1, nil},
		{"missing traits", `{"toxicScore":0}`, "hold", 200, -1, nil},
		{"null traits", `{"toxicScore":0,"traits":null}`, "hold", 200, -1, nil},
		{"null score", `{"toxicScore":null,"traits":[]}`, "hold", 200, -1, nil},
		{"string score", `{"toxicScore":"0","traits":[]}`, "hold", 200, -1, nil},
		{"negative", `{"toxicScore":-1,"traits":[]}`, "hold", 200, -1, nil},
		{"too large", `{"toxicScore":101,"traits":[]}`, "hold", 200, -1, nil},
		{"invalid trait", `{"toxicScore":0,"traits":[{}]}`, "hold", 200, -1, nil},
		{"malformed", `{`, "hold", 200, -1, nil},
		{"unauthorized", `{"toxicScore":0,"traits":[]}`, "hold", 401, -1, nil},
		{"rate limit", `{"toxicScore":0,"traits":[]}`, "hold", 429, -1, nil},
		{"server failure", `{"toxicScore":0,"traits":[]}`, "hold", 503, -1, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				fmt.Fprint(w, tc.body)
			}, time.Second)
			c.lowRiskTraits = make(map[string]struct{})
			for _, name := range tc.lowTraits {
				c.lowRiskTraits[name] = struct{}{}
			}
			d := c.Screen(context.Background(), normalFixture)
			if d.Action != tc.action || d.Level != tc.level || d.Address != normalFixture || d.HTTPStatus != tc.status || d.DurationMS <= 0 {
				t.Fatalf("unexpected decision: %+v", d)
			}
		})
	}
}

func TestCanceledScanHolds(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	c := localClient(t, func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled scan should not reach server")
	}, time.Second)
	if d := c.Screen(ctx, normalFixture); d.Action != "hold" || d.Level != -1 {
		t.Fatalf("got %+v", d)
	}
}
