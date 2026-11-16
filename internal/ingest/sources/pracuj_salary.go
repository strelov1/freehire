package sources

import (
	"fmt"
	"html"
	"strconv"
	"strings"
)

// pracujSalary reads one stated salary: the contract of employment's gross pay if stated, else
// another gross one, else a B2B contract's net pay — the order in which a figure is comparable to
// the rest of the catalogue. Only PLN, with a stated kind (gross, or net for B2B) and a monthly
// (0) or hourly (1) period. Every contract's pay is also written into the body (pracujPayParagraph).
func pracujSalary(contracts []pracujContract) (lo, hi int, period string, ok bool) {
	pick := func(match func(pracujContract) bool) (int, int, string, bool) {
		for _, c := range contracts {
			if !match(c) {
				continue
			}
			if lo, hi, period, ok := pracujContractPay(c); ok {
				return lo, hi, period, true
			}
		}
		return 0, 0, "", false
	}
	gross := func(c pracujContract) bool { return c.Salary != nil && c.Salary.Kind.Code == "gross" }
	if lo, hi, period, ok = pick(func(c pracujContract) bool { return c.ID == pracujContractUoP && gross(c) }); ok {
		return
	}
	if lo, hi, period, ok = pick(gross); ok {
		return
	}
	return pick(func(c pracujContract) bool {
		return c.ID == pracujContractB2B && c.Salary != nil && strings.HasPrefix(c.Salary.Kind.Code, "net")
	})
}

// pracujContractPay is one contract's structured pay, or ok=false when it is not a PLN range with
// a monthly or hourly period. A single figure is stated as from == to.
func pracujContractPay(c pracujContract) (lo, hi int, period string, ok bool) {
	s := c.Salary
	if s == nil || s.From == nil || !strings.EqualFold(strings.TrimSpace(s.Currency.Code), "PLN") {
		return 0, 0, "", false
	}
	switch s.TimeUnit.ID {
	case 0:
		period = "month"
	case 1:
		period = "hour"
	default:
		return 0, 0, "", false
	}
	from, to := *s.From, *s.From
	if s.To != nil {
		to = *s.To
	}
	lo, hi = int(from+0.5), int(to+0.5)
	if lo <= 0 || hi < lo {
		return 0, 0, "", false
	}
	return lo, hi, period, true
}

// pracujPayParagraph states every contract's pay as the posting does — amount, gross or net,
// period and contract — because the salary fields hold one figure and drop which contract and
// tax basis it was.
func pracujPayParagraph(contracts []pracujContract) string {
	var lines []string
	for _, c := range contracts {
		lo, hi, _, ok := pracujContractPay(c)
		if !ok {
			continue
		}
		amount := strconv.Itoa(lo)
		if hi != lo {
			amount += "–" + strconv.Itoa(hi)
		}
		line := fmt.Sprintf("%s PLN %s / %s (%s)", amount, collapseSpace(c.Salary.Kind.Name), collapseSpace(c.Salary.TimeUnit.LongForm.Name), collapseSpace(c.Name))
		lines = append(lines, "<li>"+html.EscapeString(line)+"</li>")
	}
	if len(lines) == 0 {
		return ""
	}
	return "<ul>" + strings.Join(lines, "") + "</ul>\n"
}
