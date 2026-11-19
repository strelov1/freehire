package kit

import (
	"math"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// RoundSalaryPart converts one structured salary bound from an ATS's own JSON (possibly
// fractional, e.g. Lever's $35.22 hourly) into integer salary_min/salary_max, rounding rather
// than truncating. A non-positive value (some ATS APIs emit zero for "not set") reports nil
// rather than a fabricated zero, as enrich.Sanitize does; min and max are independent, so a
// one-sided "up to $X" range still comes through.
func RoundSalaryPart(v float64) *int {
	if v <= 0 {
		return nil
	}
	r := int(math.Round(v))
	return &r
}

// IsSalaryPeriod reports whether period is one of freehire's own salary_period values. It
// validates sources whose vocabulary already matches ours (Recruitee); others map first.
func IsSalaryPeriod(period string) bool {
	for _, p := range vocab.SalaryPeriodValues {
		if p == period {
			return true
		}
	}
	return false
}

// RubMonthlySalary states a Russian employer's own salary bounds: roubles per month, the
// convention every Russian careers site (and hh.ru) quotes in. A posting with neither bound
// states no salary at all.
func RubMonthlySalary(from, to *float64) (salaryMin, salaryMax *int, currency, period string) {
	if from != nil {
		salaryMin = RoundSalaryPart(*from)
	}
	if to != nil {
		salaryMax = RoundSalaryPart(*to)
	}
	if salaryMin == nil && salaryMax == nil {
		return nil, nil, "", ""
	}
	return salaryMin, salaryMax, "RUB", "month"
}
