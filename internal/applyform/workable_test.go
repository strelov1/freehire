package applyform

import (
	"encoding/json"
	"testing"
)

// Raw JSON rather than struct literals: the option pair's naming is inverted relative to
// every other platform here, and that is precisely the kind of mistake a hand-built
// fixture would reproduce instead of catching.
const workableFixture = `[
  {"name": "Personal information", "fields": [
    {"id": "firstname", "required": true, "label": "First name", "type": "text"},
    {"id": "lastname", "required": true, "label": "Last name", "type": "text"},
    {"id": "email", "required": true, "label": "Email", "type": "email"},
    {"id": "phone", "required": true, "label": "Phone", "type": "phone"}
  ]},
  {"name": "Profile", "fields": [
    {"id": "resume", "required": true, "label": "Resume", "type": "file"},
    {"id": "education", "required": false, "label": "Education", "type": "group", "fields": [
      {"id": "school", "required": true, "label": "School", "type": "text"},
      {"id": "degree", "required": false, "label": "Degree", "type": "text"},
      {"id": "start_date", "required": false, "label": "Start date", "type": "date"}
    ]},
    {"id": "summary", "required": false, "label": "Summary", "type": "paragraph"}
  ]},
  {"name": "Details", "fields": [
    {"id": "QA_1", "required": true, "type": "dropdown", "singleOption": true,
     "label": "How engaged are you with the AI ecosystem?",
     "options": [{"name": "6166574", "value": "I attend events and know people at AI labs"},
                 {"name": "6166575", "value": "Familiar, but no active relationships"}]},
    {"id": "QA_2", "required": true, "type": "multiple",
     "label": "Which AWS services have you used?",
     "options": [{"name": "6097791", "value": "Amazon Bedrock"},
                 {"name": "6097793", "value": "SageMaker"}]},
    {"id": "QA_3", "required": true, "type": "paragraph", "label": "Why this role?"},
    {"id": "QA_4", "required": false, "type": "boolean", "label": "Do you need sponsorship?"},
    {"id": "QA_5", "required": false, "type": "date", "label": "When can you start?"},
    {"id": "QA_6", "required": false, "type": "number", "label": "Years of experience"}
  ]}
]`

func decodeWorkable(t *testing.T) Form {
	t.Helper()
	var sections []WorkableSection
	if err := json.Unmarshal([]byte(workableFixture), &sections); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return FromWorkable(sections)
}

func TestFromWorkableFields(t *testing.T) {
	form := decodeWorkable(t)

	if form.Provider != "workable" {
		t.Errorf("provider = %q, want %q", form.Provider, "workable")
	}

	byID := map[string]Field{}
	for _, f := range form.Fields {
		byID[f.ID] = f
	}

	for _, tc := range []struct {
		id       string
		typ      FieldType
		required bool
	}{
		{"firstname", TypeText, true},
		// email and phone are text boxes; the normalized vocabulary names the KIND of
		// control, and the validation hint survives in RawType.
		{"email", TypeText, true},
		{"phone", TypeText, true},
		{"resume", TypeFile, true},
		{"summary", TypeTextarea, false},
		{"QA_1", TypeSelect, true},
		{"QA_2", TypeMultiSelect, true},
		{"QA_3", TypeTextarea, true},
		{"QA_4", TypeBoolean, false},
		{"QA_5", TypeDate, false},
		{"QA_6", TypeNumber, false},
	} {
		got, ok := byID[tc.id]
		if !ok {
			t.Errorf("%s missing from the capture", tc.id)
			continue
		}
		if got.Type != tc.typ {
			t.Errorf("%s type = %q, want %q", tc.id, got.Type, tc.typ)
		}
		if got.Required != tc.required {
			t.Errorf("%s required = %v, want %v", tc.id, got.Required, tc.required)
		}
	}

	// The section is the heading a candidate reads the question under.
	if got := byID["QA_1"].Section; got != "Details" {
		t.Errorf("QA_1 section = %q, want %q", got, "Details")
	}
}

// Workable names an option's two halves the opposite way round from Greenhouse,
// Recruitee and Ashby: the identifier is `name`, the text is `value`. Written by
// analogy the mapper would label every choice with a number and submit the sentence.
func TestFromWorkableReadsTheOptionPairInItsOwnOrder(t *testing.T) {
	form := decodeWorkable(t)

	var q Field
	for _, f := range form.Fields {
		if f.ID == "QA_1" {
			q = f
		}
	}

	want := []Option{
		{Label: "I attend events and know people at AI labs", Value: "6166574"},
		{Label: "Familiar, but no active relationships", Value: "6166575"},
	}
	if len(q.Options) != len(want) {
		t.Fatalf("options = %+v, want %+v", q.Options, want)
	}
	for i, o := range q.Options {
		if o != want[i] {
			t.Errorf("option %d = %+v, want %+v (label is the platform's `value`)", i, o, want[i])
		}
	}
}

// Education and Experience are repeatable compounds. Their parts are the parts of ONE
// entry, and listing them individually would say "this application asks for your start
// date" where the true statement is "it asks for your education history".
func TestFromWorkableCapturesAGroupAsOneControl(t *testing.T) {
	form := decodeWorkable(t)

	var group *Field
	for i, f := range form.Fields {
		switch f.ID {
		case "education":
			group = &form.Fields[i]
		case "school", "degree", "start_date":
			t.Errorf("captured %q — a group's parts must not be walked", f.ID)
		}
	}
	if group == nil {
		t.Fatal("the education group was dropped entirely, want it kept as one control")
	}
	if group.Label != "Education" {
		t.Errorf("group label = %q, want %q", group.Label, "Education")
	}
	// A group is not a kind of answer, so it gets no normalized type — the same
	// dict-only rule the rest of the capture follows.
	if group.Type != "" {
		t.Errorf("group type = %q, want none — a group is not an answer kind", group.Type)
	}
	if group.RawType != "group" {
		t.Errorf("group raw type = %q, want %q", group.RawType, "group")
	}
}

// The vocabulary was measured across 40 live postings, and `multiple` appeared in none
// of the first ten — a mapper written from the smaller sample would have silently
// dropped every multi-choice question. An unmeasured kind still yields a captured field
// with no normalized type rather than nothing.
func TestFromWorkableKeepsAnUnknownKind(t *testing.T) {
	form := FromWorkable([]WorkableSection{{
		Name:   "Details",
		Fields: []WorkableField{{ID: "QA_9", Label: "Record a video", Type: "video_question"}},
	}})

	if len(form.Fields) != 1 {
		t.Fatalf("captured %+v, want the field kept", form.Fields)
	}
	if form.Fields[0].Type != "" {
		t.Errorf("type = %q, want none for an unmeasured kind", form.Fields[0].Type)
	}
	if form.Fields[0].RawType != "video_question" {
		t.Errorf("raw type = %q, want the platform's word kept", form.Fields[0].RawType)
	}
}
