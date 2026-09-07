package credentials

import (
	"slices"
	"testing"
)

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

// A canonical slug must resolve to itself through Canonical.
func TestCanonicalSlugResolvesToItself(t *testing.T) {
	if got, ok := Canonical("cissp"); !ok || got != "cissp" {
		t.Errorf("Canonical(canonical slug) = %q, %v; want cissp, true", got, ok)
	}
}

// Scan reads a job DESCRIPTION, which is prose, and a two- or three-letter alias is a word
// ordinary prose also uses. Each of these was measured against the live table: "A+ players"
// resolved comptia-a-plus, a Salesforce CSM resolved csm, and an accountancy firm resolved
// cpa. A fourth, "CISA guidelines", is knowingly left — see the table for why.
//
// The consequence was never a wrong LIST: it is a false "requires this certification",
// which caps the candidate's score at 60 and states the false requirement in all three
// stages of the analysis prompt as already established.
func TestScanIgnoresAnAcronymOrdinaryProseAlsoUses(t *testing.T) {
	cases := map[string]string{
		"We are looking for A+ players":               "comptia-a-plus",
		"Salesforce CSM and CRM tooling":              "csm",
		"Work with our CPA firm on the monthly close": "cpa",
	}
	for text, unwanted := range cases {
		t.Run(text, func(t *testing.T) {
			for _, got := range Scan(text) {
				if got == unwanted {
					t.Errorf("Scan(%q) resolved %q from prose", text, unwanted)
				}
			}
		})
	}
}

// The suppression is on the bare acronym, not on the credential. A posting that spells the
// certification out is stating a requirement, and that must still be read — otherwise the
// fix trades a false blocker for a missed one.
func TestScanStillReadsTheSpeltOutFormOfAnAmbiguousCredential(t *testing.T) {
	cases := map[string]string{
		"Must hold CompTIA A+":                             "comptia-a-plus",
		"Certified ScrumMaster required":                   "csm",
		"Certified Public Accountant (or equivalent)":      "cpa",
		"CISSP holders preferred":                          "cissp",
		"PMP certification required":                       "pmp",
		"Certified Kubernetes Administrator for the infra": "cka",
		// Not marked ambiguous, so its bare acronym still reads — see the table.
		"CISA required": "cisa",
	}
	for text, want := range cases {
		t.Run(text, func(t *testing.T) {
			if !slices.Contains(Scan(text), want) {
				t.Errorf("Scan(%q) = %v, want it to contain %q", text, Scan(text), want)
			}
		})
	}
}

// ScanLine reads ONE deliberate field — a line off a résumé's certification list — where a
// bare acronym IS the credential and nothing around it is prose. It is the permissive half
// of the same table, and the asymmetry is the point: on the résumé side a missed credential
// becomes a FALSE BLOCKER, while a spurious one merely fails to block.
//
// Every one of these was measured returning ("", false) from Canonical, which matches the
// whole string — and a résumé entry is almost never exactly an alias.
func TestScanLineReadsADecoratedResumeEntry(t *testing.T) {
	cases := map[string]string{
		"CompTIA Security+ (2022)":                      "comptia-security-plus",
		"Certified Kubernetes Administrator (CKA)":      "cka",
		"AWS Certified Solutions Architect – Associate": "aws-solutions-architect",
		"CPA, 2019": "cpa",
		"CISSP":     "cissp",
	}
	for entry, want := range cases {
		t.Run(entry, func(t *testing.T) {
			if !slices.Contains(ScanLine(entry), want) {
				t.Errorf("ScanLine(%q) = %v, want it to contain %q", entry, ScanLine(entry), want)
			}
		})
	}
}

// The slug is an identifier and the label is what a person reads. Reason and Action are
// rendered verbatim on the job page and go verbatim into all three stages of the analysis
// prompt, so a missing label would put "gcp-professional-cloud-architect" in front of a
// candidate. The table owns the vocabulary, so walking it is completeness, not consistency.
func TestEveryEntryCarriesAReadableLabel(t *testing.T) {
	for _, e := range table {
		if e.label == "" {
			t.Errorf("entry %q carries no label", e.slug)
			continue
		}
		if e.label == e.slug {
			t.Errorf("entry %q labels itself with its own slug", e.slug)
		}
		if got := Label(e.slug); got != e.label {
			t.Errorf("Label(%q) = %q, want %q", e.slug, got, e.label)
		}
	}
	// An unknown slug reads as itself rather than as an empty string: a caller
	// interpolating it produces a poor sentence, never a broken one.
	if got := Label("not-a-real-slug"); got != "not-a-real-slug" {
		t.Errorf("Label(unknown) = %q, want the slug back", got)
	}
}
