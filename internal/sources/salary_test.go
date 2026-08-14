package sources

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
		got := roundSalaryPart(c.in)
		if (got == nil) != (c.want == nil) || (got != nil && *got != *c.want) {
			t.Errorf("roundSalaryPart(%v) = %v, want %v", c.in, deref(got), deref(c.want))
		}
	}
}

func TestIsSalaryPeriod(t *testing.T) {
	for _, p := range []string{"year", "month", "day", "hour"} {
		if !isSalaryPeriod(p) {
			t.Errorf("isSalaryPeriod(%q) = false, want true", p)
		}
	}
	for _, p := range []string{"", "week", "annual", "YEAR"} {
		if isSalaryPeriod(p) {
			t.Errorf("isSalaryPeriod(%q) = true, want false", p)
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
