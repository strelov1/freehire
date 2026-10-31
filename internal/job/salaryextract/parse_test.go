package salaryextract

import "testing"

func TestParseLabeledSalary(t *testing.T) {
	cases := []struct {
		name      string
		text      string
		countries []string
		min, max  int
		minSet    bool
		maxSet    bool
		currency  string
		period    string
		ok        bool
	}{
		{
			name:      "whatjobs range, second bound without a dollar",
			text:      "<p>Salary: $120,000 - 185,000 per year</p><p>Build services.</p>",
			countries: []string{"us"},
			min:       120000, max: 185000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{
			name:      "seek en dash",
			text:      "Salary: $160,000 – $190,000 per year",
			countries: []string{"au"},
			min:       160000, max: 190000, minSet: true, maxSet: true,
			currency: "AUD", period: "year", ok: true,
		},
		{
			name: "jobstreet dotted rupiah",
			text: "Salary: Rp 6.000.000 – Rp 7.500.000 per month",
			min:  6000000, max: 7500000, minSet: true, maxSet: true,
			currency: "IDR", period: "month", ok: true,
		},
		{
			name:      "pay line without a period",
			text:      "Pay: $92,000 - $95,000",
			countries: []string{"us"},
			min:       92000, max: 95000, minSet: true, maxSet: true,
			currency: "USD", ok: true,
		},
		{
			name:      "up to, bonus ignored",
			text:      "Salary: Up to $240,000 + Bonus",
			countries: []string{"au"},
			max:       240000, maxSet: true,
			currency: "AUD", ok: true,
		},
		{
			name:      "from, percent ignored",
			text:      "Remuneration: Level 8 – from $132,445 + 17% superannuation.",
			countries: []string{"au"},
			min:       132445, minSet: true,
			currency: "AUD", ok: true,
		},
		{
			name:      "point salary with slash period",
			text:      "Pay: $130,500/year",
			countries: []string{"us"},
			min:       130500, max: 130500, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{
			name: "euro after dotted thousands",
			text: "Salary: 50.000 - 80.000 € per year",
			min:  50000, max: 80000, minSet: true, maxSet: true,
			currency: "EUR", period: "year", ok: true,
		},
		{
			name: "hourly fractional pounds",
			text: "Pay: £15.59 per hr",
			min:  16, max: 16, minSet: true, maxSet: true,
			currency: "GBP", period: "hour", ok: true,
		},
		{
			name:      "monthly slash, dollar sign outside the dollar countries",
			text:      "Salary: $1,900 - $2,800 / month",
			countries: []string{"cl"},
			min:       1900, max: 2800, minSet: true, maxSet: true,
			period: "month", ok: true,
		},
		{
			name: "bare dollar with no country stays uncoded",
			text: "Salary: $160,000 – $190,000 per year",
			min:  160000, max: 190000, minSet: true, maxSet: true,
			period: "year", ok: true,
		},
		{
			name:      "mixed dollar countries stay uncoded",
			text:      "Salary: $160,000 per year",
			countries: []string{"us", "au"},
			min:       160000, max: 160000, minSet: true, maxSet: true,
			period: "year", ok: true,
		},
		{
			name: "empty label",
			text: "Salary:\nA long posting.",
		},
		{
			name: "not a number",
			text: "Salary: Rates Negotiable",
		},
		{
			name:      "small figure with no period",
			text:      "Salary: $140+",
			countries: []string{"au"},
		},
		{
			name:      "header year disagrees with an hourly body",
			text:      "Salary: $104,000 - 208,000 per year\n\nOur work is paid hourly, with competitive earnings up to $100+ USD.",
			countries: []string{"us"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, ok := Parse(tc.text, tc.countries)
			if ok != tc.ok {
				t.Fatalf("ok = %v, want %v (%+v)", ok, tc.ok, got)
			}
			if !tc.ok {
				return
			}
			if tc.minSet != (got.Min != nil) || (got.Min != nil && *got.Min != tc.min) {
				t.Errorf("min = %v, want set=%v %d", got.Min, tc.minSet, tc.min)
			}
			if tc.maxSet != (got.Max != nil) || (got.Max != nil && *got.Max != tc.max) {
				t.Errorf("max = %v, want set=%v %d", got.Max, tc.maxSet, tc.max)
			}
			if got.Currency != tc.currency || got.Period != tc.period {
				t.Errorf("currency/period = %q/%q, want %q/%q", got.Currency, got.Period, tc.currency, tc.period)
			}
		})
	}
}
