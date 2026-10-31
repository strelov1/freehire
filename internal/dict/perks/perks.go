// Package perks reads the benefits a job description states it offers — paid time
// off, a 401(k) match, parental leave and the rest of vocab.PerkValues — for
// the `perks` search facet. Rule order is vocab.PerkValues order (a test holds it).
//
// Like the other dictionaries it is dict-only: a perk is emitted only when the text
// names it, never inferred from the employer, the country or the seniority. An empty
// result therefore means "the description mentions none of these", not "the job
// offers none of these" — most employers list only some of their benefits, and the
// filter's own copy says so.
//
// Every rule here was measured, not reasoned about. On 2026-10-31 two random samples
// of OPEN canonical postings (closed_at IS NULL AND duplicate_of IS NULL) were pulled
// from prod with TABLESAMPLE SYSTEM — 9,406 rows across the whole catalogue (5,103 of
// them English) and 2,342 is_tech rows — and every pattern was matched locally, with
// at least ten random hits per perk read by hand. The per-perk figures sit beside
// each rule as "N% of the catalogue / N% of technical postings". Meilisearch counts
// were not used: its quoted search is not a phrase search on this index.
//
// The sample also fixed the two guards every rule shares:
//
//   - A DENIAL. "Relocation Assistance: Not offered", "Sign-on Bonus: Not
//     Authorized", "the rate displayed does not include benefits such as health
//     insurance" — a phrase list alone marks each of these. Only a governing negation
//     counts (a "not" next to offer/provide/include/eligible…, or a "no"/"without"
//     directly in front), because the loose form ("any not in the sentence") threw
//     away "includes, but is not limited to, a 401(k)", "at no cost to you" and
//     "no vesting period", which were most of what it caught.
//   - A DUTY. "knowledge of health insurance benefits", "experience in
//     superannuation", "develop expertise in tuition assistance programs" — the perk's
//     words describing the WORK, not the package.
package perks

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// rule is one perk's matcher: any pattern asserts it, unless the match is denied, is a
// duty, or is followed by one of the perk's own disqualifying words.
type rule struct {
	slug string
	// label is the perk's display name. "Generous" is the employer's own word — the
	// rule asks for it — so the label repeats the employer's claim rather than making
	// one; parental leave carries no such word because any stated leave counts.
	label    string
	patterns []*regexp.Regexp
	// notAfter, when set, disqualifies a match the text right after it continues into
	// something that is not the perk ("health insurance portability" is HIPAA).
	notAfter *regexp.Regexp
}

// pto is what a time-off phrase may be called.
const pto = `(?:paid time[- ]off|time[- ]off|pto|vacation(?: time| days| policy)?|holidays?|holiday allowance|annual leave|leave policy|days off)`

// retirementAccount names the US tax-advantaged plans. The letter is required: a bare
// "401" or "457" is a suite number or a price.
const retirementAccount = `(?:401\s?\(?k\)?|403\s?\(?b\)?|457\s?\(?b\))`

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

var rules = []rule{
	// 4.6% / 6.6%. "Generous" is the employer's own word, so the chip states the
	// employer's claim — the rule asks for that word (or unlimited/flexible/
	// discretionary, the policies that have no cap) or a stated allowance at least as
	// large as a generous one in the markets that state a number: 25+ days, five or
	// more weeks, German "30 Tage Urlaub". A bare "paid time off" is NOT generous: it is
	// in a quarter of English postings and would make the chip mean nothing.
	{slug: "generous_pto", label: "Generous Paid Time Off", patterns: []*regexp.Regexp{
		re(`\b(?:generous|unlimited|uncapped|flexible|ample|plentiful|discretionary|enhanced)\s+(?:paid\s+)?` + pto + `\b`),
		re(`\b(?:2[5-9]|3\d)\+?\s*(?:working\s+|paid\s+)?days'?\s*(?:of\s+)?(?:paid\s+)?(?:annual leave|holidays?|holiday allowance|vacation|pto|paid time off|time off|leave)\b`),
		re(`\b(?:[5-9]|five|six|seven|eight)\s*weeks'?\s*(?:of\s+)?(?:paid\s+)?(?:annual leave|holidays?|vacation|pto|paid time off|time off)\b`),
		re(`\b(?:2[5-9]|3\d) (?:tage urlaub|urlaubstage)\b`),
	}},
	// 0.1% / 0.1%. Only the week itself: "30-hour work week" was tried and was a
	// part-time physician schedule three times in eight.
	{slug: "four_day_week", label: "Four Day Work Week", patterns: []*regexp.Regexp{
		re(`\b(?:4|four)[- ]day (?:work ?week|week|working week)\b`),
	}},
	// 7.2% / 5.5%. The match must sit in the same clause as the plan, either order.
	{slug: "401k_match", label: "401(k) Matching", patterns: []*regexp.Regexp{
		re(`\b` + retirementAccount + `[^.;•]{0,60}?\bmatch`),
		re(`\bmatch\w*[^.;•]{0,40}?\b` + retirementAccount),
	}},
	// 3.0% / 4.9%. Any stated parental leave, not only a "generous" one: outside the
	// statutory minimum the US does not have, offering it at all is the signal.
	{slug: "parental_leave", label: "Parental Leave", patterns: []*regexp.Regexp{
		re(`\b(?:parental|maternity|paternity|adoption)\s+leave\b|\bmaternity pay\b|\bshared parental\b`),
	}},
	// 17.3% / 15.7%. The international forms of the same perk: a UK/EU pension, an
	// Australian super, a Canadian RRSP, NZ KiwiSaver, an Indian provident fund, German
	// Altersvorsorge, Brazilian previdência privada.
	{slug: "retirement_plan", label: "Retirement Plan", patterns: []*regexp.Regexp{
		re(`\bretirement (?:plans?|savings|benefits?|contributions?|accounts?|programs?|package)\b|\bpensions?\b|\b` + retirementAccount + `|\bsuperannuation\b|\brrsp\b|\bkiwisaver\b|\bprovident fund\b|\baltersvorsorge\b|\bprevid[eê]ncia privada\b`),
	}},
	// 4.2% / 3.5%.
	{slug: "tuition_reimbursement", label: "Tuition Reimbursement", patterns: []*regexp.Regexp{
		re(`\btuition (?:reimbursement|assistance|support|aid|benefits?)\b|\beducation(?:al)? (?:assistance|reimbursement)\b`),
	}},
	// 0.3% / 0.8%. A subset of generous_pto, and a posting carries both.
	{slug: "unlimited_pto", label: "Unlimited PTO", patterns: []*regexp.Regexp{
		re(`\b(?:unlimited|uncapped)\s+(?:paid\s+)?` + pto + `\b`),
	}},
	// 14.9% / 15.3%. "Medical, Dental, Vision" is how most US postings state it, so the
	// medical-then-dental run counts. A French mutuelle or a Brazilian plano de saúde is
	// not here: both are near-universal by law or collective agreement and would not
	// narrow anything.
	{
		slug:  "health_insurance",
		label: "Health Insurance",
		patterns: []*regexp.Regexp{
			re(`\b(?:health|medical|healthcare) (?:insurance|coverage|cover|benefits?)\b|\bprivate (?:medical|health(?:care)?)\b|\b(?:medical|health)(?:,| and| &|/)\s*dental\b|\bseguro (?:m[eé]dico|de gastos m[eé]dicos)\b`),
		},
		notAfter: re(`^\s*(?:portability|claims?|billing|verification|compan|carriers?|industry)`),
	},
	// 0.7% / 0.8%. An Employee Assistance Program is NOT counted on its own: it is
	// benefits-list boilerplate at every large US employer. "mental health services"
	// is not either — on a clinic's posting it is the work.
	{slug: "mental_health", label: "Mental Health Support", patterns: []*regexp.Regexp{
		re(`\bmental health (?:support|benefits?|days?|resources|coverage|app|platform|stipend|counsel+ing)\b|\b(?:headspace|lyra health|modern health|spring health|talkspace)\b`),
	}},
	// 0.3% / 1.0%. A bare "development fund" was a UN agency and a Saudi government
	// programme in a sample of eight, so the kind of development is named.
	{slug: "learning_budget", label: "Learning Budget", patterns: []*regexp.Regexp{
		re(`\b(?:learning|training|education(?:al)?|professional development|personal development|l&d|learning (?:and|&) development|conference|cpd|continuing education) (?:budget|stipend|allowance|fund)\b`),
	}},
	// 0.1% / 0.4%. A plain "setup stipend" was an IN-office desk.
	{slug: "home_office_stipend", label: "Home Office Stipend", patterns: []*regexp.Regexp{
		re(`\b(?:home[- ]office|remote[- ]work(?:ing)?|work[- ]from[- ]home|wfh)\s(?:equipment |setup )?(?:stipend|allowance|budget|reimbursement)\b`),
	}},
	// 0.2% / 0.4%. The word reads the same in German and Dutch postings.
	{slug: "sabbatical", label: "Sabbatical", patterns: []*regexp.Regexp{
		re(`\bsabbaticals?\b`),
	}},
	// 0.9% / 0.3%. Mostly healthcare and the trades.
	{slug: "sign_on_bonus", label: "Sign-On Bonus", patterns: []*regexp.Regexp{
		re(`\b(?:sign[- ]?on|signing|joining) bonus(?:es)?\b`),
	}},
}

// denialBefore is a negation that governs what follows it in the same clause.
var denialBefore = re(`(?:\b(?:not|n't|unable to|cannot|ineligible)\s+(?:be\s+)?(?:offer|provid|include|including|eligible|qualif|sponsor|availab|authori)\w*[^.;:!?•·|]{0,40}$|\b(?:no|without)\s+$)`)

// denialAfter is a denial stated as the perk's value or predicate ("Relocation
// Assistance Not offered", "Sign-on Bonus: Not Authorized").
var denialAfter = re(`^[\s:\-–)]*(?:is |are |will |would |may )?(?:be )?(?:not\b|n't|no\b|none\b|unavailable)`)

// dutyBefore puts the perk's words in the job's duties or requirements.
var dutyBefore = re(`\b(?:administer\w*|administration of|reconcil\w*|audit\w*|processing|knowledge of|expertise in|experience (?:with|in)|calculat\w*)\b[^.;:!?•·|]{0,30}$`)

// beforeWindow and afterWindow bound the context a guard reads around a match.
const (
	beforeWindow = 80
	afterWindow  = 25
)

// Labels maps every perk slug to its display name, for the filter chips (generated
// into the SPA's contracts so the two cannot drift).
func Labels() map[string]string {
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		out[r.slug] = r.label
	}
	return out
}

// FromDescription returns the perks a job description states, in vocab.PerkValues
// order, or nil when it states none. The description is read as visible text, not as
// markup, so a tag between a label and its value cannot hide a denial.
func FromDescription(desc string) []string {
	if strings.TrimSpace(desc) == "" {
		return nil
	}
	// The text renderer marks bold as *word*; the asterisks would sit between a
	// qualifier and its noun ("*Unlimited* PTO"), so they go with the whitespace.
	text := strings.Join(strings.Fields(strings.ReplaceAll(htmltext.ToText(desc), "*", " ")), " ")
	var out []string
	for _, r := range rules {
		if r.asserted(text) {
			out = append(out, r.slug)
		}
	}
	return out
}

func (r rule) asserted(text string) bool {
	for _, p := range r.patterns {
		for _, loc := range p.FindAllStringIndex(text, -1) {
			before := text[max(0, loc[0]-beforeWindow):loc[0]]
			after := text[loc[1]:min(len(text), loc[1]+afterWindow)]
			if denialBefore.MatchString(before) || denialAfter.MatchString(after) ||
				dutyBefore.MatchString(before) {
				continue
			}
			if r.notAfter != nil && r.notAfter.MatchString(after) {
				continue
			}
			return true
		}
	}
	return false
}
