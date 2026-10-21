package directapply

import (
	"fmt"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/applyform"
)

// Bounds on what a candidate sends. The application is delivered as an email, so these
// bound its size as much as the candidate's effort.
const (
	maxAnswer = 2000
	maxNote   = 2000
)

// Answer is one answered question as the employer receives it — the question's id, its
// wording and kind at the moment of applying, and the answer. A boolean answer is "true"
// or "false"; a select answer is one of the question's option values.
type Answer struct {
	ID     string              `json:"id"`
	Label  string              `json:"label"`
	Type   applyform.FieldType `json:"type"`
	Answer string              `json:"answer"`
}

// Readable is the answer as a person reads it: a boolean as "Yes"/"No", anything else as
// given. What the employer's email shows, and what the answer bank keeps — the bank also
// prefills ATS forms that ask in prose.
func (a Answer) Readable() string {
	if a.Type == applyform.TypeBoolean {
		return yesNo[a.Answer]
	}
	return a.Answer
}

var yesNo = map[string]string{"true": "Yes", "false": "No"}

// checkAnswers validates a candidate's answers, keyed by question id, against the vacancy's
// form and returns them in the form's order, each carrying a snapshot of its question. An
// unanswered optional question is left out. Every refusal names the question it is about.
func checkAnswers(form applyform.Form, answers map[string]string) ([]Answer, error) {
	known := make(map[string]bool, len(form.Fields))
	for _, f := range form.Fields {
		known[f.ID] = true
	}
	for id := range answers {
		if !known[id] {
			return nil, fmt.Errorf("%w: an answer to a question this vacancy does not ask", ErrInvalid)
		}
	}

	var out []Answer
	for _, f := range form.Fields {
		v := strings.TrimSpace(answers[f.ID])
		if v == "" {
			if f.Required {
				return nil, fmt.Errorf("%w: %q needs an answer", ErrInvalid, f.Label)
			}
			continue
		}
		if err := checkAnswer(f, v); err != nil {
			return nil, fmt.Errorf("%w: %q %s", ErrInvalid, f.Label, err)
		}
		out = append(out, Answer{ID: f.ID, Label: f.Label, Type: f.Type, Answer: v})
	}
	return out, nil
}

func checkAnswer(f applyform.Field, v string) error {
	switch f.Type {
	case applyform.TypeBoolean:
		if v != "true" && v != "false" {
			return fmt.Errorf("takes yes or no")
		}
	case applyform.TypeSelect:
		for _, o := range f.Options {
			if o.Value == v {
				return nil
			}
		}
		return fmt.Errorf("takes one of its listed options")
	default:
		if len([]rune(v)) > maxAnswer {
			return fmt.Errorf("takes at most %d characters", maxAnswer)
		}
	}
	return nil
}

// checkNote trims the candidate's optional note to the employer and bounds it.
func checkNote(note string) (string, error) {
	note = strings.TrimSpace(note)
	if len([]rune(note)) > maxNote {
		return "", fmt.Errorf("%w: the note takes at most %d characters", ErrInvalid, maxNote)
	}
	return note, nil
}
