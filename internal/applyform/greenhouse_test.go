package applyform

import (
	"encoding/json"
	"testing"
)

// The payload is decoded from what the platform actually sends, so the fixtures here are
// raw JSON rather than hand-built structs — a struct literal would silently pass a decode
// bug straight through, and the option value's type is exactly where such a bug lives.
const greenhouseFixture = `{
  "questions": [
    {"label": "First Name", "required": true,
     "fields": [{"name": "first_name", "type": "input_text", "values": []}]},
    {"label": "Resume/CV", "required": false,
     "fields": [{"name": "resume", "type": "input_file", "values": []},
                {"name": "resume_text", "type": "textarea", "values": []}]},
    {"label": "Will you require sponsorship?", "required": true,
     "fields": [{"name": "question_67165648", "type": "multi_value_single_select",
                 "values": [{"label": "Yes", "value": 724302231},
                            {"label": "No", "value": 724302232}]}]},
    {"label": "Which countries?", "required": true,
     "fields": [{"name": "question_67165646[]", "type": "multi_value_multi_select",
                 "values": [{"label": "France", "value": 733222805}]}]}
  ],
  "compliance": [
    {"type": "eeoc", "questions": []},
    {"type": "eeoc", "questions": [
      {"label": "DisabilityStatus", "required": false,
       "fields": [{"name": "disability_status", "type": "multi_value_single_select",
                   "values": [{"label": "I do not want to answer", "value": "3"}]}]}
    ]}
  ],
  "demographic_questions": {
    "header": "U.S. Standard Demographic Questions",
    "questions": [
      {"id": 19936, "label": "How would you describe your gender identity?",
       "required": false, "type": "multi_value_multi_select",
       "answer_options": [{"id": 121886, "label": "Man"}, {"id": 121885, "label": "Non-binary"}]}
    ]
  },
  "location_questions": [
    {"label": "Longitude", "required": true,
     "fields": [{"name": "longitude", "type": "input_hidden", "values": []}]},
    {"label": "Location", "required": true,
     "fields": [{"name": "location", "type": "input_text", "values": []}]}
  ]
}`

func decodeGreenhouse(t *testing.T) Form {
	t.Helper()
	var job GreenhouseJob
	if err := json.Unmarshal([]byte(greenhouseFixture), &job); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return FromGreenhouse(job)
}

func TestFromGreenhouseEmployerQuestions(t *testing.T) {
	form := decodeGreenhouse(t)

	if form.Provider != "greenhouse" {
		t.Errorf("provider = %q, want %q", form.Provider, "greenhouse")
	}

	t.Run("the submit name is the identifier", func(t *testing.T) {
		got := fieldByID(t, form, "first_name")
		if got.Label != "First Name" || got.Type != TypeText || !got.Required {
			t.Errorf("first_name = %+v, want the labelled required text control", got)
		}
	})

	t.Run("a question with two controls yields two fields", func(t *testing.T) {
		// Greenhouse offers Resume/CV as an upload OR pasted text under one label; both
		// are real controls with their own submit names.
		file := fieldByID(t, form, "resume")
		text := fieldByID(t, form, "resume_text")
		if file.Type != TypeFile || text.Type != TypeTextarea {
			t.Errorf("resume pair = (%q, %q), want (%q, %q)", file.Type, text.Type, TypeFile, TypeTextarea)
		}
		if file.Label != "Resume/CV" || text.Label != "Resume/CV" {
			t.Errorf("both controls should carry the question's label, got %q and %q", file.Label, text.Label)
		}
	})

	t.Run("numeric option values survive as the platform sent them", func(t *testing.T) {
		got := fieldByID(t, form, "question_67165648")
		if got.Type != TypeSelect {
			t.Errorf("type = %q, want %q", got.Type, TypeSelect)
		}
		want := []Option{{Label: "Yes", Value: "724302231"}, {Label: "No", Value: "724302232"}}
		if len(got.Options) != 2 {
			t.Fatalf("options = %v, want %v", got.Options, want)
		}
		for i, o := range got.Options {
			if o != want[i] {
				t.Errorf("option %d = %v, want %v", i, o, want[i])
			}
		}
	})

	t.Run("a multi-select keeps its bracketed submit name", func(t *testing.T) {
		got := fieldByID(t, form, "question_67165646[]")
		if got.Type != TypeMultiSelect {
			t.Errorf("type = %q, want %q — the trailing [] is part of the name, not decoration",
				got.Type, TypeMultiSelect)
		}
	})
}

// Greenhouse sends its equal-opportunity survey in two blocks, both structurally distinct
// from the employer's own questions and both legally distinct in what may be done with
// them. Collapsing that distinction would leave a consumer to rediscover it by reading
// question text, which is not a boundary anything should be guessing at.
func TestFromGreenhouseMarksSurveyQuestions(t *testing.T) {
	form := decodeGreenhouse(t)

	t.Run("a compliance question is marked and keeps its string option value", func(t *testing.T) {
		got := fieldByID(t, form, "disability_status")
		if !got.Demographic {
			t.Error("compliance question not marked demographic")
		}
		if len(got.Options) != 1 || got.Options[0].Value != "3" {
			t.Errorf("options = %v, want the platform's string value \"3\" intact", got.Options)
		}
	})

	t.Run("the demographic survey is marked and carries its heading", func(t *testing.T) {
		got := fieldByID(t, form, "19936")
		if !got.Demographic {
			t.Error("demographic question not marked demographic")
		}
		if got.Section != "U.S. Standard Demographic Questions" {
			t.Errorf("section = %q, want the platform's own heading", got.Section)
		}
		if got.Type != TypeMultiSelect || len(got.Options) != 2 {
			t.Errorf("got %+v, want a two-option multiselect", got)
		}
		if got.Options[0] != (Option{Label: "Man", Value: "121886"}) {
			t.Errorf("option = %v, want the answer option's own id as the value", got.Options[0])
		}
	})

	t.Run("an employer question is not marked", func(t *testing.T) {
		if got := fieldByID(t, form, "first_name"); got.Demographic {
			t.Error("first_name marked demographic, want the employer's own questions unmarked")
		}
	})
}

// The location block is where Greenhouse hides the controls a candidate never sees but an
// application is rejected without.
func TestFromGreenhouseLocationQuestions(t *testing.T) {
	form := decodeGreenhouse(t)

	if got := fieldByID(t, form, "longitude"); got.Type != TypeHidden || !got.Required {
		t.Errorf("longitude = %+v, want a required hidden control", got)
	}
	if got := fieldByID(t, form, "location"); got.Type != TypeText {
		t.Errorf("location type = %q, want %q", got.Type, TypeText)
	}
}

func TestFromGreenhouseTypeVocabulary(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want FieldType
	}{
		{"input_text", TypeText},
		{"textarea", TypeTextarea},
		{"input_file", TypeFile},
		{"multi_value_single_select", TypeSelect},
		{"multi_value_multi_select", TypeMultiSelect},
		{"input_hidden", TypeHidden},
		{"something_new", ""},
	} {
		form := FromGreenhouse(GreenhouseJob{
			Questions: []GreenhouseQuestion{{
				Label:  "Q",
				Fields: []GreenhouseField{{Name: "f", Type: tc.raw}},
			}},
		})
		got := fieldByID(t, form, "f")
		if got.Type != tc.want {
			t.Errorf("type %q -> %q, want %q", tc.raw, got.Type, tc.want)
		}
		if got.RawType != tc.raw {
			t.Errorf("type %q -> raw %q, want it preserved", tc.raw, got.RawType)
		}
	}
}

// An empty compliance block carries no questions at all; it must not become a field.
func TestFromGreenhouseSkipsEmptyBlocks(t *testing.T) {
	form := FromGreenhouse(GreenhouseJob{
		Compliance: []GreenhouseCompliance{{Type: "eeoc"}},
	})

	if len(form.Fields) != 0 {
		t.Errorf("captured %+v, want nothing from a compliance block with no questions", form.Fields)
	}
}
