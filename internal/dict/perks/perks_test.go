package perks

import (
	"slices"
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Every case is a phrase read off a real catalogue posting in the 2026-10-31 sample,
// trimmed to the clause that decides it.
func TestFromDescription(t *testing.T) {
	cases := []struct {
		name string
		desc string
		want []string
	}{
		// Asserted.
		{"generous pto", "Comprehensive Medical, Dental, and Vision Insurance Generous Paid Time Off and Extended Illness Benefits", []string{"generous_pto", "health_insurance"}},
		{"uk day count", "Company pension 25 days annual leave + Bank holidays", []string{"generous_pto", "retirement_plan"}},
		{"german day count", "Konditionen: 30 Tage Urlaub, Weihnachts- und Urlaubsgeld", []string{"generous_pto"}},
		{"unlimited is also generous", "Benefits & Perks Unlimited PTO Lunch & dinner in the office every day", []string{"generous_pto", "unlimited_pto"}},
		{"four day week", "Why Join Ford Aerospace? 4 day working week (Mon - Thurs)", []string{"four_day_week"}},
		{"four-day workweek", "Smart Fridays with the option of a four-day workweek", []string{"four_day_week"}},
		{"401k match", "401(k) with 50% employer match on the first 6% you contribute", []string{"401k_match", "retirement_plan"}},
		{"match before plan", "Company match on your 403(b) contributions", []string{"401k_match", "retirement_plan"}},
		{"plain 401k is a plan, not a match", "Benefits: 401(k) Health insurance", []string{"health_insurance", "retirement_plan"}},
		{"parental leave", "Paid parental leave for both mothers and fathers", []string{"parental_leave"}},
		{"superannuation", "Access to salary packaging and superannuation contributions", []string{"retirement_plan"}},
		{"tuition", "Tuition reimbursement program 403(b) with employer match", []string{"401k_match", "retirement_plan", "tuition_reimbursement"}},
		{"equity is not a perk", "Equity: 50-150 stock options (0.1-0.4%) vested over 4 years with a one-year cliff.", nil},
		{"private medical", "Contributory pension scheme Private medical insurance. Life assurance.", []string{"health_insurance", "retirement_plan"}},
		{"mental health", "Access to mental health resources, therapy, and coaching", []string{"mental_health"}},
		{"learning budget", "Hybrid office Competitive salary Learning and Development budget 24 days PTO", []string{"learning_budget"}},
		{"home office", "We also provide a $500 work-from-home stipend to support your remote setup", []string{"home_office_stipend"}},
		{"home office equipment", "Xantrion will provide a cell phone, internet connection, and home office equipment allowance.", []string{"home_office_stipend"}},
		{"sabbatical", "4 weeks sabbatical after 5 years", []string{"sabbatical"}},
		{"sign-on", "Competitive pay + $1,000 sign-on bonus Clean, safety-focused shop environment", []string{"sign_on_bonus"}},
		{"markup between label and value", "<ul><li><b>Unlimited</b> PTO</li><li>Sabbatical</li></ul>", []string{"generous_pto", "unlimited_pto", "sabbatical"}},

		// Near-misses.
		{"bare pto is not generous", "Benefits include paid time off and paid holidays.", nil},
		{"30-hour week is not four days", "W2 physicians earn $300,000 per year plus benefits for a 30-hour work week", nil},
		{"suite number is no 401k", "Visit us at 401 Main Street, Suite 457", nil},
		{"hipaa", "Must obtain Health Insurance Portability and Accountability Act (HIPAA) certification", nil},
		{"insurance billing duty", "1-2 years experience in healthcare insurance billing and verification", nil},
		{"knowledge of benefits", "Knowledge of case management and health insurance benefits.", nil},
		{"superannuation duty", "At least two years' experience in superannuation, life insurance or a related role.", nil},
		{"tuition duty", "Develop expertise in childcare subsidy and tuition assistance programs across service areas.", nil},
		{"un fund", "bring into operations the United Nations Capital Development Fund as an organ", nil},
		{"mental health services is the work", "committed to providing intensive mental health services to children", nil},
		{"in-office desk", "One Medical membership In-office desk setup stipend", nil},
		{"sign-on denied", "Recruitment/Relocation Incentives (Sign-on Bonus): Not Authorized", nil},
		{"does not include", "The rate displayed does not include benefits such as health insurance, paid or unpaid time off", nil},
		{"no 401k", "This is a contract role with no 401(k).", nil},
		{"empty", "", nil},

		// A loose negation must not cancel the perk.
		{"not limited to", "Our Total Rewards package includes, but is not limited to, paid time off, a 401K retirement plan", []string{"retirement_plan"}},
		{"no cost", "life insurance at no cost to you) Retirement programs, including 401(k) employer matching", []string{"401k_match", "retirement_plan"}},
		{"no vesting", "401(k) with no vesting period and a 4% company match", []string{"401k_match", "retirement_plan"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := FromDescription(c.desc)
			want := c.want
			slices.SortFunc(want, byVocab)
			if !slices.Equal(got, want) {
				t.Errorf("FromDescription(%q)\n got %v\nwant %v", c.desc, got, want)
			}
		})
	}
}

func byVocab(a, b string) int {
	return slices.Index(vocab.PerkValues, a) - slices.Index(vocab.PerkValues, b)
}

// Every emitted slug is a vocabulary member — the search filter validates against
// the same list, so a stray slug would be stored and never findable.
func TestRulesCoverVocab(t *testing.T) {
	if len(rules) != len(vocab.PerkValues) {
		t.Fatalf("%d rules, %d vocabulary values", len(rules), len(vocab.PerkValues))
	}
	for i, r := range rules {
		if r.slug != vocab.PerkValues[i] {
			t.Fatalf("rule %d is %q, vocab has %q", i, r.slug, vocab.PerkValues[i])
		}
		if r.label == "" {
			t.Errorf("rule %q has no label", r.slug)
		}
	}
}
