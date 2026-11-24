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

func TestIsTelecommute(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"TELECOMMUTE", true}, {"telecommute", true}, {"Telecommute", true}, {" TELECOMMUTE ", true},
		{"TELE_COMMUTE", false}, {"ON_SITE", false}, {"", false},
	}
	for _, tc := range cases {
		if got := IsTelecommute(tc.in); got != tc.want {
			t.Errorf("IsTelecommute(%q) = %v, want %v", tc.in, got, tc.want)
		}
	}
}

func TestOneOrMany(t *testing.T) {
	type area struct {
		Name string `json:"name"`
	}
	// Both element kinds the adapters decode: an enum string, and a schema.org node. A row
	// names the kind it applies to; the other kind is a type mismatch and must error.
	tests := []struct {
		name string
		raw  string
		strs []string // want for oneOrMany[string]; nil means a type mismatch
		objs []area   // want for oneOrMany[area]; nil means a type mismatch
	}{
		{name: "one string", raw: `"FULL_TIME"`, strs: []string{"FULL_TIME"}},
		{name: "array of strings", raw: `["FULL_TIME","TEMPORARY"]`, strs: []string{"FULL_TIME", "TEMPORARY"}},
		{name: "one object", raw: `{"name":"US"}`, objs: []area{{Name: "US"}}},
		{name: "array of objects", raw: `[{"name":"US"},{"name":"Canada"}]`, objs: []area{{Name: "US"}, {Name: "Canada"}}},
		{name: "empty array", raw: `[]`, strs: []string{}, objs: []area{}},
		// null is "nothing stated" and decodes to one zero value, as it did at every caller
		// before the helper; a caller that must not keep it drops it itself (ptsecurity).
		{name: "null", raw: `null`, strs: []string{""}, objs: []area{{}}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := oneOrMany[string](json.RawMessage(tc.raw))
			if tc.strs == nil {
				if err == nil {
					t.Errorf("oneOrMany[string](%s) = %q, want a type error", tc.raw, got)
				}
			} else if err != nil || !slices.Equal(got, tc.strs) {
				t.Errorf("oneOrMany[string](%s) = %q, %v; want %q", tc.raw, got, err, tc.strs)
			}
			gotObj, err := oneOrMany[area](json.RawMessage(tc.raw))
			if tc.objs == nil {
				if err == nil {
					t.Errorf("oneOrMany[area](%s) = %+v, want a type error", tc.raw, gotObj)
				}
			} else if err != nil || !slices.Equal(gotObj, tc.objs) {
				t.Errorf("oneOrMany[area](%s) = %+v, %v; want %+v", tc.raw, gotObj, err, tc.objs)
			}
		})
	}

	// A body that is not JSON at all is an error too, never a zero value.
	for _, raw := range []string{``, `nul`, `42`} {
		if _, err := oneOrMany[string](json.RawMessage(raw)); err == nil {
			t.Errorf("oneOrMany[string](%s) must error", raw)
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

func TestSchemaFixedTermEmploymentType(t *testing.T) {
	for raw, want := range map[string]string{
		`["FULL_TIME"]`:             "full_time",
		`["TEMPORARY","FULL_TIME"]`: "full_time",
		`["TEMPORARY"]`:             "",
		`"PART_TIME"`:               "part_time",
		`["INTERN"]`:                "internship",
		`null`:                      "",
	} {
		if got := schemaFixedTermEmploymentType([]byte(raw)); got != want {
			t.Errorf("schemaFixedTermEmploymentType(%s) = %q, want %q", raw, got, want)
		}
	}
}
