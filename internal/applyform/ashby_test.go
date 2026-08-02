package applyform

import (
	"encoding/json"
	"testing"
)

// Ashby returns each control's descriptor as an opaque JSON blob inside the entry, so the
// fixture is raw JSON: a struct literal would skip the decode step that is the only place
// this mapper can go wrong.
const ashbyFixture = `{
  "sections": [
    {"title": "Personal details", "isHidden": false, "fieldEntries": [
      {"id": "form1__systemfield_name", "isRequired": true, "isHidden": false,
       "field": {"path": "_systemfield_name", "title": "First and last name", "type": "String"}},
      {"id": "form1__systemfield_email", "isRequired": true, "isHidden": false,
       "field": {"path": "_systemfield_email", "title": "Email", "type": "Email"}},
      {"id": "form1__systemfield_resume", "isRequired": true, "isHidden": false,
       "field": {"path": "_systemfield_resume", "title": "Resume", "type": "File"}}
    ]},
    {"title": "General background", "isHidden": false, "fieldEntries": [
      {"id": "form1_a0980767", "isRequired": true, "isHidden": false,
       "field": {"path": "a0980767", "title": "How long have you been working remotely?",
                 "type": "ValueSelect",
                 "selectableValues": [{"label": "Less than 2 years", "value": "Less than 2 years"},
                                      {"label": "More than 10 years", "value": "More than 10 years"}]}},
      {"id": "form1_80be47fa", "isRequired": false, "isHidden": false,
       "field": {"path": "80be47fa", "title": "Which languages do you speak fluently?",
                 "type": "MultiValueSelect",
                 "selectableValues": [{"label": "English", "value": "English"}]}},
      {"id": "form1_b2af20a1", "isRequired": true, "isHidden": false,
       "field": {"path": "b2af20a1", "title": "Why did you apply?", "type": "LongText"}}
    ]},
    {"title": "Internal", "isHidden": true, "fieldEntries": [
      {"id": "form1_secret", "isRequired": true, "isHidden": false,
       "field": {"path": "secret", "title": "Internal note", "type": "String"}}
    ]},
    {"title": "Mixed", "isHidden": false, "fieldEntries": [
      {"id": "form1_shown", "isRequired": false, "isHidden": false,
       "field": {"path": "shown", "title": "Shown", "type": "String"}},
      {"id": "form1_concealed", "isRequired": true, "isHidden": true,
       "field": {"path": "concealed", "title": "Concealed", "type": "String"}}
    ]}
  ]
}`

func decodeAshby(t *testing.T) Form {
	t.Helper()
	var form AshbyApplicationForm
	if err := json.Unmarshal([]byte(ashbyFixture), &form); err != nil {
		t.Fatalf("decode fixture: %v", err)
	}
	return FromAshby(form)
}

func TestFromAshbyFields(t *testing.T) {
	form := decodeAshby(t)

	if form.Provider != "ashby" {
		t.Errorf("provider = %q, want %q", form.Provider, "ashby")
	}

	t.Run("the field path is the identifier, not the form-scoped entry id", func(t *testing.T) {
		// The entry id is scoped to one rendered form; the path is what the platform's own
		// client keys a submitted answer on.
		got := fieldByID(t, form, "_systemfield_email")
		if got.Label != "Email" || !got.Required {
			t.Errorf("email = %+v, want the labelled required control", got)
		}
		if hasField(form, "form1__systemfield_email") {
			t.Error("captured the entry id as an identifier, want the field path")
		}
	})

	t.Run("the section heading is carried", func(t *testing.T) {
		got := fieldByID(t, form, "_systemfield_name")
		if got.Section != "Personal details" {
			t.Errorf("section = %q, want %q", got.Section, "Personal details")
		}
	})

	t.Run("selectable values become options", func(t *testing.T) {
		got := fieldByID(t, form, "a0980767")
		if got.Type != TypeSelect {
			t.Errorf("type = %q, want %q", got.Type, TypeSelect)
		}
		want := []Option{
			{Label: "Less than 2 years", Value: "Less than 2 years"},
			{Label: "More than 10 years", Value: "More than 10 years"},
		}
		if len(got.Options) != len(want) {
			t.Fatalf("options = %v, want %v", got.Options, want)
		}
		for i, o := range got.Options {
			if o != want[i] {
				t.Errorf("option %d = %v, want %v", i, o, want[i])
			}
		}
	})

	t.Run("a long-text answer is distinguishable from a one-liner", func(t *testing.T) {
		// This is the distinction that decides whether an application costs a minute or an
		// evening, so it must not collapse into plain text.
		if got := fieldByID(t, form, "b2af20a1"); got.Type != TypeTextarea {
			t.Errorf("LongText -> %q, want %q", got.Type, TypeTextarea)
		}
	})
}

// A control the platform hides is not part of what a candidate faces, and neither is
// anything inside a hidden section.
func TestFromAshbySkipsHidden(t *testing.T) {
	form := decodeAshby(t)

	if hasField(form, "secret") {
		t.Error("captured a control from a hidden section")
	}
	if hasField(form, "concealed") {
		t.Error("captured a hidden control")
	}
	if !hasField(form, "shown") {
		t.Error("dropped a visible control that shares a section with a hidden one")
	}
}

func TestFromAshbyTypeVocabulary(t *testing.T) {
	for _, tc := range []struct {
		raw  string
		want FieldType
	}{
		{"String", TypeText},
		{"Email", TypeText},
		{"LongText", TypeTextarea},
		{"ValueSelect", TypeSelect},
		{"MultiValueSelect", TypeMultiSelect},
		{"Boolean", TypeBoolean},
		{"File", TypeFile},
		// A location picker is not a select and not free text; the vocabulary has no
		// honest entry for it, so it gets none.
		{"Location", ""},
		{"SomethingNew", ""},
	} {
		form := FromAshby(AshbyApplicationForm{
			Sections: []AshbySection{{FieldEntries: []AshbyFieldEntry{
				{Field: AshbyField{Path: "p", Title: "T", Type: tc.raw}},
			}}},
		})
		got := fieldByID(t, form, "p")
		if got.Type != tc.want {
			t.Errorf("type %q -> %q, want %q", tc.raw, got.Type, tc.want)
		}
		if got.RawType != tc.raw {
			t.Errorf("type %q -> raw %q, want it preserved", tc.raw, got.RawType)
		}
	}
}
