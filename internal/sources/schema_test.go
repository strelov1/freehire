package sources

import "testing"

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
