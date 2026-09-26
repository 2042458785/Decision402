package agent

import (
	"fmt"
	"strconv"
	"strings"
)

// selectionSummary is derived from screened candidates and the Go policy.
// Model prose must not turn a live preview into a claimed simulation/payment.
func selectionSummary(p Policy, candidates []Candidate, selected *Candidate) string {
	var blocked, held, excluded, eligible []string
	for _, c := range candidates {
		switch {
		case c.Eligible:
			eligible = append(eligible, c.Service.ID)
		case c.Level >= 2:
			blocked = append(blocked, c.Service.ID)
		case c.Level < 0:
			held = append(held, c.Service.ID)
		default:
			excluded = append(excluded, c.Service.ID)
		}
	}
	var filters []string
	if len(blocked) > 0 {
		filters = append(filters, strings.Join(blocked, "/")+" blocked for risk")
	}
	if len(held) > 0 {
		filters = append(filters, strings.Join(held, "/")+" held for missing risk data")
	}
	if len(excluded) > 0 {
		filters = append(filters, strings.Join(excluded, "/")+" outside policy or budget")
	}
	if len(eligible) > 0 {
		filters = append(filters, strings.Join(eligible, "/")+" eligible")
	}
	if selected == nil || selected.Quote == nil {
		return "Screening: " + strings.Join(filters, "; ") + ". Selection: no eligible service."
	}
	amount, _ := strconv.ParseInt(selected.Quote.Amount, 10, 64)
	price := strings.TrimRight(strings.TrimRight(FormatMoney(amount), "0"), ".")
	preference := "price first"
	if p.Preference == "risk" {
		preference = "risk first; price breaks ties"
	}
	return fmt.Sprintf("Screening: %s. Selection: %s; chose %s (%s USDC).", strings.Join(filters, "; "), preference, selected.Service.ID, price)
}
