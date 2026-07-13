package notify

import "testing"

func TestSalaryString(t *testing.T) {
	cases := []struct {
		name             string
		min, max         int
		currency, period string
		want             string
	}{
		{"range", 130000, 170000, "USD", "year", "$130K—$170K / year"},
		{"min only", 90000, 0, "EUR", "year", "€90K / year"},
		{"max only", 0, 50000, "GBP", "month", "£50K / month"},
		{"equal bounds collapse", 100000, 100000, "USD", "year", "$100K / year"},
		{"unknown currency is a prefix", 20000, 30000, "PLN", "month", "PLN 20K—PLN 30K / month"},
		{"hourly rate not abbreviated", 50, 80, "USD", "hour", "$50—$80 / hour"},
		{"fractional thousands", 4500, 0, "USD", "", "$4.5K"},
		{"no figure", 0, 0, "USD", "year", ""},
	}
	for _, tc := range cases {
		j := DigestJob{SalaryMin: tc.min, SalaryMax: tc.max, SalaryCurrency: tc.currency, SalaryPeriod: tc.period}
		if got := j.SalaryString(); got != tc.want {
			t.Errorf("%s: SalaryString(%d,%d,%q,%q) = %q, want %q", tc.name, tc.min, tc.max, tc.currency, tc.period, got, tc.want)
		}
	}
}

func TestPlural(t *testing.T) {
	if got := Plural(1); got != "" {
		t.Errorf("Plural(1) = %q, want \"\"", got)
	}
	for _, n := range []int{0, 2, 5} {
		if got := Plural(n); got != "s" {
			t.Errorf("Plural(%d) = %q, want \"s\"", n, got)
		}
	}
}
