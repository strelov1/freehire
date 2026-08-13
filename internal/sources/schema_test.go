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
	cases := map[string][]string{
		`{"@type":"Country","name":"US"}`:                                       []string{"US"},
		`[{"@type":"Country","name":"US"},{"@type":"Country","name":"Canada"}]`: []string{"US", "Canada"},
		`[]`: nil,
	}
	for in, want := range cases {
		var a schemaNamedAreas
		if err := json.Unmarshal([]byte(in), &a); err != nil {
			t.Errorf("Unmarshal(%s): %v", in, err)
			continue
		}
		if got := a.Names(); !slices.Equal(got, want) {
			t.Errorf("Unmarshal(%s).Names() = %v, want %v", in, got, want)
		}
	}
}
