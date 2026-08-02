package applyform

// WorkableSection is one headed group of controls in a Workable application form. The
// form arrives as a bare array of these from
// apply.workable.com/api/v1/jobs/{shortcode}/form — addressed by the posting shortcode
// alone, with no board or account, which is the second half of the stored external id.
type WorkableSection struct {
	Name   string          `json:"name"`
	Fields []WorkableField `json:"fields"`
}

// WorkableField is one control. A field of type "group" carries its own nested Fields —
// see FromWorkable for why those are not walked.
type WorkableField struct {
	ID       string           `json:"id"`
	Label    string           `json:"label"`
	Type     string           `json:"type"`
	Required bool             `json:"required"`
	Options  []WorkableOption `json:"options"`
	Fields   []WorkableField  `json:"fields"`
}

// WorkableOption is one permitted answer, and its two halves are named the OPPOSITE way
// round from every other platform captured here: the identifier is Name and the text a
// candidate reads is Value. Greenhouse, Recruitee and Ashby all mean the reverse by
// those words, so a mapping written by analogy would label every choice with a number
// and submit the sentence — wrong in both directions at once, and invisible until
// somebody read a captured form.
type WorkableOption struct {
	Name  string        `json:"name"`
	Value platformValue `json:"value"`
}

// workableTypes maps Workable's control types onto the normalized vocabulary. Measured
// across 40 live technical postings, counting nested fields: text 310, paragraph 126,
// date 86, boolean 81, multiple 65, file 63, group 40, email 37, phone 34, dropdown 26,
// number 15.
//
// `multiple` appeared in NONE of the first ten postings sampled. A mapper written from
// that sample would have silently dropped every multi-choice question — which is the
// shape of the most substantial things employers ask.
//
// email and phone map to plain text because the vocabulary names the KIND of control and
// both are text boxes; the validation hint survives in RawType, as Ashby's Email already
// does. `group` is absent on purpose — it is not a kind of answer, and FromWorkable
// treats it structurally.
var workableTypes = map[string]FieldType{
	"text":      TypeText,
	"email":     TypeText,
	"phone":     TypeText,
	"paragraph": TypeTextarea,
	"date":      TypeDate,
	"number":    TypeNumber,
	"boolean":   TypeBoolean,
	"file":      TypeFile,
	"dropdown":  TypeSelect,
	"multiple":  TypeMultiSelect,
}

// FromWorkable captures the application form described by a Workable posting.
func FromWorkable(sections []WorkableSection) Form {
	form := Form{Provider: "workable"}

	for _, section := range sections {
		for _, field := range section.Fields {
			// A group — Education, Experience — is a repeatable compound whose nested
			// fields are the parts of ONE entry. Walking them would say "this
			// application asks for your start date" where the true statement is "it
			// asks for your education history", so the group is kept whole and its
			// children are not visited.
			f := Field{
				ID:       field.ID,
				Label:    field.Label,
				Type:     workableTypes[field.Type],
				RawType:  field.Type,
				Required: field.Required,
				Section:  section.Name,
			}
			for _, opt := range field.Options {
				f.Options = append(f.Options, Option{
					Label: string(opt.Value),
					Value: opt.Name,
				})
			}
			form.Fields = append(form.Fields, f)
		}
	}

	return form
}
