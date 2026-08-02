// Package applyform captures the application form an ATS published for a job — the
// questions a candidate would have to answer, and the identifiers the platform expects
// back when they do.
//
// The package keeps the platform's own vocabulary. Field identifiers, option values and
// question text are stored exactly as the platform returned them, unnormalized, which is
// the opposite of what internal/skilltag and internal/classify do and is deliberate:
// those dictionaries exist to make facets comparable ACROSS platforms, whereas a form
// field's identifier is not for comparing. `question_67165648` means nothing except to
// Greenhouse, and it exists to be handed back to Greenhouse. Any mapping of it is loss.
//
// The store has one reader: display.go projects a captured form into what a candidate
// sees on the job page, which wants almost none of the above — the question text and one
// word about the answer. The verbatim identifiers exist for the consumer that has not
// been built yet, the one that fills a form rather than describing it.
//
// The one thing that IS normalized is the kind of control, because "is this a dropdown"
// is a question every consumer asks and no consumer should answer three times. That
// mapping follows the repository's dict-only rule: a word the dictionary does not know
// yields no normalized type at all rather than a guess, and the platform's own word is
// kept alongside so nothing is lost.
package applyform

// FieldType is the normalized kind of a form control — the small controlled vocabulary
// three platforms' own type names map onto. It is empty when a platform describes a
// control with a word this package does not recognize.
type FieldType string

const (
	// TypeText is a single-line free-text answer.
	TypeText FieldType = "text"
	// TypeTextarea is a long-form written answer — the expensive kind, the one that
	// turns an application into an evening's work.
	TypeTextarea FieldType = "textarea"
	// TypeSelect is one answer chosen from an enumerated list.
	TypeSelect FieldType = "select"
	// TypeMultiSelect is any number of answers chosen from an enumerated list.
	TypeMultiSelect FieldType = "multiselect"
	// TypeBoolean is a yes/no answer or a consent checkbox.
	TypeBoolean FieldType = "boolean"
	// TypeFile is an upload — a CV, a cover letter, a portfolio.
	TypeFile FieldType = "file"
	// TypeDate is a calendar date, typically an availability date.
	TypeDate FieldType = "date"
	// TypeNumber is a numeric answer, typically years of experience or a salary figure.
	TypeNumber FieldType = "number"
	// TypeHidden is a control the platform fills itself and the candidate never sees.
	// It is captured because submitting without it fails, not because anyone reads it.
	TypeHidden FieldType = "hidden"
	// TypeInfo is not an answerable control at all — a block of text the employer put in
	// the middle of the form. Captured so a consumer counting questions does not count
	// it as one.
	TypeInfo FieldType = "info"
)

// Form is one job's application form as a single platform published it.
type Form struct {
	// Provider is the ATS this was read from, in the same vocabulary as jobs.source.
	Provider string `json:"provider"`
	// Fields are the form's controls in the order the platform presented them. A form
	// with none is still meaningful: it says this employer asks for nothing beyond the
	// standard fields.
	Fields []Field `json:"fields,omitempty"`
}

// Field is one control of an application form.
type Field struct {
	// ID is what the platform calls this control when an application is submitted —
	// Greenhouse's `question_67165648`, Ashby's field path, Lever's `cards[uuid][field0]`.
	// Opaque by design; it is a token to hand back, not a name to interpret.
	ID string `json:"id"`
	// Label is the question as the employer wrote it.
	Label string `json:"label"`
	// Type is the normalized control kind, empty when RawType is a word the dictionary
	// does not know.
	Type FieldType `json:"type,omitempty"`
	// RawType is the platform's own word for the control — its type name where it has
	// one, its configuration key where the control is standard and typeless. Always
	// kept: it is the evidence behind Type, and the only record left when Type is empty.
	RawType string `json:"raw_type"`
	// Required is whether the platform refuses the application without an answer.
	Required bool `json:"required"`
	// Section is the heading the platform grouped this control under, empty where it
	// groups nothing. Kept because on a long form the heading is often what makes an
	// otherwise ambiguous question answerable.
	Section string `json:"section,omitempty"`
	// Demographic marks a control the PLATFORM ITSELF classifies as an equal-opportunity
	// or diversity question — Greenhouse serves these in a separate block from the
	// employer's own questions, and that separation is a fact worth keeping rather than
	// flattening away. Nothing here acts on it; it exists so that whatever eventually
	// answers a form can decline to answer these, which is a boundary a consumer must
	// not have to rediscover by reading question text.
	Demographic bool `json:"demographic,omitempty"`
	// Options are the permitted answers where the platform enumerates them, each with the
	// value the platform expects rather than the label it displays. The two differ often
	// enough — Greenhouse sends numeric ids behind human labels — that storing only one
	// would make the capture unusable.
	Options []Option `json:"options,omitempty"`
}

// Option is one permitted answer to an enumerated control.
type Option struct {
	// Label is what the candidate reads.
	Label string `json:"label"`
	// Value is what the platform expects to receive. Equal to Label on platforms that
	// submit the text itself; a numeric id on those that do not.
	Value string `json:"value"`
}
