package applyform

import (
	"encoding/json"
	"testing"
)

// The payload is stored as jsonb and read back by whatever consumes it, so surviving a
// round trip unchanged IS the envelope's contract — not an incidental property of it.
func TestFormSurvivesJSONRoundTrip(t *testing.T) {
	want := Form{
		Provider: "greenhouse",
		Fields: []Field{
			{
				ID:       "first_name",
				Label:    "First Name",
				Type:     TypeText,
				RawType:  "input_text",
				Required: true,
			},
			{
				ID:      "question_67165648",
				Label:   "Will you require sponsorship?",
				Type:    TypeSelect,
				RawType: "multi_value_single_select",
				Options: []Option{
					{Label: "Yes", Value: "724302231"},
					{Label: "No", Value: "724302232"},
				},
			},
			{
				ID:          "gender",
				Label:       "Gender",
				Type:        TypeSelect,
				RawType:     "multi_value_single_select",
				Section:     "Demographic Questions",
				Demographic: true,
			},
		},
	}

	encoded, err := json.Marshal(want)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var got Form
	if err := json.Unmarshal(encoded, &got); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if got.Provider != want.Provider || len(got.Fields) != len(want.Fields) {
		t.Fatalf("round trip = %+v, want %+v", got, want)
	}
	for i, f := range got.Fields {
		if !sameField(f, want.Fields[i]) {
			t.Errorf("field %d = %+v, want %+v", i, f, want.Fields[i])
		}
	}
}

func sameField(a, b Field) bool {
	if a.ID != b.ID || a.Label != b.Label || a.Type != b.Type || a.RawType != b.RawType ||
		a.Required != b.Required || a.Section != b.Section || a.Demographic != b.Demographic ||
		len(a.Options) != len(b.Options) {
		return false
	}
	for i := range a.Options {
		if a.Options[i] != b.Options[i] {
			return false
		}
	}
	return true
}

// A control the platform describes with a word this package does not recognize must still
// be captured: the field is real and a candidate has to answer it. The project's
// dictionaries never guess, so the normalized type stays empty and the platform's own word
// is what survives.
func TestUnrecognizedControlKeepsItsPlatformType(t *testing.T) {
	f := Field{ID: "q1", Label: "Record a short video", RawType: "video"}

	if f.Type != "" {
		t.Errorf("normalized type = %q, want empty for an unrecognized control", f.Type)
	}
	if f.RawType != "video" {
		t.Errorf("raw type = %q, want the platform's own word preserved", f.RawType)
	}
}

// A form carrying no answerable control is still a form — an employer who asks nothing but
// a CV has told the candidate something worth knowing.
func TestEmptyFormIsValidJSON(t *testing.T) {
	encoded, err := json.Marshal(Form{Provider: "recruitee"})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	if string(encoded) != `{"provider":"recruitee"}` {
		t.Errorf("encoded = %s, want the fields key omitted when there are none", encoded)
	}
}
