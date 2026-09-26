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
		filters = append(filters, strings.Join(blocked, "/")+" 风险阻断")
	}
	if len(held) > 0 {
		filters = append(filters, strings.Join(held, "/")+" 信息不足暂停")
	}
	if len(excluded) > 0 {
		filters = append(filters, strings.Join(excluded, "/")+" 超出授权或预算")
	}
	if len(eligible) > 0 {
		filters = append(filters, strings.Join(eligible, "/")+" 符合授权")
	}
	if selected == nil || selected.Quote == nil {
		return "过滤：" + strings.Join(filters, "；") + "。筛选：没有合格服务。"
	}
	amount, _ := strconv.ParseInt(selected.Quote.Amount, 10, 64)
	price := strings.TrimRight(strings.TrimRight(FormatMoney(amount), "0"), ".")
	preference := "价格优先"
	if p.Preference == "risk" {
		preference = "风险优先，同等级比较价格"
	}
	return fmt.Sprintf("过滤：%s。筛选：%s，选择 %s（%s USDC）。", strings.Join(filters, "；"), preference, selected.Service.ID, price)
}
