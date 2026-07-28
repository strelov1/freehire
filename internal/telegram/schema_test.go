package telegram

import (
	"testing"
)

func TestRequestSchema_DescribesTheExtractionContract(t *testing.T) {
	s, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	props, ok := s["properties"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no properties")
	}
	jobs, ok := props["jobs"].(map[string]any)
	if !ok {
		t.Fatal("schema carries no jobs property")
	}
	item, ok := jobs["items"].(map[string]any)
	if !ok {
		t.Fatal("jobs carries no item schema")
	}
	fields, ok := item["properties"].(map[string]any)
	if !ok {
		t.Fatal("job item carries no properties")
	}

	for _, field := range []string{"title", "company", "location", "remote", "description"} {
		if _, ok := fields[field]; !ok {
			t.Errorf("job item is missing %q, so the model would stop returning it", field)
		}
	}
	if item["additionalProperties"] != false {
		t.Error("job item allows additional properties, which strict mode rejects")
	}
}

// A post that is not a vacancy is a normal outcome, and the schema must leave the
// model able to say so rather than forcing it to produce a job.
func TestRequestSchema_AllowsAnEmptyExtraction(t *testing.T) {
	s, err := requestSchema()
	if err != nil {
		t.Fatalf("requestSchema: %v", err)
	}

	props, _ := s["properties"].(map[string]any)
	jobs, _ := props["jobs"].(map[string]any)
	if _, hasMin := jobs["minItems"]; hasMin {
		t.Error("jobs carries a minimum count; zero jobs is a valid extraction")
	}
}

// The repair runs before unmarshalling, so it must survive the migration: a schema
// constrains structure, not the raw control characters a model writes inside a string.
func TestParseExtraction_StillRepairsControlCharacters(t *testing.T) {
	raw := "{\"jobs\":[{\"title\":\"Go dev\",\"description\":\"line one\nline two\"}]}"

	ex, err := parseExtraction(raw)
	if err != nil {
		t.Fatalf("parseExtraction: %v", err)
	}
	if len(ex.Jobs) != 1 {
		t.Fatalf("jobs = %d, want 1", len(ex.Jobs))
	}
	if ex.Jobs[0].Title != "Go dev" {
		t.Errorf("title = %q, want %q", ex.Jobs[0].Title, "Go dev")
	}
}
