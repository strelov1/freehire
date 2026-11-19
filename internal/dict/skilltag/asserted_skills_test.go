package skilltag

import (
	"slices"
	"testing"
)

// Nx and design systems are skills a person asserts on a profile, but their bare forms are
// unsafe in job prose (Siemens NX; "design systems that scale"). They resolve when asserted,
// tag jobs only through qualified phrases, and never through job-side Canonicalize.
func TestAssertedOnlySkills(t *testing.T) {
	for _, tc := range []struct{ term, canonical string }{
		{"Nx", "nx"}, {"nx", "nx"}, {"NX", "nx"},
		{"design systems", "design-systems"}, {"Design System", "design-systems"},
		{"design-systems", "design-systems"},
	} {
		if got := Canonicalize([]string{tc.term}, WithResumeAcronyms()); !slices.Equal(got, []string{tc.canonical}) {
			t.Errorf("Canonicalize(%q, résumé) = %v, want [%s]", tc.term, got, tc.canonical)
		}
		if got := Canonicalize([]string{tc.term}); slices.Contains(got, tc.canonical) {
			t.Errorf("job-side Canonicalize(%q) = %v, must not resolve the bare form", tc.term, got)
		}
	}

	for _, tc := range []struct{ text, canonical string }{
		{"Our frontend lives in an Nx monorepo with Angular and TypeScript.", "nx"},
		{"Experience with Nrwl Nx and Jest.", "nx"},
		{"Build React design system components in Storybook.", "design-systems"},
		{"Join the Design Systems team.", "design-systems"},
	} {
		if got := Parse(tc.text); !slices.Contains(got, tc.canonical) {
			t.Errorf("Parse(%q) = %v, want %s", tc.text, got, tc.canonical)
		}
	}
	for _, text := range []string{
		"Mechanical engineer: 3D modelling in Siemens NX and CATIA.",
		"You will design systems that scale to millions of requests with Go and Kubernetes.",
		"Backend engineer: you design system architecture for payments.",
	} {
		if got := Parse(text); slices.Contains(got, "nx") || slices.Contains(got, "design-systems") {
			t.Errorf("Parse(%q) = %v, must not tag nx or design-systems", text, got)
		}
	}
}
