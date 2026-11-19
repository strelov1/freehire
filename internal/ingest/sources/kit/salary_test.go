package kit

import "testing"

func TestRoundSalaryPart(t *testing.T) {
	cases := []struct {
		in   float64
		want *int
	}{
		{35.22, intPtr(35)}, // rounds, never truncates (a stray decimal must not inflate)
		{35.5, intPtr(36)},  // rounds half up
		{100000, intPtr(100000)},
		{0, nil},  // the shape some ATS APIs emit for "not set"
		{-5, nil}, // nonsensical
	}
	for _, c := range cases {
		got := RoundSalaryPart(c.in)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("RoundSalaryPart(%v) = %v, want %v", c.in, deref(got), deref(c.want))
		}
	}
}

func TestIsSalaryPeriod(t *testing.T) {
	for _, p := range []string{"year", "month", "day", "hour"} {
		if !IsSalaryPeriod(p) {
			t.Errorf("IsSalaryPeriod(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"", "week", "annual", "YEAR"} {
		if IsSalaryPeriod(p) {
			t.Errorf("IsSalaryPeriod(%q) = true, want false", p)
		}
	}
}

func intPtr(v int) *int { return &v }

func deref(p *int) any {
	if p == nil {
		return nil
	}
	return *p
}

func TestRubMonthlySalary(t *testing.T) {
	from, to := 100000.4, 150000.0
	lo, hi, cur, per := RubMonthlySalary(&from, &to)
	if deref(lo) != 100000 || deref(hi) != 150000 || cur != "RUB" || per != "month" {
		t.Errorf("RubMonthlySalary = %v %v %q %q", deref(lo), deref(hi), cur, per)
	}
	if lo, hi, cur, per := RubMonthlySalary(nil, nil); lo != nil || hi != nil || cur != "" || per != "" {
		t.Errorf("no bounds: got %v %v %q %q, want no salary", lo, hi, cur, per)
	}
}
