package credentials

import "testing"

func TestCanonicalResolvesAliases(t *testing.T) {
	cases := map[string]string{
		"AWS Certified Solutions Architect":  "aws-solutions-architect",
		"aws solutions architect":            "aws-solutions-architect",
		"SAA-C03":                            "aws-solutions-architect",
		"Certified Kubernetes Administrator": "cka",
		"CKA":                                "cka",
		"CompTIA Security+":                  "comptia-security-plus",
		"security+":                          "comptia-security-plus",
		"CISSP":                              "cissp",
		"PMP":                                "pmp",
		"Project Management Professional":    "pmp",
	}
	for raw, want := range cases {
		got, ok := Canonical(raw)
		if !ok || got != want {
			t.Errorf("Canonical(%q) = %q, %v; want %q, true", raw, got, ok, want)
		}
	}
}

func TestCanonicalNormalizesWhitespaceAndPunctuation(t *testing.T) {
	// Extra spaces and surrounding punctuation must not defeat the lookup.
	if got, ok := Canonical("  AWS   Solutions   Architect  "); !ok || got != "aws-solutions-architect" {
		t.Errorf("Canonical with padding = %q, %v; want aws-solutions-architect, true", got, ok)
	}
}

func TestCanonicalUnknownReturnsNotOK(t *testing.T) {
	if got, ok := Canonical("underwater basket weaving certificate"); ok {
		t.Errorf("Canonical(unknown) = %q, true; want _, false", got)
	}
	if _, ok := Canonical(""); ok {
		t.Error("Canonical(empty) = true; want false")
	}
}

func TestIsCanonicalGatesTheControlledSet(t *testing.T) {
	if !IsCanonical("cissp") {
		t.Error("IsCanonical(cissp) = false; want true")
	}
	if IsCanonical("not-a-real-slug") {
		t.Error("IsCanonical(not-a-real-slug) = true; want false")
	}
	// A canonical slug must resolve to itself through Canonical.
	if got, ok := Canonical("cissp"); !ok || got != "cissp" {
		t.Errorf("Canonical(canonical slug) = %q, %v; want cissp, true", got, ok)
	}
}
