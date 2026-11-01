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
			name:      "benefits wording preserves both range bounds",
			text:      "Salary: $100,000 - $150,000 benefits included",
			countries: []string{"us"}, min: 100000, max: 150000, minSet: true, maxSet: true, currency: "USD", ok: true,
		},
		{
			name:      "bonus period does not override salary period",
			text:      "Salary Range:\n$120,000 per year + $5,000 bonus per month",
			countries: []string{"us"}, min: 120000, max: 120000, minSet: true, maxSet: true, currency: "USD", period: "year", ok: true,
		},
		{
			name:      "attached bonus label is not a floor",
			text:      "Salary: $120,000+ Bonus",
			countries: []string{"us"}, min: 120000, max: 120000, minSet: true, maxSet: true, currency: "USD", ok: true,
		},
		{
			name:      "attached bonus percentage is not a floor",
			text:      "Salary: $120,000+17%",
			countries: []string{"us"}, min: 120000, max: 120000, minSet: true, maxSet: true, currency: "USD", ok: true,
		},
		{
			name: "workday salary heading followed by European range",
			text: "<p><b>Salary Range:</b></p>27.510,00 € - 51.090,00 €<p><br/> </p><p><b>Job Description Summary</b></p>Build things.",
			min:  27510, max: 51090, minSet: true, maxSet: true,
			currency: "EUR", ok: true,
		},
		{
			name: "European fractional amount with trailing currency",
			text: "Salary: 27.510,60 € - 51.090,40 € per year",
			min:  27511, max: 51090, minSet: true, maxSet: true,
			currency: "EUR", period: "year", ok: true,
		},
		{name: "empty salary heading does not reach into benefits", text: "<p>Salary Range:</p><p>Benefits</p><p>€5,000 bonus</p>"},
		{name: "empty salary heading does not adopt a benefits line", text: "<p>Salary Range:</p><p>Benefits: €5,000 bonus</p>"},
		{name: "sign-on bonus under a salary heading is not salary", text: "Salary Range:\n$5,000 sign-on bonus", countries: []string{"us"}},
		{name: "trailing-currency sign-on bonus is not salary", text: "Salary Range:\n5.000 € sign-on bonus"},
		{name: "sign-on bonus with a period is not salary", text: "Salary Range:\n$5,000 sign-on bonus per year", countries: []string{"us"}},
		{
			name:      "pay line under a salary heading stays salary",
			text:      "Salary Range:\n$120,000 per year, 401k included",
			countries: []string{"us"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{
			name:      "bonus rider is not a floor",
			text:      "Salary Range:\n$120,000 + Bonus",
			countries: []string{"us"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", ok: true,
		},
		{
			name:      "trailing plus on the figure is a floor",
			text:      "Salary: $120,000+",
			countries: []string{"us"},
			min:       120000, minSet: true,
			currency: "USD", ok: true,
		},
		{name: "ungrouped scaled second bound is not dropped", text: "Salary: 100,000 - 150k per year"},
		{
			name: "CA$ under a salary heading is CAD",
			text: "Salary Range:\nCA$120,000 per year",
			min:  120000, max: 120000, minSet: true, maxSet: true,
			currency: "CAD", period: "year", ok: true,
		},
		{
			name: "AU$ under a salary heading is AUD",
			text: "Salary Range:\nAU$120,000 per year",
			min:  120000, max: 120000, minSet: true, maxSet: true,
			currency: "AUD", period: "year", ok: true,
		},
		{
			name:      "sign-on amount beside pay is not a bound",
			text:      "Salary Range:\n$120,000 per year + $5,000 sign-on bonus",
			countries: []string{"us"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{
			name:      "sign-on amount beside an inline salary is not a bound",
			text:      "Salary: $120,000 per year + $5,000 sign-on bonus",
			countries: []string{"us"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{name: "inline sign-on bonus is not salary", text: "Salary: $5,000 sign-on bonus", countries: []string{"us"}},
		{
			name:      "dashed 401k benefit is not a scaled bound",
			text:      "Salary: $120,000 per year, 401k - employer matched",
			countries: []string{"us"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{name: "dashed 401k with a later amount still refuses", text: "Salary: $120,000 per year, 401k - employer match up to $5,000", countries: []string{"us"}},
		{name: "scaled first bound before a dash still refuses", text: "Salary Range:\n100k - 150,000 per year"},
		{name: "ungrouped trailing-currency second bound is not dropped", text: "Salary Range:\n27.510,00 € - 51090 € per year"},
		{name: "unmarked scaled bounds are not read as thousands", text: "Salary Range:\n1,200k - 1,500k per year"},
		{name: "unlabeled range is not a salary", text: "<p>27.510,00 € - 51.090,00 €</p>"},
		{name: "k suffix is not read as 120", text: "Salary: $120k per year", countries: []string{"us"}},
		{name: "ungrouped second bound is not dropped", text: "Pay: $50 - 60 per hour", countries: []string{"us"}},
		{
			name:      "401k benefit is not a scaled amount",
			text:      "Salary: $120,000 per year, 401k included",
			countries: []string{"us"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{
			name:      "A$ is not retitled by a later USD",
			text:      "Salary: A$120,000 per year (USD equivalent)",
			countries: []string{"au"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "AUD", period: "year", ok: true,
		},
		{
			name:      "USD beside a bare dollar wins over the country",
			text:      "Salary: $120,000 USD per year",
			countries: []string{"au"},
			min:       120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{name: "A$ and USD on the same amount refuse", text: "Salary: A$120,000 USD per year"},
		{
			name: "US$ is USD, not SGD",
			text: "Salary: US$120,000 per year",
			min:  120000, max: 120000, minSet: true, maxSet: true,
			currency: "USD", period: "year", ok: true,
		},
		{
			name: "CA$ is CAD, not AUD",
			text: "Salary: CA$120,000 per year",
			min:  120000, max: 120000, minSet: true, maxSet: true,
			currency: "CAD", period: "year", ok: true,
		},
		{
			name: "AU$ is AUD",
			text: "Salary: AU$120,000 per year",
			min:  120000, max: 120000, minSet: true, maxSet: true,
			currency: "AUD", period: "year", ok: true,
		},
		{
			name: "S$ is SGD",
			text: "Salary: S$9,000 per month",
			min:  9000, max: 9000, minSet: true, maxSet: true,
			currency: "SGD", period: "month", ok: true,
		},
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
