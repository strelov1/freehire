package location

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// clearancePhrases are the anchored statements that a posting requires a government
// security clearance. They are tuned for PRECISION over recall on the same reasoning
// as usOnlyPhrases: a false positive hides a job the candidate could actually have
// got, while a miss merely leaves the status quo.
//
// Every UK entry carries a following noun. The scheme names themselves — "SC", "DV",
// "CTC", "L", "Q" — are one- and two-letter tokens that collide with ordinary words
// and unrelated initialisms ("an SC-registered charity", "the DV team"), which is
// exactly why eligibility.go declined to carry them for geography. Anchoring each to
// "clearance"/"cleared" buys the signal without the collision.
//
// "public trust" is listed only as "public trust clearance". Measured on the
// catalogue, the bare pair is overwhelmingly promotional copy — "commitment to public
// trust", "build public trust" — and only the full form names the US vetting tier.
// "clearance" alone is likewise absent: it carries unrelated senses (customs, medical,
// a billing "clearance specialist") that the labelled-field rule below distinguishes
// by structure rather than by the word.
var clearancePhrases = []string{
	// Scheme-neutral. "obtain a clearance" earns its place on measured evidence:
	// a live posting reading "will need to be able to obtain a clearance" named no
	// scheme anywhere and was missed by every other entry.
	"security clearance", "active clearance", "current clearance", "security-cleared",
	"clearance required", "government clearance",
	"obtain a clearance", "obtain a security clearance",
	// United Kingdom. "edv" (enhanced DV) is listed separately because the whole-word
	// match that stops "dv" matching inside ordinary words also stops it matching
	// inside "eDV".
	"sc clearance", "sc cleared", "dv clearance", "dv cleared", "edv clearance",
	"ctc clearance", "bpss", "security vetting", "developed vetting",
	"baseline personnel security standard",
	// United States. "top secret/sci" is its own entry: it is neither "ts/sci" nor
	// "top secret clearance", and live postings write it exactly this way.
	"secret clearance", "top secret clearance", "ts/sci", "top secret/sci",
	"sci clearance", "public trust clearance", "dod clearance", "polygraph",
	// Australia.
	"baseline clearance", "nv1", "nv2", "negative vetting", "positive vetting", "agsva",
}

// clearanceLabel matches the labelled-field form an ATS posting uses to state the
// requirement as structured text rather than prose: a "clearance" label, optionally
// qualified, then a separator, then the value. It captures the value so
// labelledClearanceAsserted can judge it.
//
// The value is bounded to one line and 60 bytes because these fields sit in a run of
// them ("CLEARANCE TYPE: Polygraph\nTRAVEL: 10%") and an unbounded capture would
// swallow the neighbours. A newline terminates it for the same reason.
//
// This rule exists because a phrase list alone cannot cover the form: the value varies
// without bound ("Secret", "TS with SCI eligibility", "Ability to Obtain Public
// Trust", "Yes"), while the structure is trivially recognisable. In the sampled
// catalogue rows it accounted for roughly a fifth of all true positives.
//
// The qualifier is a loose run of words rather than a fixed list, because postings
// write it every way: "Clearance Level", "Clearance Type", "Clearance Required for
// Start", "Clearance Level Must Currently Possess". Pinning the list missed the last
// of those on live data.
var clearanceLabel = regexp.MustCompile(`(?i)\bclearance(?:\s+[a-z]+){0,4}?\s*[:\-]\s*([^\n]{1,60})`)

// clearanceValues are the values a clearance label may carry that assert a real
// requirement: a scheme name, or a bare affirmation of the label itself.
var clearanceValues = []string{
	"secret", "ts", "sci", "public trust", "polygraph", "clearance",
	"baseline", "nv1", "nv2", "sc", "dv", "ctc", "bpss", "vetting",
	"yes", "required", "active", "obtain", "eligib",
}

// clearanceDenials are the leading words that deny the requirement outright. They are
// checked before clearanceValues, because "Clearance Required: No" contains the label's
// own affirmation and would otherwise assert itself.
//
// Every entry is a single leading word, because that is all valueDenies compares. A
// multi-word denial ("not required") is matched separately, against the whole value.
var clearanceDenials = []string{"no", "none", "n/a", "na", "nil"}

// RequiresClearanceFromDescription reports whether a job description states a
// government security-clearance requirement — UK SC/DV/BPSS, US Secret/TS-SCI/
// polygraph, AU baseline/NV1, or the scheme-neutral forms.
//
// It reads prose and structured ATS fields, dict-only: an unlisted phrase yields
// nothing rather than a guess, so false means "not stated" and never "stated as not
// required". The caller stores that as a tri-state (nil = unknown), because the
// dictionary cannot tell a posting that promises no clearance from one that is simply
// silent, and only a rounding error of the catalogue states the former.
//
// Being able to OBTAIN a clearance counts the same as holding one. Eligibility turns
// on nationality and residency history, so a candidate who cannot hold a clearance
// cannot obtain one either, and serving those postings as unrestricted would leave
// them in exactly the lane this signal exists to clear.
//
// The description is read as VISIBLE TEXT, not as markup. Descriptions are stored as
// HTML and tags routinely land between a label and its value — a live posting reading
// `<p><b>Security Clearance: </b></p>None/Not Required` put the denial out of reach of
// the label rule and marked a role that explicitly needs no clearance. Stripping first
// is the difference between reading what the posting says and reading how it is
// typeset.
//
// The two rules read the stripped text differently, which is why both forms are
// built here. The phrase rule keeps the line breaks, because negatedSentence treats
// them as sentence boundaries and losing them would let one paragraph's "not" cancel
// the next paragraph's requirement. The label rule needs them GONE, because stripping
// a `<p>` leaves the label and its value on separate lines and a same-line match would
// never see the value.
func RequiresClearanceFromDescription(desc string) bool {
	text := strings.ToLower(htmltext.ToText(desc))
	for _, p := range clearancePhrases {
		if phraseAssertedOutsideLabel(text, p) {
			return true
		}
	}
	return labelledClearanceAsserted(strings.Join(strings.Fields(text), " "))
}

// phraseAssertedOutsideLabel is phraseAsserted with one exclusion: an occurrence that
// is itself a field LABEL — immediately followed by a colon — asserts nothing, because
// what the posting requires is stated by the label's value, and the label rule reads
// that value.
//
// Without the exclusion, "Security Clearance: None/Not Required" marks itself. The
// denial cannot rescue it either: stripping the markup puts the value on the next
// line, and negatedSentence stops at the line break, so the phrase sees only its own
// label and reads as an unqualified assertion.
func phraseAssertedOutsideLabel(text, p string) bool {
	for offset := 0; ; {
		i := strings.Index(text[offset:], p)
		if i < 0 {
			return false
		}
		start := offset + i
		end := start + len(p)
		if wholeWordMatch(text, start, end) && !isFieldLabel(text, end) &&
			!negatedSentence(sentenceAround(text, start, end)) {
			return true
		}
		offset = end
	}
}

// isFieldLabel reports whether the phrase ending at end is immediately followed by a
// colon, making it a field label rather than prose. Emphasis markers are skipped:
// htmltext.ToText renders a bolded label as "*Security Clearance:*", so the colon sits
// behind an asterisk rather than against the phrase.
func isFieldLabel(text string, end int) bool {
	for end < len(text) && (text[end] == '*' || text[end] == ' ') {
		end++
	}
	return end < len(text) && text[end] == ':'
}

// labelledClearanceAsserted reports whether a "Clearance: <value>" field asserts a
// requirement. Every occurrence is examined rather than only the first: a posting may
// carry both a denial and a real field, and one unrecognised value must not hide a
// later recognised one.
// flat is the stripped text with every whitespace run folded to a single space, so a
// label and the value below it sit on one line.
func labelledClearanceAsserted(flat string) bool {
	for _, m := range clearanceLabel.FindAllStringSubmatch(flat, -1) {
		value := strings.TrimSpace(m[1])
		if valueDenies(value) {
			continue
		}
		for _, v := range clearanceValues {
			if containsWord(value, v) {
				return true
			}
		}
	}
	return false
}

// valueDenies reports whether a clearance field's value denies the requirement. The
// match is against the value's leading WORD so "None" denies while "Nonprofit sector
// experience" does not, and so a denial cannot be read out of the middle of a value
// that names a scheme.
//
// The word ends at the first non-letter, not at the first space: postings write the
// denial as "None/Not Required", and splitting on whitespace yields "none/not", which
// matches no entry and lets the label go on to assert itself.
func valueDenies(value string) bool {
	head := leadingWord(value)
	if head == "" {
		return true
	}
	for _, d := range clearanceDenials {
		if head == d {
			return true
		}
	}
	return strings.HasPrefix(value, "not required")
}

// containsWord reports whether v occurs in value on word boundaries. A plain substring
// test cannot be used: half of clearanceValues are two-letter scheme names that hide
// inside ordinary English — "sc" in "describe" and "discuss", "ts" in "contracts", "dv"
// in "advise" — so "Clearance: discuss with your recruiter" would assert a requirement
// out of a value that says nobody knows of one.
//
// "eligib" is the deliberate exception the trailing-boundary check must tolerate: it is
// a stem standing in for "eligible"/"eligibility", so only its leading boundary is
// meaningful. Listing it as a stem beats listing every inflection.
func containsWord(value, v string) bool {
	for offset := 0; ; {
		i := strings.Index(value[offset:], v)
		if i < 0 {
			return false
		}
		start := offset + i
		end := start + len(v)
		leadingOK := start == 0 || !isWordByte(value[start-1])
		trailingOK := v == "eligib" || end == len(value) || !isWordByte(value[end])
		if leadingOK && trailingOK {
			return true
		}
		offset = start + 1
	}
}

// leadingWord returns the run of letters that starts value, ignoring any leading
// punctuation. "n/a" is the one denial written with a separator inside it, so it is
// recognised from its "n" head plus the separator that follows.
func leadingWord(value string) string {
	start := 0
	for start < len(value) && !isWordByte(value[start]) {
		start++
	}
	end := start
	for end < len(value) && isWordByte(value[end]) {
		end++
	}
	head := value[start:end]
	if head == "n" && end < len(value) && strings.HasPrefix(value[end:], "/a") {
		return "n/a"
	}
	return head
}
