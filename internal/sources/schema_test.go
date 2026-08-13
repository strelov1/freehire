package sources

import (
	"encoding/json"
	"slices"
	"testing"
)

func TestSchemaEmploymentType(t *testing.T) {
	cases := map[string]string{
		"FULL_TIME": "full_time", "PART_TIME": "part_time",
		"CONTRACTOR": "contract", "TEMPORARY": "contract",
		"INTERN": "internship", "OTHER": "", "": "",
	}
	for in, want := range cases {
		if got := schemaEmploymentType(in); got != want {
			t.Errorf("schemaEmploymentType(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestSchemaNamedAreasDecodesSingleObjectOrArray(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"single object", `{"@type":"Country","name":"US"}`, []string{"US"}},
		{"array", `[{"@type":"Country","name":"US"},{"@type":"Country","name":"Canada"}]`, []string{"US", "Canada"}},
		{"empty array", `[]`, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var a schemaNamedAreas
			if err := json.Unmarshal([]byte(tc.in), &a); err != nil {
				t.Fatalf("Unmarshal(%s): %v", tc.in, err)
			}
			if got := a.Names(); !slices.Equal(got, tc.want) {
				t.Errorf("Unmarshal(%s).Names() = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}
