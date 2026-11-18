package llmschema

import (
	"encoding/json"
	"os"
	"reflect"
	"testing"
)

// TestSchemaGolden pins the schema sent to the model as an LLM contract: any change
// to the generated document must be deliberate. Compared as decoded JSON, so key
// order is irrelevant. Regenerate with UPDATE_GOLDEN=1.
func TestSchemaGolden(t *testing.T) {
	s, err := Of[contract](Enum("nickname", []string{"a", "b"}), Enum("tags", []string{"x"}), AsText("years", "nickname"))
	if err != nil {
		t.Fatal(err)
	}
	const path = "testdata/schema.golden.json"
	if os.Getenv("UPDATE_GOLDEN") != "" {
		b, _ := json.MarshalIndent(s, "", "  ")
		if err := os.MkdirAll("testdata", 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, append(b, '\n'), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var want, got any
	if err := json.Unmarshal(raw, &want); err != nil {
		t.Fatal(err)
	}
	b, _ := json.Marshal(s)
	if err := json.Unmarshal(b, &got); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("schema drifted from golden\n got: %s", b)
	}
}
