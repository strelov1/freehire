package certification

import (
	"reflect"
	"testing"
)

func TestCanonicalize(t *testing.T) {
	cases := []struct {
		name   string
		tokens []string
		want   []string
	}{
		{"empty", nil, nil},
		{
			"known aliases resolve",
			[]string{"AWS Certified Solutions Architect", "PMP", "CKA"},
			[]string{"aws-solutions-architect", "cka", "pmp"},
		},
		{
			"case and spacing insensitive",
			[]string{"aws certified solutions architect - associate"},
			[]string{"aws-solutions-architect"},
		},
		{
			"duplicate input dedups",
			[]string{"PMP", "pmp", "Project Management Professional"},
			[]string{"pmp"},
		},
		{
			"unresolved is dropped, not passed through",
			[]string{"Certified Underwater Basket Weaver"},
			nil,
		},
		{
			"mixed resolved and unresolved",
			[]string{"CISSP", "Certified Underwater Basket Weaver", "CKAD"},
			[]string{"cissp", "ckad"},
		},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := Canonicalize(c.tokens)
			if !reflect.DeepEqual(got, c.want) {
				t.Errorf("Canonicalize(%v) = %v, want %v", c.tokens, got, c.want)
			}
		})
	}
}

// The occupational-safety credentials. They live here rather than in skilltag for two
// reasons that agree: they name credentials rather than skills, and skilltag's own
// invariant rejects them outright — a scoped acronym there must resolve to a canonical
// that already exists, because an acronym is another alias and never a new facet value.
func TestCanonicalize_OccupationalSafetyCredentials(t *testing.T) {
	cases := []struct {
		token string
		want  string
	}{
		{"NEBOSH", "nebosh"},
		{"NEBOSH General Certificate", "nebosh"},
		{"IOSH", "iosh"},
		{"IOSH Managing Safely", "iosh"},
		{"CSP", "csp"},
		{"Certified Safety Professional", "csp"},
		{"CIH", "cih"},
		{"Certified Industrial Hygienist", "cih"},
		{"CHMM", "chmm"},
		{"Certified Hazardous Materials Manager", "chmm"},
		{"HAZWOPER", "hazwoper"},
	}
	for _, tc := range cases {
		got := Canonicalize([]string{tc.token})
		if !reflect.DeepEqual(got, []string{tc.want}) {
			t.Errorf("Canonicalize([%q]) = %v, want [%q]", tc.token, got, tc.want)
		}
	}
}
