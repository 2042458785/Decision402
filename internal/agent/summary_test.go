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
	want := "过滤：A/B 风险阻断；C/D 符合授权。筛选：价格优先，选择 C（0.01 USDC）。"
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
	if selected != nil || !strings.Contains(selectionSummary(p, candidates, selected), "没有合格服务") {
		t.Fatalf("expected no eligible service, got %+v", selected)
	}
}
