package applyform

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

// ProviderFreehire is the provider of a form no ATS published: the questions a verified
// employer wrote for a vacancy that takes applications on freehire. It is stored in
// apply_forms beside the captured ones so every reader of a form reads it the same way.
const ProviderFreehire = "freehire"

// Limits on what an employer may ask. A form is something a candidate has to fill in, so
// these bound the candidate's effort as much as the storage.
const (
	maxEmployerQuestions = 10
	maxQuestionLabel     = 300
	minSelectOptions     = 2
	maxSelectOptions     = 20
)

// ErrInvalidQuestions wraps every reason an employer's questions are refused. Its text
// after the colon is user-facing.
var ErrInvalidQuestions = errors.New("invalid questions")

// EmployerQuestion is one question as an employer writes it: a label, one of the three
// kinds a candidate can answer without an upload, and for a list its options.
type EmployerQuestion struct {
	Label    string
	Type     FieldType
	Required bool
	Options  []string
}

// FreehireForm validates an employer's questions and builds the form stored for the
// vacancy. Field ids are minted by position (q1, q2, …) — an answer names the question it
// answers by id, never by its wording, and the employer never chooses one. Labels and
// options are trimmed; an option's value is its label, since nothing downstream expects a
// separate id. No questions is a valid form: the employer asks for nothing but the CV.
func FreehireForm(questions []EmployerQuestion) (Form, error) {
	if len(questions) > maxEmployerQuestions {
		return Form{}, fmt.Errorf("%w: at most %d questions", ErrInvalidQuestions, maxEmployerQuestions)
	}
	form := Form{Provider: ProviderFreehire}
	for i, q := range questions {
		field, err := employerField(q)
		if err != nil {
			return Form{}, fmt.Errorf("%w: question %d: %s", ErrInvalidQuestions, i+1, err)
		}
		field.ID = "q" + strconv.Itoa(i+1)
		form.Fields = append(form.Fields, field)
	}
	return form, nil
}

func employerField(q EmployerQuestion) (Field, error) {
	label := strings.TrimSpace(q.Label)
	if label == "" {
		return Field{}, errors.New("the question is empty")
	}
	if len([]rune(label)) > maxQuestionLabel {
		return Field{}, fmt.Errorf("the question is longer than %d characters", maxQuestionLabel)
	}
	field := Field{Label: label, Type: q.Type, RawType: string(q.Type), Required: q.Required}
	switch q.Type {
	case TypeText, TypeBoolean:
		if len(q.Options) > 0 {
			return Field{}, errors.New("only a list question takes options")
		}
	case TypeSelect:
		options, err := employerOptions(q.Options)
		if err != nil {
			return Field{}, err
		}
		field.Options = options
	default:
		return Field{}, errors.New("the answer must be text, yes/no, or a choice from a list")
	}
	return field, nil
}

func employerOptions(raw []string) ([]Option, error) {
	if len(raw) < minSelectOptions || len(raw) > maxSelectOptions {
		return nil, fmt.Errorf("a list takes %d to %d options", minSelectOptions, maxSelectOptions)
	}
	seen := make(map[string]bool, len(raw))
	options := make([]Option, 0, len(raw))
	for _, o := range raw {
		o = strings.TrimSpace(o)
		if o == "" {
			return nil, errors.New("an option is empty")
		}
		if seen[o] {
			return nil, fmt.Errorf("the option %q appears twice", o)
		}
		seen[o] = true
		options = append(options, Option{Label: o, Value: o})
	}
	return options, nil
}
