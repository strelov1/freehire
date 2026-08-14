package location

import "strings"

// usOnlyPhrases are anchored, US-specific eligibility statements that mark a role
// as restricted to the United States. Like descriptionWorkModePhrases, they are
// tuned for PRECISION over recall in long prose: only phrases that essentially
// never appear outside a genuinely US-restricted posting are listed, so a bare
// "citizen", "secret", or "security" token cannot misfire. Citizenship phrases are
// unambiguous; the clearance phrases are US-specific terms of art ("Secret
// clearance", "TS/SCI") that other countries' vetting schemes do not use (the UK
// says "SC"/"DV", not "Secret clearance"), so generic "security clearance" is
// deliberately excluded to avoid mislabeling a UK/AU role as US.
var usOnlyPhrases = []string{
	"u.s. citizen", "us citizen", "united states citizen", "citizen of the united states",
	"u.s. citizenship", "us citizenship",
	"secret clearance", "ts/sci",
}

// USOnlyFromDescription reports whether a job description carries a hard US-only
// eligibility signal (US citizenship or a US security clearance). It reads prose,
// so it is a lowest-priority geography hint used only to rescue a job the location
// dictionary could not pin to a country (see jobderive): a bare-"Remote" posting
// that resolved to the global bucket but requires US citizenship is US-restricted,
// not open-anywhere. It never guesses — an absent phrase yields false.
//
// A phrase found in a sentence that also denies it ("does not require US citizenship",
// "no US citizenship required") is not a match: an unanchored Contains would read the
// denial as the assertion, which is the opposite mistake this function exists to avoid —
// the same precision-over-recall rule the phrase list itself follows.
func USOnlyFromDescription(desc string) bool {
	lower := strings.ToLower(desc)
	for _, p := range usOnlyPhrases {
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
		if !negatedSentence(sentenceAround(lower, start, end)) {
			return true
		}
		offset = end
	}
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
// trade this file already makes elsewhere — missing a real US-only signal costs less than
// mislabeling a globally-open role as US-restricted.
func negatedSentence(sentence string) bool {
	for _, word := range strings.Fields(sentence) {
		word = strings.Trim(word, ".,;:!?()\"'")
		if negationWords[word] || strings.HasSuffix(word, "n't") || strings.HasPrefix(word, "non-") {
			return true
		}
	}
	return false
}
