package agent

import (
	"errors"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"Decision402/internal/probe"
	"github.com/x402-foundation/x402/go/v2/types"
)

const Network = "eip155:84532"
const Asset = "0x036CbD53842c5426634e7929541eC2318f3dCF7e"
const MaxDemoAtomic int64 = 100000 // 0.10 test USDC; a server-side ceiling.

var addressPattern = regexp.MustCompile(`^0x[0-9a-fA-F]{40}$`)
var moneyPattern = regexp.MustCompile(`^(0|[1-9][0-9]*)(\.[0-9]{1,6})?$`)

// Money never uses floating point. All payment arithmetic uses micro-USDC.
func Money(s string) (int64, error) {
	if len(s) > 16 || !moneyPattern.MatchString(s) {
		return 0, errors.New("Amount must be a positive decimal string with at most six fractional digits")
	}
	parts := strings.SplitN(s, ".", 2)
	fraction := ""
	if len(parts) == 2 {
		fraction = parts[1]
	}
	fraction += strings.Repeat("0", 6-len(fraction))
	return strconv.ParseInt(parts[0]+fraction, 10, 64)
}
func FormatMoney(n int64) string { return fmt.Sprintf("%d.%06d", n/1000000, n%1000000) }

type Policy struct {
	PerPayment string `json:"per_payment"`
	TaskBudget string `json:"task_budget"`
	MaxRisk    int    `json:"max_risk"`
	Preference string `json:"preference"`
}

func (p Policy) Validate() error {
	single, e1 := Money(p.PerPayment)
	total, e2 := Money(p.TaskBudget)
	if e1 != nil || e2 != nil || single <= 0 || total <= 0 || single > MaxDemoAtomic || total > MaxDemoAtomic {
		return errors.New("Local demo: per-payment and task budgets must be above zero and at most 0.10 USDC")
	}
	if p.MaxRisk != 0 && p.MaxRisk != 1 {
		return errors.New("Only risk levels 0 and 1 can be authorized; high risk is always blocked")
	}
	if p.Preference != "price" && p.Preference != "risk" {
		return errors.New("Priority must be price or risk")
	}
	return nil
}

type Service struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	URL    string `json:"url"`
	PayTo  string `json:"pay_to"`
	Amount string `json:"amount"`
}
type Candidate struct {
	Service  Service                    `json:"service"`
	Quote    *types.PaymentRequirements `json:"quote,omitempty"`
	Risk     *probe.Decision            `json:"risk,omitempty"`
	Level    int                        `json:"level"` // -1 unknown, 0 no flags, 1 approved advisory risk, 2 blocked.
	Eligible bool                       `json:"eligible"`
	Reason   string                     `json:"reason"`
	Source   string                     `json:"source"`
}

func Rank(p Policy, candidates []Candidate) ([]Candidate, *Candidate) {
	single, _ := Money(p.PerPayment)
	total, _ := Money(p.TaskBudget)
	out := append([]Candidate(nil), candidates...)
	for i := range out {
		c := &out[i]
		c.Eligible = false
		if c.Quote == nil {
			if c.Reason == "" {
				c.Reason = "No valid offer"
			}
			continue
		}
		amount, err := strconv.ParseInt(c.Quote.Amount, 10, 64)
		switch {
		case c.Level < 0:
			if c.Reason == "" {
				c.Reason = "Risk information unavailable; payment on hold"
			}
		case c.Level >= 2:
			if c.Reason == "" {
				c.Reason = "Blocked by project policy"
			}
		case c.Level > p.MaxRisk:
			c.Reason = "Exceeds the owner's accepted risk level"
		case err != nil || amount <= 0 || amount > single || amount > total:
			c.Reason = "Exceeds the per-payment or task budget, or has an invalid amount"
		default:
			c.Eligible = true
			c.Reason = "Within the authorized budget and risk level"
		}
	}
	options := []Candidate{}
	for _, c := range out {
		if c.Eligible {
			options = append(options, c)
		}
	}
	sort.Slice(options, func(i, j int) bool {
		a, b := options[i], options[j]
		x, _ := strconv.ParseInt(a.Quote.Amount, 10, 64)
		y, _ := strconv.ParseInt(b.Quote.Amount, 10, 64)
		if p.Preference == "risk" && a.Level != b.Level {
			return a.Level < b.Level
		}
		if x != y {
			return x < y
		}
		if a.Level != b.Level {
			return a.Level < b.Level
		}
		return a.Service.ID < b.Service.ID
	})
	if len(options) == 0 {
		return out, nil
	}
	winner := options[0]
	if p.Preference == "price" {
		winner.Reason = "Lowest-priced eligible service; risk breaks price ties"
	} else {
		winner.Reason = "Lowest-risk eligible service; price breaks risk ties"
	}
	return out, &winner
}

func CheckQuote(q types.PaymentRequirements, s Service) error {
	if q.Scheme != "exact" || q.Network != Network || !strings.EqualFold(q.Asset, Asset) || !strings.EqualFold(q.PayTo, s.PayTo) || !addressPattern.MatchString(q.PayTo) {
		return errors.New("Offer network, token, scheme, or recipient does not match")
	}
	n, err := strconv.ParseInt(q.Amount, 10, 64)
	if err != nil || n <= 0 || n > MaxDemoAtomic || q.Amount != s.Amount {
		return errors.New("Offer amount changed or exceeds the demo cap")
	}
	if q.MaxTimeoutSeconds <= 0 || q.MaxTimeoutSeconds > 300 {
		return errors.New("Authorization timeout is out of range")
	}
	if q.Extra["name"] != "USDC" || q.Extra["version"] != "2" {
		return errors.New("USDC signing domain does not match")
	}
	if m, ok := q.Extra["assetTransferMethod"]; ok && m != "eip3009" && m != "" {
		return errors.New("Unsupported payment authorization method")
	}
	return nil
}
