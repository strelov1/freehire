package location

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/stringset"
)

// usOnlyPhrases are anchored, US-specific eligibility statements that mark a role
// as restricted to the United States. Like descriptionWorkModePhrases, they are
// tuned for PRECISION over recall in long prose: only phrases that essentially
// never appear outside a genuinely US-restricted posting are listed, so a bare
// "citizen", "secret", or "security" token cannot misfire. Citizenship phrases are
// unambiguous; the clearance phrases are US-specific terms of art ("Secret
// clearance", "TS/SCI") that other countries' vetting schemes do not use (the UK
// says "SC"/"DV", not "Secret clearance"), so generic "security clearance" is
// deliberately excluded to avoid mislabeling a UK/AU role as US.
// The work-authorization phrasings were the largest gap: "authorized to work in
// the United States" alone matches ~950 postings catalogue-wide, and in a sampled
// 53 of them 15 carried no resolved geography at all — exactly the bucket this
// rescue exists for. They read as requirements, not as the protected-characteristic
// lists in equal-opportunity boilerplate, which say "citizenship status" without
// naming a nationality and so cannot match these.
var usOnlyPhrases = []string{
	"u.s. citizen", "us citizen", "united states citizen", "citizen of the united states",
	"u.s. citizenship", "us citizenship", "united states citizenship",
	"authorized to work in the united states", "authorised to work in the united states",
	"authorization to work in the united states",
	"secret clearance", "ts/sci",
}

// euOnlyPhrases mark a role as restricted to the European Union. They pin a region
// and no country, because the restriction genuinely is region-level: a posting open
// to anyone holding EU work rights is not a posting in one member state, and
// enumerating 27 countries into the `countries` facet would claim a precision the
// text does not carry.
//
// This list is deliberately short because the catalogue barely uses these phrasings:
// a full-text sweep put "right to work in the EU" at 45 hits, of which exactly ONE
// carried the phrase verbatim. EU-restricted postings overwhelmingly state a member
// state instead, which the location dictionary already pins. Note that "no visa
// sponsorship" does NOT belong here — it says the employer will not file paperwork,
// not that only EU nationals may apply, and it has its own `visa_sponsorship` facet.
var euOnlyPhrases = []string{
	"right to work in the eu", "right to work in the european union",
	"eu work authorisation", "eu work authorization",
	"valid eu work permit",
}

// ukOnlyPhrases mark a role as restricted to the United Kingdom. This pins a country
// as well as a region, because the UK is one.
//
// "right to work in the UK" is the dominant form (~435 catalogue-wide). A sample of 35
// verbatim matches named no second area — no "UK or EU" phrasings — so pinning the UK
// alone does not quietly narrow a posting that was open wider.
//
// The UK's own clearance vocabulary ("SC", "DV", "BPSS") is deliberately absent: those
// are short tokens that collide with ordinary words and unrelated initialisms, and no
// longer anchoring form of them appeared often enough to earn a place.
var ukOnlyPhrases = []string{
	"right to work in the uk", "right to work in the united kingdom",
	"eligible to work in the uk", "authorised to work in the uk",
	"british citizen", "british citizenship",
}

// auOnlyPhrases mark a role as restricted to Australia. Australian government and
// defence contracts drive this: a sample of 44 verbatim matches had 16 with no resolved
// geography, the highest unpinned share of any area measured.
//
// Both noun forms are listed. Matching is whole-word (with a plural "s" allowed), so the
// shorter entry does NOT reach inside the longer one — that is what stops "uk" matching
// "Ukraine", and it costs an explicit variant here.
var auOnlyPhrases = []string{
	"australian citizen", "australian citizenship",
}

// caOnlyPhrases mark a role as restricted to Canada. Both forms are clean in the
// catalogue — 38 of 38 sampled "eligible to work in Canada" matches carried the phrase
// verbatim — though few of them are geographically unpinned, so this rule rescues less
// than the others. It is here because the phrasings are unambiguous, not because the
// volume is large.
var caOnlyPhrases = []string{
	"canadian citizen", "canadian citizenship", "eligible to work in canada",
}

// eligibilityRule pairs a set of anchored eligibility phrases with the geography an
// asserted phrase pins a posting to. Countries may be empty where the restriction is
// genuinely region-level (the EU); regions never are, because every rescue exists to
// move a posting OUT of the global bucket and a region-less result would not.
type eligibilityRule struct {
	countries []string
	regions   []string
	phrases   []string
}

// eligibilityRules is the full set, consulted in order. Order carries no priority —
// EligibilityFromDescription unions every rule that matches — but a stable list keeps
// its test table readable.
var eligibilityRules = []eligibilityRule{
	{countries: []string{"us"}, regions: []string{"north_america"}, phrases: usOnlyPhrases},
	{regions: []string{"eu"}, phrases: euOnlyPhrases},
	{countries: []string{"gb"}, regions: []string{"uk"}, phrases: ukOnlyPhrases},
	{countries: []string{"au"}, regions: []string{"apac"}, phrases: auOnlyPhrases},
	{countries: []string{"ca"}, regions: []string{"north_america"}, phrases: caOnlyPhrases},
}

// EligibilityFromDescription reports the geography a job description's eligibility
// statements restrict a posting to. It reads prose, so it is a lowest-priority
// geography hint used only to rescue a job the location dictionary could not pin
// (see jobderive): a bare-"Remote" posting that resolved to the global bucket but
// requires US citizenship is US-restricted, not open-anywhere. It never guesses — no
// asserted phrase yields two empty slices.
//
// Matching rules are UNIONED rather than resolved by priority. A posting saying "you
// must be authorized to work in the US or Canada" is genuinely open to both, and
// picking one would be a coin flip. Unioning is safe precisely because these are
// anchored eligibility statements and not place names: prose that merely lists offices
// in three countries asserts eligibility in none of them.
//
// A phrase found in a sentence that also denies it ("does not require US citizenship",
// "no US citizenship required") is not a match: an unanchored Contains would read the
// denial as the assertion, which is the opposite mistake this function exists to avoid —
// the same precision-over-recall rule the phrase lists themselves follow.
func EligibilityFromDescription(desc string) (countries, regions []string) {
	lower := strings.ToLower(desc)
	countrySet := make(map[string]struct{})
	regionSet := make(map[string]struct{})
	for _, rule := range eligibilityRules {
		if !ruleAsserted(lower, rule) {
			continue
		}
		for _, c := range rule.countries {
			countrySet[c] = struct{}{}
		}
		for _, r := range rule.regions {
			regionSet[r] = struct{}{}
		}
	}
	return stringset.Sorted(countrySet), stringset.Sorted(regionSet)
}

// ruleAsserted reports whether any one of a rule's phrases is asserted in lower.
func ruleAsserted(lower string, rule eligibilityRule) bool {
	for _, p := range rule.phrases {
		if phraseAsserted(lower, p) {
			return true
		}
	}
	return false
}

// negationWindow bounds how far a sentence boundary search looks in each direction from a
// phrase match, so one long unpunctuated block of text cannot pull an unrelated paragraph's
// wording into the check.
const negationWindow = 80

// sponsorshipTail matches "without sponsorship" and its common expansions. "without" is
// a negation word everywhere else, but in this construction it STRENGTHENS the
// eligibility requirement rather than denying it — "authorized to work in the US without
// sponsorship" is more restrictive, not less. The tail is stripped before the negation
// scan because the dominant US work-authorization phrasing in the catalogue carries it
// verbatim ("...without the need for current or future sponsorship"), so leaving it in
// would silently suppress the single highest-volume signal this file has.
var sponsorshipTail = regexp.MustCompile(
	`without\s+(?:the\s+need\s+for\s+|any\s+|requiring\s+)?(?:current\s+or\s+future\s+)?(?:visa\s+|employment\s+|immigration\s+)?sponsor\w*`)

// negationWords cancel the eligibility signal a phrase would otherwise assert. Checked as
// whole words (or, for "n't"/"non-", as a suffix/prefix of one) against the sentence the
// phrase sits in, not the phrase itself — "does not require US citizenship" denies the
// signal from three words away.
var negationWords = map[string]bool{
	"not": true, "no": true, "cannot": true, "never": true,
	"without": true, "except": true, "unless": true, "excluding": true,
}

// phraseAsserted reports whether p occurs in lower at least once outside a sentence that
// denies it. A phrase can appear more than once — a denied mention earlier in the text must
// not hide a genuine assertion later.
func phraseAsserted(lower, p string) bool {
	for offset := 0; ; {
		i := strings.Index(lower[offset:], p)
		if i < 0 {
			return false
		}
		start := offset + i
		end := start + len(p)
		if wholeWordMatch(lower, start, end) && !negatedSentence(sentenceAround(lower, start, end)) {
			return true
		}
		offset = end
	}
}

// wholeWordMatch reports whether lower[start:end] sits on word boundaries, so a phrase
// cannot match inside a longer word. Without it "right to work in the uk" matches "right
// to work in the Ukraine" and files a Ukrainian posting under the United Kingdom.
//
// A single trailing "s" is allowed, because the plural is how these statements are
// usually written ("United States citizens", "Australian citizens") and the phrase lists
// carry the singular. Nothing longer is: "us citizen" must NOT swallow "US citizenship",
// which is a separate list entry that matches itself exactly.
func wholeWordMatch(lower string, start, end int) bool {
	if start > 0 && isWordByte(lower[start-1]) {
		return false
	}
	if end < len(lower) && lower[end] == 's' {
		end++
	}
	return end == len(lower) || !isWordByte(lower[end])
}

// isWordByte reports whether b continues a word. ASCII-only is enough: every eligibility
// phrase is ASCII, so only an ASCII neighbour can extend one into a longer word.
func isWordByte(b byte) bool {
	return b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9'
}

// sentenceAround returns the text around lower[start:end], trimmed to the nearest sentence
// boundary (., !, ?, or newline) on each side, capped at negationWindow bytes in each
// direction.
func sentenceAround(lower string, start, end int) string {
	lo := start - negationWindow
	if lo < 0 {
		lo = 0
	}
	if i := strings.LastIndexAny(lower[lo:start], ".!?\n"); i >= 0 {
		lo += i + 1
	}
	hi := end + negationWindow
	if hi > len(lower) {
		hi = len(lower)
	}
	if i := strings.IndexAny(lower[end:hi], ".!?\n"); i >= 0 {
		hi = end + i
	}
	return lower[lo:hi]
}

// negatedSentence reports whether a sentence carries a negation word, checked whole-word so
// "notable" or "cannot deploy without approval" style incidental substrings of unrelated
// words don't misfire the way an unanchored Contains would.
//
// The check is sentence-wide, not clause-scoped to the phrase: a sentence that negates
// something else entirely ("Applicants must not have unresolved conflicts and must be a US
// Citizen") suppresses the match too. That is the safe side of the same precision-over-recall
// trade this file already makes elsewhere — missing a real eligibility signal costs less than
// mislabeling a globally-open role as restricted.
func negatedSentence(sentence string) bool {
	sentence = sponsorshipTail.ReplaceAllString(sentence, " ")
	for _, word := range strings.Fields(sentence) {
		word = strings.Trim(word, ".,;:!?()\"'")
		if negationWords[word] || strings.HasSuffix(word, "n't") || strings.HasPrefix(word, "non-") {
			return true
		}
	}
	return false
}
