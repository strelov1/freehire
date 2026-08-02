package applyform

import (
	"bytes"
	"encoding/json"
	"strconv"
)

// platformValue decodes a JSON string OR a bare number, keeping the scalar token
// VERBATIM as the string value. Greenhouse genuinely sends both: an employer's own
// question enumerates its answers with numeric ids (724302231), while the compliance
// survey enumerates the same kind of answer with string ids ("3"). encoding/json aborts
// the whole unmarshal on the first type mismatch, so without this one compliance block
// would silently discard an entire captured form.
//
// Deliberately not an internal/flexjson type: that package coerces numerically and exists
// for LLM output, whereas this token is neither a number nor a guess — it is what the
// platform expects back on submission, and it has to survive as written. Same reasoning as
// internal/resumeextract's verbatimString, which the flexjson doc points to as the
// precedent for a package keeping its own scalar with different semantics.
type platformValue string

func (v *platformValue) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) == 0 || string(b) == "null" {
		*v = ""
		return nil
	}
	if b[0] == '"' {
		var s string
		if err := json.Unmarshal(b, &s); err != nil {
			return err
		}
		*v = platformValue(s)
		return nil
	}
	// A bare number (or any other scalar token) — kept exactly as written.
	*v = platformValue(b)
	return nil
}

// GreenhouseJob is the part of a Greenhouse job-board posting that describes its
// application form. It arrives only from the per-posting endpoint with questions=true —
// the board listing ignores that parameter, which is why a Greenhouse capture costs its
// own request.
type GreenhouseJob struct {
	// Questions are the employer's own.
	Questions []GreenhouseQuestion `json:"questions"`
	// Compliance are the legally mandated survey blocks (EEOC and friends), served
	// separately from the employer's questions and structured identically to them.
	Compliance []GreenhouseCompliance `json:"compliance"`
	// DemographicQuestions is the optional diversity survey — a THIRD shape, with no
	// fields wrapper and its answers under answer_options.
	DemographicQuestions *GreenhouseDemographic `json:"demographic_questions"`
	// LocationQuestions hold the controls a candidate never sees but an application is
	// rejected without: the hidden latitude/longitude pair beside the location text.
	LocationQuestions []GreenhouseQuestion `json:"location_questions"`
}

// GreenhouseQuestion is one labelled question, which may drive more than one control —
// "Resume/CV" offers an upload and a paste-the-text area under a single label, and each
// has its own submit name.
type GreenhouseQuestion struct {
	Label    string            `json:"label"`
	Required bool              `json:"required"`
	Fields   []GreenhouseField `json:"fields"`
}

// GreenhouseField is one control of a Greenhouse question. Name is the submit identifier.
type GreenhouseField struct {
	Name   string            `json:"name"`
	Type   string            `json:"type"`
	Values []GreenhouseValue `json:"values"`
}

// GreenhouseValue is one permitted answer.
type GreenhouseValue struct {
	Label string        `json:"label"`
	Value platformValue `json:"value"`
}

// GreenhouseCompliance is one legally mandated survey block.
type GreenhouseCompliance struct {
	Type      string               `json:"type"`
	Questions []GreenhouseQuestion `json:"questions"`
}

// GreenhouseDemographic is the optional diversity survey.
type GreenhouseDemographic struct {
	Header    string                          `json:"header"`
	Questions []GreenhouseDemographicQuestion `json:"questions"`
}

// GreenhouseDemographicQuestion is one survey question. Unlike every other Greenhouse
// question it carries its type and answers directly, with no fields wrapper.
type GreenhouseDemographicQuestion struct {
	ID       int64  `json:"id"`
	Label    string `json:"label"`
	Required bool   `json:"required"`
	Type     string `json:"type"`
	Options  []struct {
		ID    int64  `json:"id"`
		Label string `json:"label"`
	} `json:"answer_options"`
}

// greenhouseTypes maps Greenhouse's control types onto the normalized vocabulary. The set
// was observed across live postings; anything outside it deliberately has no entry, so a
// new control type surfaces as a field with a raw type and no normalized one rather than
// being filed under the nearest neighbour.
var greenhouseTypes = map[string]FieldType{
	"input_text":                TypeText,
	"textarea":                  TypeTextarea,
	"input_file":                TypeFile,
	"multi_value_single_select": TypeSelect,
	"multi_value_multi_select":  TypeMultiSelect,
	"input_hidden":              TypeHidden,
}

// FromGreenhouse captures the application form described by a Greenhouse posting.
func FromGreenhouse(job GreenhouseJob) Form {
	form := Form{Provider: "greenhouse"}

	form.Fields = append(form.Fields, greenhouseFields(job.Questions, "", false)...)

	for _, block := range job.Compliance {
		// A compliance block with no questions is a description with nothing to answer —
		// Greenhouse sends one on almost every posting.
		form.Fields = append(form.Fields, greenhouseFields(block.Questions, block.Type, true)...)
	}

	if d := job.DemographicQuestions; d != nil {
		for _, q := range d.Questions {
			// The survey's submit encoding is not the plain field name the rest of the
			// form uses, and it was not observed. The numeric question id is what the
			// platform identifies these by, and it is enough for the only thing anything
			// should do with them: count them, and decline to answer them.
			f := Field{
				ID:          strconv.FormatInt(q.ID, 10),
				Label:       q.Label,
				Type:        greenhouseTypes[q.Type],
				RawType:     q.Type,
				Required:    q.Required,
				Section:     d.Header,
				Demographic: true,
			}
			for _, opt := range q.Options {
				f.Options = append(f.Options, Option{
					Label: opt.Label,
					Value: strconv.FormatInt(opt.ID, 10),
				})
			}
			form.Fields = append(form.Fields, f)
		}
	}

	// Last, because they are last on the form and because nobody reading a capture wants
	// a hidden longitude before the employer's actual questions.
	form.Fields = append(form.Fields, greenhouseFields(job.LocationQuestions, "", false)...)

	return form
}

// greenhouseFields flattens questions into controls. One question may drive several, each
// inheriting the question's label and required flag — which is right, because the label is
// the thing the candidate is answering and the controls are just how it is collected.
func greenhouseFields(questions []GreenhouseQuestion, section string, demographic bool) []Field {
	var fields []Field
	for _, q := range questions {
		for _, c := range q.Fields {
			f := Field{
				ID:          c.Name,
				Label:       q.Label,
				Type:        greenhouseTypes[c.Type],
				RawType:     c.Type,
				Required:    q.Required,
				Section:     section,
				Demographic: demographic,
			}
			for _, v := range c.Values {
				f.Options = append(f.Options, Option{Label: v.Label, Value: string(v.Value)})
			}
			fields = append(fields, f)
		}
	}
	return fields
}
