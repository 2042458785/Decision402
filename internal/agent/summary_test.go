package agent

import (
	"strings"
	"testing"
)

func TestSelectionSummaryExplainsRealDemo(t *testing.T) {
	p := Policy{PerPayment: "0.1", TaskBudget: "0.1", MaxRisk: 0, Preference: "price"}
	candidates, selected := Rank(p, []Candidate{
		{Service: Service{ID: "A"}, Level: 2, Quote: testQuote("5000")},
		{Service: Service{ID: "B"}, Level: 2, Quote: testQuote("20000")},
		{Service: Service{ID: "C"}, Level: 0, Quote: testQuote("10000")},
		{Service: Service{ID: "D"}, Level: 0, Quote: testQuote("50000")},
	})
	got := selectionSummary(p, candidates, selected)
	want := "Screening: A/B blocked for risk; C/D eligible. Selection: price first; chose C (0.01 USDC)."
	if got != want {
		t.Fatalf("summary=%q, want %q", got, want)
	}
}

func TestSelectionSummaryShowsNoEligibleService(t *testing.T) {
	p := Policy{PerPayment: "0.005", TaskBudget: "0.005", MaxRisk: 0, Preference: "price"}
	candidates, selected := Rank(p, []Candidate{
		{Service: Service{ID: "A"}, Level: 2, Quote: testQuote("5000")},
		{Service: Service{ID: "C"}, Level: 0, Quote: testQuote("10000")},
	})
	if selected != nil || !strings.Contains(selectionSummary(p, candidates, selected), "no eligible service") {
		t.Fatalf("expected no eligible service, got %+v", selected)
	}
}
