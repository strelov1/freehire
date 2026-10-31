package jobfacts

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/dict/vocab"
	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// Equity reports what a posting states about equity compensation: vocab.EquityOnly
// when equity is the whole pay, vocab.EquityWithSalary when equity is offered beside
// it, and "" when the posting says nothing the matcher can read as an equity offer.
// "" is "not stated", never "no equity": a posting that is silent and one that
// promises no equity read the same, and the second is a claim the catalogue almost
// never makes.
//
// Measured 2026-10-31 on 5,254 random open postings (tablesample, prod):
//
//   - The bare word is noise. "equity" occurs in 4.1% of them and roughly seven in ten
//     of those are another sense: diversity/pay/internal/health equity, private
//     equity firms, brand equity, equity research. So the word counts only in a
//     compensation context — next to a cash word ("salary + equity", "bonus and/or
//     equity"), as the head of a compensation noun ("equity grant", "equity
//     package"), after an offering verb ("all roles receive equity"), or under a
//     compensation adjective ("meaningful equity").
//   - The instruments are not: stock/share options, RSUs, ESOP, VSOP/virtual or phantom
//     shares, stock grants and awards each name an equity offer by themselves.
//   - Together they mark 2.0% of the sample (103 rows); reading every hit, 4 were wrong
//     before the exclusions below were added (an "equity@" mail address, a legal
//     definition of "wages" listing stock options, an "equity donation matching" perk,
//     a wealth manager advising clients on "equity compensation"). On a second,
//     held-out sample of 22,513 they mark 1.9% (422); of the ~145 hits read, one was
//     wrong ("equity options" marketed as a derivative, now excluded).
//   - 96 of the 106 also state cash pay in the text; the other ten are ordinary
//     salaried roles listing equity among benefits. Only 18 carry a stored salary
//     figure, so a salary FIGURE cannot be what "with salary" means — equity beside
//     an unstated salary is still equity beside a salary. What separates the two
//     values is the explicit equity-only statement, never the absence of a figure.
//
// An employee stock PURCHASE plan (ESPP) is deliberately not equity: the employee buys
// shares with their own pay at a discount, which is a savings benefit, not equity
// granted as compensation. It was the most frequent instrument in the sample (0.67%,
// mostly large retailers), so counting it would have made the facet mean "a big
// public company" more than "this job pays in equity".
func Equity(title, description string) string {
	text := equityText(htmltext.ToText(description))
	head := equityText(title)
	if equityOnlyStated(head) || equityOnlyStated(text) {
		return vocab.EquityOnly
	}
	if equityOffered(head + "\n" + text) {
		return vocab.EquityWithSalary
	}
	return ""
}

// equityText lowercases, drops the asterisks htmltext renders bold text as, and
// collapses the spaces inside each line, so "<b>salary</b> + equity" still reads as one
// phrase while a line break still ends a sentence.
func equityText(s string) string {
	s = strings.NewReplacer("’", "'", "*", "").Replace(strings.ToLower(s))
	lines := strings.Split(s, "\n")
	for i, line := range lines {
		lines[i] = strings.Join(strings.Fields(line), " ")
	}
	return strings.Join(lines, "\n")
}

// equityMod is the adjective a compensation sentence puts before "equity". "optional"
// is absent on purpose: the sampled "optional equity donation matching" is a charity perk.
const equityMod = `(?:meaningful|significant|generous|competitive|substantial|attractive|early[- ]stage|company|startup|founder|founding|target|annual|new[- ]hire|employee|real|potential|additional)`

var (
	// The compensation contexts that make the word "equity" an offer.
	reEquityAfterCash = regexp.MustCompile(`\b(?:salary|salaries|base(?: pay| salary)?|pay|compensation|cash|bonus(?:es)?|ote|commission|incentives?)\s*(?:\+|&|and/or|and|plus|,|/|or)\s*(?:an? )?(?:` + equityMod + `\s+)*equity\b`)
	// A figure directly before it: "£90k + equity", "$150,000 + equity". Only "+" — a
	// figure followed by a comma is a phone number as often as a salary.
	reEquityAfterFigure = regexp.MustCompile(`(?:[0-9]k?|[$£€])\s*\+\s*(?:` + equityMod + `\s+)*equity\b`)
	reEquityBeforeCash  = regexp.MustCompile(`\bequity\s*(?:\+|&|and|plus|,|/)\s*(?:an? )?(?:competitive |performance[- ]?(?:based|linked) )?(?:salary|base|bonus|cash|benefits|incentive)`)
	reEquityModified    = regexp.MustCompile(`\b` + equityMod + `\s+equity\b`)
	// "equity compensation" is not in the list: it is also the name of a product
	// category (equity compensation platforms) and of a financial-planning topic.
	// "equity options" is excluded below as another sense: a sampled banking role
	// marketed them as derivatives.
	reEquityNoun = regexp.MustCompile(`\bequity\s+(?:package|grants?|awards?|plan|program(?:me)?|component|stake|incentives?|refresh(?:ers?)?|ownership|participation|pool|allocation|upside|offer|opportunit(?:y|ies)|rewards?)\b`)
	// "up to 0.5% equity", "1.5% – 3.0% equity": a stake quoted as a share.
	reEquityPercent = regexp.MustCompile(`\b[0-9]+(?:\.[0-9]+)?\s?%\s*(?:[-–]\s*[0-9]+(?:\.[0-9]+)?\s?%\s*)?(?:of\s+)?equity\b`)
	reEquityVerb    = regexp.MustCompile(`\b(?:receive|receives|includes?|including|offers?|eligible for|participate in|get|earn)\s+(?:an? |the |our |company )?equity\b`)

	// The instruments, each an equity offer by itself.
	reEquityInstrument = regexp.MustCompile(`\b(?:stock|share)[- ]options?\b` +
		`|\besops?\b|\bemployee (?:stock|share) ownership\b` +
		`|\brsus?\b|\brestricted stock\b` +
		`|\bvsops?\b|\bvirtual (?:stock|share) options?\b|\bvirtual shares\b|\bvirtuelle (?:anteile|beteiligung)\b|\bphantom (?:equity|stock|shares?)\b` +
		`|\bbspce\b` +
		`|\bstock (?:grants?|awards?)\b|\b(?:stock|equity)[- ]based (?:long[- ]term )?(?:incentives?|awards?)\b`)

	// An equity token preceded by one of these is another sense of the word.
	// "diversity and equity" and "debt and equity" join with a conjunction; the other
	// senses modify the word directly ("pay equity", "private equity"), so a
	// conjunction after THEM is a compensation list ("pay and equity packages").
	reEquityOtherSenseBefore = regexp.MustCompile(`(?:\b(?:diversity|inclusion|debt)[\s,&-]*(?:and\s+)?|\b(?:pay|internal|health|racial|social|brand|home|employment|gender|digital|educational|education|departmental|peer|team|private|public|global|vehicle|negative|shareholders?'?|stockholders?'?)[\s-]+)$`)
	// ...or followed by one of these.
	reEquityOtherSenseAfter = regexp.MustCompile(`^(?:[\s,&-]*(?:and\s+)?(?:diversity|inclusion|access|belonging|justice)|\s*@|[\s-]+(?:partners?|firms?|funds?|investors?|investments?|backed|research|markets?|capital|derivatives|analysts?|traders?|trading|donations?|issuers?|fundraising|financing|mining|valuations?|transactions?|deals?|portfolio|securities|index|indices|holders?|line|loans?|options?|contributions?)\b)`)

	// A sentence that DEFINES pay for a legal notice — "wages include all forms of pay,
	// including ... stock options" — lists the instruments without offering any.
	reEquityLegalSentence = regexp.MustCompile(`all forms of (?:pay|compensation)|wage rates|"wages"|discriminat`)
	// "discounted stock options" is how a sampled retailer described its purchase plan.
	reEquityPurchasePlan = regexp.MustCompile(`discounted\s+$`)
	// Only governing denials: "not limited to" and "no vesting period" do not
	// deny the offer. Keep this local to each match so a later offer can still count.
	reEquityDeniedBefore = regexp.MustCompile(`(?:\b(?:no|without)\s+(?:(?:any|an?)\s+)?|\b(?:not|never|cannot|can't|don't|doesn't|won't)\s+(?:be\s+)?(?:offer|offered|offering|include|includes|including|provide|provided|providing|receive|eligible for)\s+(?:(?:any|an?)\s+)?)(?:` + equityMod + `\s+)*$`)
	reEquityDeniedAfter  = regexp.MustCompile(`^[\s:,()-]*(?:(?:is|are|will be)\s+)?(?:not (?:offered|available|included|provided)|unavailable)\b`)
)

var equityOfferRules = []*regexp.Regexp{
	reEquityAfterCash, reEquityAfterFigure, reEquityBeforeCash, reEquityModified, reEquityNoun, reEquityPercent, reEquityVerb,
}

// equityOffered reports whether the text states equity as part of the offer.
//
// The rules are alternations RE2 scans the whole body for, ~8ms a posting measured, so
// each family runs only when its anchor word occurs at all — "equity" is in ~4% of
// postings, and the instruments' stems in fewer.
func equityOffered(s string) bool {
	if strings.Contains(s, "equity") && equityWordOffered(s) {
		return true
	}
	for _, stem := range instrumentStems {
		if strings.Contains(s, stem) {
			return equityInstrumentOffered(s)
		}
	}
	return false
}

// instrumentStems holds a substring of every reEquityInstrument alternative.
var instrumentStems = []string{"stock", "share", "esop", "rsu", "vsop", "phantom", "virtuelle", "bspce", "equity"}

// equityWordOffered reports whether some use of the word "equity" is an offer.
func equityWordOffered(s string) bool {
	for _, re := range equityOfferRules {
		for _, loc := range re.FindAllStringIndex(s, -1) {
			if offerAt(s, loc, equityWordAt(s, loc)) {
				return true
			}
		}
	}
	return false
}

// equityInstrumentOffered reports whether an equity instrument is named as an offer.
func equityInstrumentOffered(s string) bool {
	for _, loc := range reEquityInstrument.FindAllStringIndex(s, -1) {
		if reEquityPurchasePlan.MatchString(s[max(0, loc[0]-12):loc[0]]) {
			continue
		}
		if offerAt(s, loc, -1) {
			return true
		}
	}
	return false
}

// equityWordAt returns where the word "equity" starts inside the match, or -1.
func equityWordAt(s string, loc []int) int {
	i := strings.Index(s[loc[0]:loc[1]], "equity")
	if i < 0 {
		return -1
	}
	return loc[0] + i
}

// offerAt rejects a match whose equity word is another sense of it, or whose sentence
// is a legal definition rather than an offer.
func offerAt(s string, loc []int, word int) bool {
	if reEquityDeniedBefore.MatchString(s[max(0, loc[0]-80):loc[0]]) ||
		reEquityDeniedAfter.MatchString(s[loc[1]:min(len(s), loc[1]+50)]) {
		return false
	}
	if word >= 0 {
		if reEquityDeniedBefore.MatchString(s[max(0, word-80):word]) {
			return false
		}
		if reEquityOtherSenseBefore.MatchString(s[max(0, word-40):word]) {
			return false
		}
		end := word + len("equity")
		if reEquityOtherSenseAfter.MatchString(s[end:min(len(s), end+30)]) {
			return false
		}
	}
	return !reEquityLegalSentence.MatchString(sentenceAround(s, loc[0]))
}

// The equity-only statements. Measured 2026-10-31 on 699 postings a search for
// equity-only phrasing returned (they are too rare for a random sample: 0 of 27,767
// random open postings carried one), the explicit phrases below were each read in
// context; the phrases dropped and why:
//
//   - "sweat equity" alone: "sweat equity ventures" is a studio's business model and
//     "equity and shares, LTIP, sweat equity etc" a CEO search preferring a salaried
//     hire. Only "sweat equity only/position/role/basis" stays.
//   - a title "(Equity)": as often a recruiter's "salary + equity" shorthand as an
//     unpaid founding role, with nothing in the title to tell them apart.
//   - "unpaid" and "commission-only": overwhelmingly internships, volunteering and
//     sales roles that offer no equity at all — and commission is cash.
var (
	reEquityOnlyPhrase = regexp.MustCompile(`\bequity[- ]only\b|\bonly equity\b|\bsweat equity (?:only|position|role|basis)\b` +
		`|\b(?:purely|solely|entirely|100%) equity\b|\b(?:paid|compensated) (?:entirely |solely |only |purely )?(?:in|with|through) equity\b` +
		`|\bequity[- ]based (?:role|position|internship|opportunity|founding|collaboration|participation|co-?founder|founder)` +
		`|\(equity[- ]based\)`)
	// An unqualified "paid in equity" can continue into an explicit mixed package.
	// Adverbs such as "only" or "entirely" retain their equity-only precedence.
	reEquityPaidWithCash = regexp.MustCompile(`^(?:paid|compensated) (?:in|with|through) equity\s*(?:and|plus|[+&])\s*(?:an? )?(?:competitive |base )?(?:cash|salary)\b`)
	// "not equity-only", "non equity-only" deny it; "long-only equity" and "equity only
	// funds" are an investment mandate (the other-sense check covers what follows).
	reEquityOnlyDenied = regexp.MustCompile(`(?:\bnot|\bno|\bnon|\blong|\bshort)[\s-]+(?:an? )?$`)
	// A posting that offers equity and states there is no cash pay. "no salary history"
	// and its siblings are pay-transparency boilerplate, not a statement about this role.
	reNoCashPay            = regexp.MustCompile(`\bno (?:cash |fixed |base )?(?:salary|compensation)\b|\bno cash\b|\bnot a (?:salaried|paid) (?:role|position)\b|\bwithout (?:a )?salary\b`)
	reNoCashPayBoilerplate = regexp.MustCompile(`^\s*(?:history|histories|requirements?|expectations?|information|range|ranges|cap|negotiation|disclosure|questions?)\b`)
)

// equityOnlyStated reports whether the text states that equity is the whole pay.
func equityOnlyStated(s string) bool {
	if !strings.Contains(s, "equity") {
		return false
	}
	for _, loc := range reEquityOnlyPhrase.FindAllStringIndex(s, -1) {
		if reEquityPaidWithCash.MatchString(s[loc[0]:]) {
			continue
		}
		if reEquityOnlyDenied.MatchString(s[max(0, loc[0]-12):loc[0]]) {
			continue
		}
		if reEquityOtherSenseAfter.MatchString(s[loc[1]:min(len(s), loc[1]+30)]) {
			continue
		}
		return true
	}
	// Commission is cash, so "no salary, commission only" is not equity-only even
	// when the posting also grants stock.
	if strings.Contains(s, "commission") {
		return false
	}
	for _, loc := range reNoCashPay.FindAllStringIndex(s, -1) {
		if reNoCashPayBoilerplate.MatchString(s[loc[1]:min(len(s), loc[1]+20)]) {
			continue
		}
		return equityWordOffered(s)
	}
	return false
}
