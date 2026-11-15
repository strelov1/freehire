package location

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// clearancePhrases are the anchored statements that a posting requires a government
// security clearance, tuned for PRECISION over recall as usOnlyPhrases is: a false
// positive hides a job the candidate could have got, a miss leaves the status quo.
//
// Every UK entry carries a following noun. The scheme names ("SC", "DV", "CTC", "L",
// "Q") collide with ordinary words and initialisms ("an SC-registered charity"), which
// is why eligibility.go declined to carry them for geography; anchoring each to
// "clearance"/"cleared" avoids the collision.
//
// "public trust" is listed only as "public trust clearance": the bare pair is mostly
// promotional copy ("commitment to public trust"). "clearance" alone is absent too, as
// it has unrelated senses (customs, medical, billing); the labelled-field rule below
// tells them apart by structure.
var clearancePhrases = []string{
	// Scheme-neutral. "obtain a clearance" is needed for postings that name no scheme.
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
// requirement as structured text: a "clearance" label, optionally qualified, a
// separator, then the value, which it captures for labelledClearanceAsserted.
//
// The value is bounded to one line and 60 bytes because these fields sit in a run
// ("CLEARANCE TYPE: Polygraph\nTRAVEL: 10%") and an unbounded capture would swallow
// the neighbours.
//
// A phrase list alone cannot cover the form: the value varies without bound ("Secret",
// "TS with SCI eligibility", "Yes"), while the structure is easy to recognise.
//
// The qualifier is a loose run of words, not a fixed list, because postings write it
// every way ("Clearance Level Must Currently Possess").
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
// nothing, so false means "not stated", never "stated as not required". The caller
// stores a tri-state (nil = unknown) because the dictionary cannot tell a posting that
// promises no clearance from a silent one.
//
// Being able to OBTAIN a clearance counts as holding one: eligibility turns on
// nationality and residency, so a candidate who cannot hold one cannot obtain one.
//
// The description is read as VISIBLE TEXT, not markup: tags routinely land between a
// label and its value (`<p><b>Security Clearance: </b></p>None/Not Required`).
//
// The two rules read the stripped text differently, so both forms are built here. The
// phrase rule keeps line breaks, because negatedSentence treats them as sentence
// boundaries and one paragraph's "not" must not cancel the next one's requirement. The
// label rule needs them GONE, because stripping a `<p>` leaves label and value on
// separate lines.
func RequiresClearanceFromDescription(desc string) bool {
	text := strings.ToLower(htmltext.ToText(desc))
	return requiresClearanceText(text, strings.Join(strings.Fields(text), " "))
}

// requiresClearanceText is RequiresClearanceFromDescription over text already stripped
// and lowercased, plus its whitespace-folded form, so ClearanceLevelsFromDescription can
// share one strip with it.
func requiresClearanceText(text, flat string) bool {
	text = clearanceFalseFriends.Replace(text)
	flat = clearanceFalseFriends.Replace(flat)
	for _, p := range clearancePhrases {
		if phraseAssertedOutsideLabel(text, p) {
			return true
		}
	}
	return labelledClearanceAsserted(flat)
}

// phraseAssertedOutsideLabel is phraseAsserted with one exclusion: an occurrence that
// is itself a field LABEL (immediately followed by a colon) asserts nothing, because
// the label's value states the requirement and the label rule reads that.
//
// Without it, "Security Clearance: None/Not Required" marks itself: stripping puts the
// value on the next line, negatedSentence stops at the line break, and the phrase
// reads as unqualified.
func phraseAssertedOutsideLabel(text, p string) bool {
	for offset := 0; ; {
		i := strings.Index(text[offset:], p)
		if i < 0 {
			return false
		}
		start := offset + i
		end := start + len(p)
		if wholeWordMatch(text, start, end) && !isFieldLabel(text, end) {
			if question, yes := questionField(text, end); question {
				if yes {
					return true
				}
			} else if !negatedSentence(sentenceAround(text, start, end)) {
				return true
			}
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

// questionField reports whether the phrase ending at end is the label of a
// question-form field ("Government Clearance Required? No", a Workday template) and
// whether its answer is yes. Such a field states the requirement only by its answer.
func questionField(text string, end int) (question, yes bool) {
	m := questionTail.FindStringSubmatch(text[end:])
	if m == nil || strings.HasPrefix(m[1], "?") {
		return false, false
	}
	return true, strings.HasPrefix(strings.TrimLeft(m[1], "* \t\n"), "yes")
}

// questionTail is the rest of a question-form label and its answer. htmltext renders a
// bolded label as "*Government Clearance Required?* No", so emphasis is skipped on
// both sides. A doubled "??" is not a question: it is how an emoji bullet survives a
// lossy feed ("?? Active SC clearance required ??").
var questionTail = regexp.MustCompile(`^(?:\s+[a-z]+){0,2}\s*\?([?*\s]*[a-z]*)`)

// clearanceFalseFriends are phrases that carry a clearance word without stating a
// security clearance (the Employee Polygraph Protection Act notice, tuberculosis "TB
// clearance", medical, drug-screen and customs fields), blanked before any rule reads
// the text.
var clearanceFalseFriends = strings.NewReplacer(
	"polygraph protection act", "                        ",
	"tb clearance", "            ",
	"medical clearance", "                 ",
	"customs clearance", "                 ",
	"drug screen clearance", "                     ",
	"health clearance", "                ",
)

// labelledClearanceAsserted reports whether a "Clearance: <value>" field asserts a
// requirement. Every occurrence is examined rather than only the first: a posting may
// carry both a denial and a real field, and one unrecognised value must not hide a
// later recognised one. flat is the stripped text with every whitespace run folded to a single space, so a
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

// valueDenies reports whether a clearance field's value denies the requirement. It
// matches the value's leading WORD so "None" denies while "Nonprofit sector
// experience" does not, and a denial cannot be read out of the middle of a value that
// names a scheme.
//
// The word ends at the first non-letter, not the first space: "None/Not Required"
// would otherwise split to "none/not", match no entry, and let the label assert itself.
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

// containsWord reports whether v occurs in value on word boundaries. A substring test
// would not do: half of clearanceValues are two-letter scheme names that hide inside
// ordinary words ("sc" in "describe", "ts" in "contracts", "dv" in "advise").
//
// "eligib" is the exception to the trailing-boundary check: it is a stem standing in
// for "eligible"/"eligibility", so only its leading boundary matters.
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
