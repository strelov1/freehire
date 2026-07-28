package resumeextract

import (
	"encoding/json"
	"slices"
	"testing"
)

func schemaProperties(t *testing.T) map[string]any {
	t.Helper()

	s, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no properties")
	}

	return props
}

// The model is handed a CV with the contacts redacted out. Under strict mode every
// property is required, so a contact field left in the schema is an order to invent
// one — the opposite of the rule this package is built on.
func TestRequestSchema_DoesNotAskTheModelForContacts(t *testing.T) {
	props := schemaProperties(t)

	for _, field := range contactFields {
		if _, ok := props[field]; ok {
			t.Errorf("schema asks the model for %q, which only PII detection may supply", field)
		}
	}
}

func TestRequestSchema_CarriesTheSemanticFields(t *testing.T) {
	props := schemaProperties(t)

	for _, field := range []string{"headline", "summary", "experience", "education", "skills", "projects"} {
		if _, ok := props[field]; !ok {
			t.Errorf("schema is missing %q, so the model would stop returning it", field)
		}
	}
}

// Asked for an integer, a model given "5.9 years" returns 6; truncInt returns 5. The
// schema must not take that arithmetic away from the decoder.
func TestRequestSchema_AsksForTotalYearsAsText(t *testing.T) {
	years, ok := schemaProperties(t)["total_years"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no total_years")
	}

	switch typ := years["type"].(type) {
	case string:
		if typ != "string" {
			t.Errorf("total_years type = %q, want string", typ)
		}
	case []any:
		if !slices.Contains(typ, any("string")) {
			t.Errorf("total_years type = %v, want string among them", typ)
		}
	default:
		t.Fatalf("total_years has no type: %#v", years)
	}
}

// The schema constrains the type; the truncation is still the decoder's job, and this
// is the pairing that keeps a fractional year count from rounding up.
func TestStructured_FractionalYearsStillTruncate(t *testing.T) {
	var s Structured
	if err := json.Unmarshal([]byte(`{"total_years":"5.9"}`), &s); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if s.TotalYears != 5 {
		t.Errorf("total_years = %d, want 5 — rounding up invents experience", s.TotalYears)
	}
}
