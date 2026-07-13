package notify

import (
	"strconv"
	"strings"
)

// currencySymbols maps the common ISO 4217 codes to a glyph; any other code is
// used as a prefix verbatim (e.g. "PLN 20K").
var currencySymbols = map[string]string{"USD": "$", "EUR": "€", "GBP": "£"}

// SalaryString renders a compensation string like "$130K—$170K / year" from the
// digest job's salary fields, or "" when no figure is known. Both channel
// notifiers (Telegram, email) render the same string; they differ only in the
// surrounding markup, so the formatting lives here once instead of per channel.
func (j DigestJob) SalaryString() string {
	return formatSalary(j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod)
}

// formatSalary renders a compensation string from the raw salary fields, or "" when
// no figure is known. A zero bound counts as absent (matching enrichment's
// positive-or-nil convention), so a one-sided range renders alone. Amounts of 1000+
// are abbreviated with a K suffix; smaller figures (e.g. hourly rates) are shown in full.
func formatSalary(min, max int, currency, period string) string {
	if min <= 0 && max <= 0 {
		return ""
	}
	sym := currencySymbols[strings.ToUpper(currency)]
	if sym == "" && currency != "" {
		sym = currency + " "
	}
	var amount string
	switch {
	case min > 0 && max > 0 && min != max:
		amount = sym + shortMoney(min) + "—" + sym + shortMoney(max)
	case min > 0:
		amount = sym + shortMoney(min)
	default: // only max is known
		amount = sym + shortMoney(max)
	}
	if period != "" {
		amount += " / " + period
	}
	return amount
}

// shortMoney abbreviates 12000→"12K", 4500→"4.5K", and leaves sub-thousand
// figures (e.g. hourly rates) in full: 950→"950".
func shortMoney(v int) string {
	if v < 1000 {
		return strconv.Itoa(v)
	}
	if v%1000 == 0 {
		return strconv.Itoa(v/1000) + "K"
	}
	return strconv.FormatFloat(float64(v)/1000, 'f', 1, 64) + "K"
}

// Plural returns the plural suffix for a count: "" for exactly one, "s" otherwise.
func Plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}
