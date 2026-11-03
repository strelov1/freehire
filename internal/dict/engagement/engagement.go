// Package engagement reads the legal terms a job posting states it engages its worker
// on — vocab.EngagementValues, the `engagement` search facet. Today that is the UK's
// IR35 status: whether HMRC's off-payroll rules treat the contract as employment
// (Inside IR35, taxed like a salary through an umbrella or PAYE) or as a business
// supplying a service (Outside IR35, the contractor's own limited company). For a UK
// contractor it is the first filter, because the same day rate is a different income
// on either side of it.
//
// Like the other dictionaries it is dict-only: a value is emitted only when the text
// states it, never inferred from the rate, the client or the sector. An empty result
// means "the posting does not say", and a posting that states a status as still to be
// decided ("IR35 determination pending") says nothing yet.
//
// Every rule was measured, not reasoned about. On 2026-11-03 the 1,354 postings the
// public search returns for "IR35" (1,132 of them British; reed, whatjobs-uk and
// adzuna carry most) were matched locally and every wording around the name read by
// hand. The status is written three ways, and each is a rule below: the status
// before the name ("Inside IR35", "outside of IR35", "within IR35", "in-scope IR35"),
// the name as a field label with the status as its value ("IR35: Inside", "IR35
// Status: Outside", "IR35 status is outside"), and a hashtag. 886 state inside, 435
// outside, one both; the 32 left state neither — HR and tax roles about IR35
// compliance itself, a pending determination, a client exempt from the rules, a bare
// "Contract - IR35", and postings in other languages the search matched loosely. A
// random 57 of the tagged ones were read by hand; all 57 were right.
package engagement

import (
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/platform/htmltext"
)

// rule is one value's matcher: any pattern asserts it unless a negation governs it.
type rule struct {
	slug, label string
	patterns    []*regexp.Regexp
}

// ir35 is the name as postings spell it: "IR35", "IR 35", "IR-35". What follows it
// may not be a digit, which keeps a part number like "IR350" out, but may be a letter:
// a feed that dropped the markup between two lines glues the next word on ("Outside
// IR35We are looking", "Inside IR35November start") — three postings in the sample.
const ir35 = `ir\s?-?35(?:\b|[^0-9])`

// field is the name used as a field label, with the room a label leaves before its
// value: "IR35: Inside", "IR35 Status: Inside", "IR35 status is outside", "IR35 -
// Outside", "IR35 Inside".
const field = `\b` + ir35 + `(?:\s+(?:status|determination|classification))?\s*(?:[:\-–—]\s*)?(?:is\s+)?(?:deemed\s+)?`

// scope is what may sit between a status word and the name: "inside the scope of
// IR35", "outside of IR35".
const scope = `\s*(?:of\s+)?(?:the\s+)?(?:scope\s+of\s+)?`

// either joins two statuses offered side by side: "Inside or Outside IR35", one
// posting in the sample, which states both.
const either = `\s*(?:or|and|/|&)\s*`

func re(s string) *regexp.Regexp { return regexp.MustCompile(`(?i)` + s) }

var rules = []rule{
	{slug: "ir35_outside", label: "Outside IR35", patterns: []*regexp.Regexp{
		// "outsdie" is a misspelling read in the sample; "out of scope" is HMRC's own
		// wording for the same determination.
		re(`\b(?:outside|outsdie|out[- ]of[- ]scope)` + scope + ir35),
		re(field + `(?:outside|out[- ]of[- ]scope)\b`),
		re(`#outsideir35\b`),
		// "Outside or Inside IR35": the name follows the second status only.
		re(`\boutside` + either + `inside` + scope + ir35),
	}},
	{slug: "ir35_inside", label: "Inside IR35", patterns: []*regexp.Regexp{
		// "Umbrellainside IR35" is a template that glued the two words; it is the
		// umbrella company the inside status implies, so the glued form still counts.
		re(`(?:\bumbrella|\b)(?:inside|within|in[- ]scope)` + scope + ir35),
		re(field + `(?:inside|in[- ]scope)\b`),
		re(`#insideir35\b`),
		re(`\binside` + either + `outside` + scope + ir35),
	}},
}

// negated rejects a status a negation governs: "this role is not inside IR35".
var negated = re(`\b(?:not|isn'?t|never)\s+(?:be\s+)?$`)

// beforeWindow is how far back a negation is looked for.
const beforeWindow = 12

// Labels maps every value to its display name, for the filter chips (generated into
// the SPA's contracts so the two cannot drift).
func Labels() map[string]string {
	out := make(map[string]string, len(rules))
	for _, r := range rules {
		out[r.slug] = r.label
	}
	return out
}

// FromText returns the engagement terms a posting's title and description state, in
// vocab.EngagementValues order, or nil when they state none. Both are read: a
// recruiter's title carries the status about as often as the body does. The
// description is read as visible text, so a tag between a field label and its value
// cannot hide the value.
func FromText(title, description string) []string {
	// A full stop between the two keeps a title's last word and the body's first from
	// reading as one phrase.
	text := strings.TrimSpace(title) + ". " + htmltext.ToText(description)
	// The text renderer marks bold as *word*; the asterisks would sit between a field
	// label and its value ("*IR35:* Inside"), so they go with the whitespace.
	text = strings.Join(strings.Fields(strings.ReplaceAll(text, "*", " ")), " ")
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
			if !negated.MatchString(text[max(0, loc[0]-beforeWindow):loc[0]]) {
				return true
			}
		}
	}
	return false
}
