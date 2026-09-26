package probe

import (
	"context"
	"encoding/json"
	"strings"
)

// Decision is our local demo policy, not an Intercepta allow/deny verdict.
// Level -1 means unknown/held; 0 clean; 1 explicitly allowlisted advisory
// signal; 2 blocked by our conservative policy.
type Decision struct {
	Action     string  `json:"action"`
	Level      int     `json:"level"`
	Address    string  `json:"address"`
	RiskData   string  `json:"risk_data"`
	HTTPStatus int     `json:"http_status"`
	DurationMS float64 `json:"duration_ms"`
	ToxicScore *int    `json:"toxicScore,omitempty"`
	Traits     []Trait `json:"traits,omitempty"`
	Reason     string  `json:"reason"`
}

type Trait struct {
	Risk        *int   `json:"risk"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

func (c *Client) Screen(ctx context.Context, address string) Decision {
	return evaluate(c.Scan(ctx, "payment", address), c.lowRiskTraits)
}

// Evaluate uses the default policy: live level 1 is disabled until exact
// low-risk trait names are reviewed and configured.
func Evaluate(r Result) Decision { return evaluate(r, nil) }

func evaluate(r Result, lowRiskTraits map[string]struct{}) Decision {
	d := Decision{Action: "hold", Level: -1, Address: r.Address, RiskData: "Intercepta mainnet risk data", HTTPStatus: r.HTTPStatus, DurationMS: r.DurationMS}
	if r.Error != "" || !r.HTTPOK || !r.ValidJSON || r.Truncated || r.SecretRedacted {
		d.Reason = "risk scan unavailable or invalid; no signature created"
		return d
	}
	var payload struct {
		ToxicScore *int            `json:"toxicScore"`
		Traits     json.RawMessage `json:"traits"`
	}
	if err := json.Unmarshal(r.body, &payload); err != nil || payload.ToxicScore == nil || *payload.ToxicScore < 0 || *payload.ToxicScore > 100 {
		d.Reason = "missing or invalid toxicScore; no signature created"
		return d
	}
	d.ToxicScore = payload.ToxicScore
	if len(payload.Traits) == 0 || string(payload.Traits) == "null" || json.Unmarshal(payload.Traits, &d.Traits) != nil || d.Traits == nil {
		d.Reason = "missing or invalid traits; no signature created"
		return d
	}
	for _, trait := range d.Traits {
		if trait.Risk == nil || *trait.Risk < 0 || *trait.Risk > 100 || trait.Name == "" || trait.Description == "" {
			d.Reason = "incomplete risk trait; no signature created"
			return d
		}
	}

	// These are project rules, not sponsor-published risk bands. Any positive
	// total score or trait risk above our advisory ceiling is blocked.
	if *d.ToxicScore > 0 {
		d.Action, d.Level = "deny", 2
		d.Reason = "project policy blocks any positive toxic score"
		return d
	}
	for _, trait := range d.Traits {
		name := strings.ToLower(trait.Name)
		hardSignal := false
		for _, marker := range []string{"sanction", "scam", "phish", "drain", "attack", "exploit", "malicious", "rug", "honeypot", "poison", "blackmail", "launder", "mixer"} {
			if strings.Contains(name, marker) {
				hardSignal = true
				break
			}
		}
		if hardSignal || *trait.Risk > 20 {
			d.Action, d.Level = "deny", 2
			d.Reason = "project policy blocks a severe or known malicious trait"
			return d
		}
		if _, ok := lowRiskTraits[trait.Name]; !ok {
			d.Reason = "unreviewed risk trait; hold until its exact API name is approved"
			return d
		}
	}
	if len(d.Traits) > 0 {
		d.Action, d.Level = "allow", 1
		d.Reason = "only explicitly allowlisted advisory traits; user risk authorization and quote checks still required"
		return d
	}
	d.Action, d.Level = "allow", 0
	d.Reason = "zero reported score and no traits; quote checks must also pass; not a safety guarantee"
	return d
}
