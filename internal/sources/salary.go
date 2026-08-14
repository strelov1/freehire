package sources

import (
	"math"

	"github.com/strelov1/freehire/internal/vocab"
)

// roundSalaryPart converts one structured salary bound from an ATS's own JSON
// (occasionally fractional — Lever reports hourly wages like $35.22) into freehire's
// integer salary_min/salary_max, rounding rather than truncating so a fractional
// rate is never inflated by a stray decimal point (see the enrich package's identical
// fix for the LLM path). A non-positive value — the shape some ATS APIs emit for "not
// set" (an all-zero range) — reports absent (nil) rather than a fabricated zero, same
// as enrich.Sanitize's positiveOrNil for the LLM path; min and max are independent, so
// a one-sided "up to $X" range still comes through.
func roundSalaryPart(v float64) *int {
	if v <= 0 {
		return nil
	}
	r := int(math.Round(v))
	return &r
}

// isSalaryPeriod reports whether period is one of freehire's own salary_period
// values. For a source (like Recruitee) whose own period vocabulary already matches
// ours exactly, this is validation rather than translation — an adapter with a
// different vocabulary (Lever, Ashby) maps first and never needs this.
func isSalaryPeriod(period string) bool {
	for _, p := range vocab.SalaryPeriodValues {
		if p == period {
			return true
		}
	}
	return false
}
