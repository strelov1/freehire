package applyform

// AshbyApplicationForm is the application form Ashby's job-board GraphQL API returns for
// one posting. The public posting API that ingest crawls carries no form at all, which is
// why an Ashby capture costs its own request.
//
// Retrieving it has one trap worth naming here, because it is invisible from the Go side:
// in Ashby's schema the entry's `field` is typed `JSON!`, so a GraphQL selection set on it
// fails the WHOLE query rather than that one field. It has to be requested as a scalar and
// decoded here.
type AshbyApplicationForm struct {
	Sections []AshbySection `json:"sections"`
}

// AshbySection is one headed group of controls.
type AshbySection struct {
	Title string `json:"title"`
	// IsHidden marks a section the candidate never sees.
	IsHidden     bool              `json:"isHidden"`
	FieldEntries []AshbyFieldEntry `json:"fieldEntries"`
}

// AshbyFieldEntry is one control's placement on this form: whether this form requires it,
// whether this form shows it, and the field descriptor itself.
type AshbyFieldEntry struct {
	// ID is scoped to the rendered form ("{formId}_{fieldId}") and is deliberately NOT
	// used as the captured identifier — see FromAshby.
	ID         string     `json:"id"`
	IsRequired bool       `json:"isRequired"`
	IsHidden   bool       `json:"isHidden"`
	Field      AshbyField `json:"field"`
}

// AshbyField is the control descriptor Ashby returns as an opaque JSON object.
type AshbyField struct {
	// Path is what Ashby's own client keys a submitted answer on — "_systemfield_email"
	// for a standard control, a bare uuid for an employer's question.
	Path             string        `json:"path"`
	Title            string        `json:"title"`
	Type             string        `json:"type"`
	SelectableValues []AshbyOption `json:"selectableValues"`
}

// AshbyOption is one permitted answer. The value is usually the label repeated, but not
// always, and it is decoded tolerantly for the same reason Greenhouse's is.
type AshbyOption struct {
	Label string        `json:"label"`
	Value platformValue `json:"value"`
}

// ashbyTypes maps Ashby's control types onto the normalized vocabulary. The set was
// measured across live postings; anything outside it deliberately has no entry.
//
// Email maps to plain text on purpose: the normalized vocabulary describes the KIND of
// control, and an email box is a text box — Greenhouse sends the same control as
// `input_text`. The validation hint is not lost, it survives in RawType.
//
// Location has no entry, and that is the dict-only rule doing its job rather than an
// omission: Ashby's location control is a place-autocomplete, which is neither a select
// (its values are not enumerated) nor free text (it will not accept arbitrary input), and
// there is no honest existing entry for it.
var ashbyTypes = map[string]FieldType{
	"String":           TypeText,
	"Email":            TypeText,
	"LongText":         TypeTextarea,
	"ValueSelect":      TypeSelect,
	"MultiValueSelect": TypeMultiSelect,
	"Boolean":          TypeBoolean,
	"File":             TypeFile,
}

// FromAshby captures the application form described by an Ashby posting.
func FromAshby(af AshbyApplicationForm) Form {
	form := Form{Provider: "ashby"}

	for _, section := range af.Sections {
		if section.IsHidden {
			continue
		}
		for _, entry := range section.FieldEntries {
			// A control the candidate never sees is not part of what applying costs.
			if entry.IsHidden {
				continue
			}
			// The field's path, not the entry's id: the id is scoped to one rendered form
			// and changes with it, while the path is the stable name an answer is
			// submitted under.
			f := Field{
				ID:       entry.Field.Path,
				Label:    entry.Field.Title,
				Type:     ashbyTypes[entry.Field.Type],
				RawType:  entry.Field.Type,
				Required: entry.IsRequired,
				Section:  section.Title,
			}
			for _, opt := range entry.Field.SelectableValues {
				f.Options = append(f.Options, Option{Label: opt.Label, Value: string(opt.Value)})
			}
			form.Fields = append(form.Fields, f)
		}
	}

	return form
}
