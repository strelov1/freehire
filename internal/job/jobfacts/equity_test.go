package jobfacts

import (
	"testing"

	"github.com/strelov1/freehire/internal/dict/vocab"
)

// Every case is a phrase read in a real sampled posting (2026-10-31), trimmed.
func TestEquity(t *testing.T) {
	tests := []struct {
		name, title, desc, want string
	}{
		// Equity beside cash pay.
		{"salary plus equity", "", "Compensation: Competitive salary + equity", vocab.EquityWithSalary},
		{"figure plus equity", "", "📍 London | 💰 £90k + equity + Incentive Awards", vocab.EquityWithSalary},
		{"bonus and/or equity", "", "Certain roles may be eligible for an annual bonus and/or equity grant.", vocab.EquityWithSalary},
		{"plus target equity", "", "The salary for this position is between $75,000 and $100,000, plus target equity.", vocab.EquityWithSalary},
		{"equity plus benefits", "", "$221000 (USD) + 42.86% bonus target + equity + benefits", vocab.EquityWithSalary},
		{"modified equity", "", "Benefits: Meaningful equity, Health coverage, Free meals", vocab.EquityWithSalary},
		{"equity grant noun", "", "Highly competitive equity grants are included in the majority of full time offers.", vocab.EquityWithSalary},
		{"offering verb", "", "In addition to a competitive base salary, all roles receive equity.", vocab.EquityWithSalary},
		{"pay and equity packages", "", "Formic's pay and equity packages are thoughtfully benchmarked against peer companies.", vocab.EquityWithSalary},
		{"stock options", "", "Benefits: $103,000 OTE, Stock options, Generous 401(k) match", vocab.EquityWithSalary},
		{"share options", "", "We share the success of the company by continuously issuing share options.", vocab.EquityWithSalary},
		{"RSUs", "", "Every full-time Celonaut is an owner, receiving Restricted Stock Units (RSUs).", vocab.EquityWithSalary},
		{"ESOP", "", "Employee Stock Ownership Plan (ESOP) – become a part-owner of the company.", vocab.EquityWithSalary},
		{"VSOP", "", "Deine Beteiligung: VSOP-Programm je nach Rolle.", vocab.EquityWithSalary},
		{"phantom equity", "", "Phantom equity: share in our success.", vocab.EquityWithSalary},
		{"stock awards", "", "Total compensation is made up of base pay, bonus, and stock awards.", vocab.EquityWithSalary},
		{"percent of equity", "", "Up to 0.5% equity, vested over four years.", vocab.EquityWithSalary},

		{"paid in equity and cash", "", "Compensated in equity and cash.", vocab.EquityWithSalary},
		{"paid in equity plus salary", "", "You will be paid in equity plus a competitive salary.", vocab.EquityWithSalary},
		{"explicit equity only before salary", "", "Paid only in equity at this stage. Salary starts after funding.", vocab.EquityOnly},

		// Equity is the whole pay.
		{"equity-only statement", "", "This is an equity-only role at this stage. There is no salary or cash compensation pre-funding.", vocab.EquityOnly},
		{"equity only title", "Founding Engineer (Equity Only Role)", "Compensation: 0.1% – 1.0%", vocab.EquityOnly},
		{"equity-based title", "Infrastructure Engineer (Equity-Based)", "Remote (global).", vocab.EquityOnly},
		{"equity-based internship", "", "This is an equity-based internship role. It is not a salaried or paid position.", vocab.EquityOnly},
		{"sweat equity position", "", "Please note this is a sweat equity only position before applying.", vocab.EquityOnly},
		{"no salary beside founder equity", "", "Founder equity and vesting. There is no salary before the seed round.", vocab.EquityOnly},
		// The explicit statement wins over a figure the board shows anyway: Wellfound
		// lists a range on postings that then say to disregard it.
		{"equity only despite a figure", "", "Compensation: ₹12L – ₹40L • 10.0% – 20.0%. Equity-only position: no cash salary until funding.", vocab.EquityOnly},

		// Not stated: other senses of the words, and near misses.
		{"DEI", "", "We are committed to diversity, equity and inclusion.", ""},
		{"DEI comma list", "", "Our Commitment to Equity, Diversity & Inclusion.", ""},
		{"pay equity", "", "Offers consider internal equity and pay equity across the team.", ""},
		{"private equity firm", "", "We are a private equity-backed company. Experience in private equity, M&A or banking.", ""},
		{"equity research", "", "Join our equity research team covering equity markets.", ""},
		{"health equity", "", "Advancing health equity in the communities we serve.", ""},
		{"vehicle equity", "", "Reimbursements can help you build equity in your vehicle over time.", ""},
		{"mail address", "", "Contact the District Equity Coordinator, 641-844-5523, equity@iavalley.edu.", ""},
		{"donation matching", "", "Competitive compensation and benefits, optional equity donation matching.", ""},
		{"client advice", "", "Advising clients from budgeting and benefits elections to equity compensation.", ""},
		{"equity options as product", "", "Marketing of options and derivatives including equity options.", ""},
		{"legal definition of wages", "", `Wages include "all forms of pay, including salary, overtime pay, bonuses, stock, stock options, profit sharing".`, ""},
		{"discounted purchase", "", "401K with match, discounted stock options, dependent day care, and more!", ""},
		{"ESPP is not equity", "", "Employee stock purchase plan (ESPP) with a 15% discount.", ""},
		{"stock associate", "Stock Associate", "Keep shelves stocked and share updates with the team.", ""},
		{"commission only is not equity only", "", "This is a commission-only role, no salary.", ""},
		{"unpaid internship", "Marketing Intern (Unpaid)", "This is an unpaid internship.", ""},
		{"stock options denied", "", "We do not offer stock options.", ""},
		{"modified equity grants denied", "", "We do not offer meaningful equity grants.", ""},
		{"no modified equity grants", "", "No meaningful equity grants are offered.", ""},
		{"no vesting period preserves offer", "", "Meaningful equity grants with no vesting period.", vocab.EquityWithSalary},
		{"equity grants denied", "", "Compensation does not include equity grants.", ""},
		{"RSUs denied", "", "No RSUs are offered for this role.", ""},
		{"equity denied after", "", "Stock options are not available.", ""},
		{"later offer survives denial", "", "No stock options. We offer RSUs instead.", vocab.EquityWithSalary},
		{"no equity", "", "Compensation: no equity. This is an unpaid internship. There is no salary.", ""},
		{"sweat equity studio", "", "We provide sweat equity ventures and pre-seed introductions.", ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := Equity(tt.title, tt.desc); got != tt.want {
				t.Errorf("Equity(%q, %q) = %q, want %q", tt.title, tt.desc, got, tt.want)
			}
		})
	}
}

// A posting's markup must not hide a phrase: the matcher reads visible text.
func TestEquity_ReadsVisibleText(t *testing.T) {
	if got := Equity("", "<ul><li><b>Competitive salary</b> + <i>equity</i></li></ul>"); got != vocab.EquityWithSalary {
		t.Errorf("Equity over markup = %q, want %q", got, vocab.EquityWithSalary)
	}
}
