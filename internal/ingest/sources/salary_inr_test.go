package sources

import "testing"

func TestINRSalary(t *testing.T) {
	type want struct {
		lo, hi int // 0 = absent
		period string
	}
	tests := []struct {
		label string
		want  *want // nil = no salary
	}{
		// Lakh and crore figures are annual CTC, the unit Indian salaries are quoted in.
		{"₹12L – ₹18L", &want{1200000, 1800000, "year"}},
		{"₹ 12-18 LPA", &want{1200000, 1800000, "year"}},
		{"12-18 Lacs P.A.", &want{1200000, 1800000, "year"}},
		{"12 Lacs P.A.", &want{1200000, 0, "year"}},
		{"INR 12 - 16 LPA", &want{1200000, 1600000, "year"}},
		{"₹2.0 LPA – ₹3.5 LPA", &want{200000, 350000, "year"}},
		{"95 Lacs-2.5 Cr PA", &want{9500000, 25000000, "year"}},
		{"₹40L – ₹1 cr", &want{4000000, 10000000, "year"}},
		{"15 to 18 LPA", &want{1500000, 1800000, "year"}},
		{"Upto 5 LPA", &want{0, 500000, "year"}},
		{"Up to 60 Lakhs", &want{0, 6000000, "year"}},
		{"₹20–30 LPA", &want{2000000, 3000000, "year"}},
		{"CTC: ₹20–30 LPA", &want{2000000, 3000000, "year"}},
		{"12.5 lakhs per annum", &want{1250000, 0, "year"}},
		// Plain rupees state their period, or state no salary at all.
		{"INR 50,000 per month", &want{50000, 0, "month"}},
		{"₹25,000 – ₹30,000 per month", &want{25000, 30000, "month"}},
		{"₹10,000 /month", &want{10000, 0, "month"}},
		{"Rs. 3,00,000 - 4,50,000 per annum", &want{300000, 450000, "year"}},
		{"₹1.5 lakh per month", &want{150000, 0, "month"}},
		{"₹3,00,000 – ₹6,00,000", nil},
		{"₹10,000 – ₹15,000", nil},
		{"₹75,000 – ₹1.5L", nil},
		{"₹60,000 – ₹1.2L per month", &want{60000, 120000, "month"}},
		// No figure.
		{"Not Disclosed", nil},
		{"Not disclosed by recruiter", nil},
		{"Competitive salary + ESOPs", nil},
		{"", nil},
		// Another currency is not this parser's to read.
		{"$150k – $180k", nil},
		// Inverted or zero ranges are not salaries.
		{"₹18L – ₹12L", nil},
		{"0-0 Lacs P.A.", nil},
	}
	for _, tc := range tests {
		lo, hi, cur, period := inrSalary(tc.label)
		if tc.want == nil {
			if lo != nil || hi != nil || cur != "" || period != "" {
				t.Errorf("inrSalary(%q) = %v/%v %q %q, want none", tc.label, intOrZero(lo), intOrZero(hi), cur, period)
			}
			continue
		}
		if intOrZero(lo) != tc.want.lo || intOrZero(hi) != tc.want.hi || cur != "INR" || period != tc.want.period {
			t.Errorf("inrSalary(%q) = %d/%d %q %q, want %d/%d INR %q", tc.label, intOrZero(lo), intOrZero(hi), cur, period, tc.want.lo, tc.want.hi, tc.want.period)
		}
	}
}

func intOrZero(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
