package applyform

import (
	"slices"
	"strconv"
)

// RecruiteeOffer is the part of a Recruitee offer that describes its application form.
// The board listing the ingest adapter already downloads carries all of it, so capturing
// a Recruitee form costs no request at all — the adapter simply stops discarding it.
//
// `dynamic_fields` is deliberately not decoded. It appears on every offer and was empty
// on every one of roughly 150 live offers across 25 boards, so its populated shape has
// never been observed — and a mapper written against a shape nobody has seen is a guess
// with a struct tag on it. Add it when an offer turns up carrying one.
type RecruiteeOffer struct {
	OpenQuestions []RecruiteeQuestion `json:"open_questions"`

	// The standard fields are configuration, not questions: each is "required",
	// "optional", or "off". There is no flag for name and email because the platform
	// refuses an application without them either way.
	OptionsCV          string `json:"options_cv"`
	OptionsCoverLetter string `json:"options_cover_letter"`
	OptionsPhone       string `json:"options_phone"`
	OptionsPhoto       string `json:"options_photo"`
	OptionsSalutation  string `json:"options_salutation"`
	OptionsTitle       string `json:"options_title"`
}

// RecruiteeQuestion is one employer-authored question on a Recruitee offer.
type RecruiteeQuestion struct {
	ID       int64                     `json:"id"`
	Position int                       `json:"position"`
	Required bool                      `json:"required"`
	Kind     string                    `json:"kind"`
	Body     string                    `json:"body"`
	Options  []RecruiteeQuestionOption `json:"open_question_options"`
}

// RecruiteeQuestionOption is one permitted answer to an enumerated Recruitee question.
// The submitted value is the option's numeric id, not the text the candidate reads.
type RecruiteeQuestionOption struct {
	ID       int64  `json:"id"`
	Position int    `json:"position"`
	Body     string `json:"body"`
}

// recruiteeKinds maps Recruitee's question kinds onto the normalized vocabulary. The set
// was measured across roughly 250 live offers rather than guessed, and a kind outside it
// deliberately has no entry: the dict-only rule says emit nothing for an unknown, so a
// new kind surfaces as a field with a raw type and no normalized one instead of being
// quietly filed under the nearest neighbour.
//
// `legal` is a consent checkbox and `infobox` is not a control at all — a block of text
// the employer dropped into the form. Both are captured, the second so that anything
// counting questions does not count it as one. `video` has no entry on purpose: nothing
// in the vocabulary describes recording a video answer, and inventing an entry would be
// the guess the rule forbids.
var recruiteeKinds = map[string]FieldType{
	"string":        TypeText,
	"text":          TypeTextarea,
	"single_choice": TypeSelect,
	"multi_choice":  TypeMultiSelect,
	"boolean":       TypeBoolean,
	"legal":         TypeBoolean,
	"file":          TypeFile,
	"date":          TypeDate,
	"number":        TypeNumber,
	"salary":        TypeNumber,
	"infobox":       TypeInfo,
}

// FromRecruitee captures the application form described by a Recruitee offer.
func FromRecruitee(o RecruiteeOffer) Form {
	// The platform's own validation response names these two whether or not the employer
	// configured anything, so they are the floor every Recruitee form stands on.
	fields := []Field{
		{ID: "name", Label: "Full name", Type: TypeText, RawType: "name", Required: true},
		{ID: "email", Label: "Email", Type: TypeText, RawType: "email", Required: true},
	}

	// The configurable standard fields, in the order the platform presents them. RawType
	// is the configuration key rather than a type word, because that key is the only name
	// Recruitee has for these controls.
	for _, sf := range []struct {
		id, label, key, flag string
		typ                  FieldType
	}{
		{"cv", "CV", "options_cv", o.OptionsCV, TypeFile},
		{"cover_letter", "Cover letter", "options_cover_letter", o.OptionsCoverLetter, TypeFile},
		{"phone", "Phone", "options_phone", o.OptionsPhone, TypeText},
		{"photo", "Photo", "options_photo", o.OptionsPhoto, TypeFile},
		{"salutation", "Salutation", "options_salutation", o.OptionsSalutation, TypeText},
		{"title", "Title", "options_title", o.OptionsTitle, TypeText},
	} {
		// "off" means the employer switched the control off, and an unset flag means the
		// platform did not mention it — neither is something to tell a candidate to
		// prepare for.
		if sf.flag != "required" && sf.flag != "optional" {
			continue
		}
		fields = append(fields, Field{
			ID:       sf.id,
			Label:    sf.label,
			Type:     sf.typ,
			RawType:  sf.key,
			Required: sf.flag == "required",
		})
	}

	// Recruitee carries an explicit position and does not promise to send the questions
	// in it, so the order the candidate will see is restored here rather than assumed.
	questions := slices.Clone(o.OpenQuestions)
	slices.SortStableFunc(questions, func(a, b RecruiteeQuestion) int { return a.Position - b.Position })

	for _, q := range questions {
		f := Field{
			ID:       strconv.FormatInt(q.ID, 10),
			Label:    q.Body,
			Type:     recruiteeKinds[q.Kind],
			RawType:  q.Kind,
			Required: q.Required,
		}
		for _, opt := range q.Options {
			f.Options = append(f.Options, Option{
				Label: opt.Body,
				Value: strconv.FormatInt(opt.ID, 10),
			})
		}
		fields = append(fields, f)
	}

	return Form{Provider: "recruitee", Fields: fields}
}
