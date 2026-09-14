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
