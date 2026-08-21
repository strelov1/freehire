package searchintent

import (
	"encoding/json"
	"testing"
)

// The schema is derived from the proposal type by reflection, so a change to a FIELD's
// Go type silently changes what the model is told to send. That is how the bounds broke:
// flexInt became a struct (to tell "" apart from "0"), reflection saw a struct with no
// exported fields, and every bound was declared to the model as an empty OBJECT. Models
// honouring the schema dutifully returned {} and the bound vanished — which looked for
// days like the model ignoring instructions.
//
// These assertions are cheap and they are the only thing standing between a field's Go
// type and a silently useless schema.
func TestSchemaDeclaresEveryFieldAsSomethingTheModelCanWrite(t *testing.T) {
	schema, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}
	raw, err := json.Marshal(schema)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var doc struct {
		Properties map[string]struct {
			Type  any `json:"type"`
			Items *struct {
				Type any `json:"type"`
			} `json:"items"`
			Properties map[string]any `json:"properties"`
		} `json:"properties"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("decode schema: %v", err)
	}

	// An object with no properties is a field the model can only answer with "{}".
	for name, prop := range doc.Properties {
		if isObject(prop.Type) && len(prop.Properties) == 0 {
			t.Errorf("%q is declared as an empty object — the model has nothing it can write there", name)
		}
	}

	for _, name := range []string{"salary_min", "posted_within_days", "experience_years_max"} {
		prop, ok := doc.Properties[name]
		if !ok {
			t.Errorf("%q is missing from the schema, so the model can never send it", name)
			continue
		}
		if isObject(prop.Type) {
			t.Errorf("%q is declared as an object; it is a bound and must be asked for as text or a number", name)
		}
	}

	// The facets are lists, and a list of objects is the same trap one level down.
	for _, name := range []string{"skills", "regions", "countries", "seniority"} {
		prop, ok := doc.Properties[name]
		if !ok {
			t.Errorf("%q is missing from the schema", name)
			continue
		}
		if prop.Items == nil || isObject(prop.Items.Type) {
			t.Errorf("%q items = %v, want strings", name, prop.Items)
		}
	}
}

// isObject reports whether a schema `type` (a string, or a list like ["string","null"])
// names an object.
func isObject(t any) bool {
	switch v := t.(type) {
	case string:
		return v == "object"
	case []any:
		for _, one := range v {
			if s, ok := one.(string); ok && s == "object" {
				return true
			}
		}
	}
	return false
}
