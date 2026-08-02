package applyform

import (
	"slices"
	"strings"
)

// Display is a captured form shaped for a candidate to read rather than for a
// machine to submit. It is the near-inverse of what the store keeps: the store holds
// the platform's identifiers and option values because they exist to be handed back,
// and a reader wants none of that — only the question and one word about the answer.
type Display struct {
	// Provider is the platform the form was read from, so a reader can say where the
	// answer came from.
	Provider string `json:"provider"`
	// Basics are the controls every application demands — name, contact details, CV —
	// listed once instead of one entry each. Stated rather than dropped, because a
	// form that does NOT want a CV is worth knowing too.
	Basics []string `json:"basics"`
	// Questions are the employer's own, in the order the form presents them.
	Questions []Question `json:"questions"`
}

// Question is one thing the employer will ask.
type Question struct {
	// Text is the question as the employer wrote it, unedited.
	Text string `json:"text"`
	// Required is whether the platform refuses the application without an answer.
	Required bool `json:"required"`
	// Answer names the kind of answer expected, empty where naming it would add
	// nothing (a one-line answer is the default expectation) or where the capture
	// could not normalize the control's kind.
	Answer string `json:"answer,omitempty"`
}

// answerWords name each control kind for a reader. The absent entries are deliberate:
// a single-line answer is what anyone assumes, so naming it is noise, and a kind the
// capture left unnormalized gets no word rather than the nearest one — the same
// dict-only rule the capture follows, carried through to display.
var answerWords = map[FieldType]string{
	TypeTextarea:    "written answer",
	TypeSelect:      "choose one",
	TypeMultiSelect: "choose any",
	TypeBoolean:     "yes / no",
	TypeFile:        "upload",
}

// standardFieldIDs are the controls each platform puts on every form. They are
// identified by the identifiers OUR OWN mappers produce, not by guessing from labels:
// the mappers are deterministic, so this set is exact rather than heuristic.
//
// Ashby is absent because it needs no list — it prefixes every standard field's path
// with `_systemfield_`, which isStandard checks directly.
var standardFieldIDs = map[string][]string{
	"greenhouse": {
		"first_name", "last_name", "email", "phone",
		"resume", "resume_text", "cover_letter", "cover_letter_text",
		"location", "longitude", "latitude",
	},
	"recruitee": {
		"name", "email", "phone", "cv", "cover_letter", "photo", "salutation", "title",
	},
}

// isStandard reports whether a field is one every application demands rather than
// something this employer chose to ask.
func isStandard(provider string, f Field) bool {
	if strings.HasPrefix(f.ID, "_systemfield_") {
		return true
	}
	return slices.Contains(standardFieldIDs[provider], f.ID)
}

// ForDisplay projects a captured form into what a candidate reads. It is pure: no
// database, no network, so what it includes and what it refuses can be tested
// without a fixture of either.
func (f Form) ForDisplay() Display {
	d := Display{Provider: f.Provider}

	for _, field := range f.Fields {
		switch {
		// Neither is a question: one the platform fills itself and the candidate
		// never sees, and one that is a block of text the employer placed mid-form.
		case field.Type == TypeHidden || field.Type == TypeInfo:
			continue

		// Not the employer's questions — the platform's mandated diversity survey,
		// always optional and near-identical everywhere. Listing it would bury what
		// a candidate actually has to prepare for.
		case field.Demographic:
			continue

		case isStandard(f.Provider, field):
			// One label may cover several controls (Greenhouse offers the CV as an
			// upload OR pasted text under a single label), so the reader sees it once.
			if !slices.Contains(d.Basics, field.Label) {
				d.Basics = append(d.Basics, field.Label)
			}

		default:
			d.Questions = append(d.Questions, Question{
				Text:     field.Label,
				Required: field.Required,
				Answer:   answerWords[field.Type],
			})
		}
	}

	return d
}
