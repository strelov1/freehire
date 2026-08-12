package skilltag

import (
	"reflect"
	"slices"
	"strings"
	"testing"
)

func TestPreferredFromText_LongestWins(t *testing.T) {
	got := PreferredFromText("We practice IaC and infrastructure as code daily.")
	if got["infrastructure-as-code"] != "infrastructure as code" {
		t.Fatalf("preferred = %q, want the spelled-out form; full map %#v", got["infrastructure-as-code"], got)
	}
}

func TestPreferredFromText_JDOnlyAcronym(t *testing.T) {
	got := PreferredFromText("Experience with IaC and Terraform.")
	if got["infrastructure-as-code"] != "IaC" {
		t.Fatalf("preferred = %q, want IaC; full map %#v", got["infrastructure-as-code"], got)
	}
	// Terraform has one spelling, so there is nothing to align and nothing to report.
	if _, ok := got["terraform"]; ok {
		t.Errorf("terraform is in the map, but it has no interchangeable spellings: %#v", got)
	}
}

func TestPreferredFromText_UnknownIgnored(t *testing.T) {
	got := PreferredFromText("Must know BlorpleDB and the FluxCapacitor framework.")
	if len(got) != 0 {
		t.Fatalf("got %#v, want empty — unknown jargon must not invent spellings", got)
	}
}

// The spelling written into a CV comes from the curated table, so a vacancy that shouts
// its requirements does not make the candidate shout back.
func TestPreferredFromText_JDCasingNotCopied(t *testing.T) {
	got := PreferredFromText("REQUIREMENTS: KUBERNETES, POSTGRESQL, TYPESCRIPT.")
	for canonical, want := range map[string]string{
		"kubernetes": "Kubernetes",
		"postgresql": "PostgreSQL",
		"typescript": "TypeScript",
	} {
		if got[canonical] != want {
			t.Errorf("%s = %q, want %q", canonical, got[canonical], want)
		}
	}
}

// A description is raw ATS HTML and stripMarkup leaves a space where each tag was, so a
// spelling cut out of the source would carry the markup's whitespace onto the page.
func TestPreferredFromText_MarkupWhitespaceNotCarried(t *testing.T) {
	got := PreferredFromText("<ul><li>Strong <b>infrastructure</b> as code (Terraform)</li>\n<li>Runs on\nKubernetes</li></ul>")
	for canonical, want := range map[string]string{
		"infrastructure-as-code": "infrastructure as code",
		"kubernetes":             "Kubernetes",
	} {
		if got[canonical] != want {
			t.Errorf("%s = %q, want %q", canonical, got[canonical], want)
		}
	}
	for canonical, surface := range got {
		if strings.ContainsAny(surface, "\n\t") || strings.Contains(surface, "  ") {
			t.Errorf("%s = %q carries markup whitespace", canonical, surface)
		}
	}
}

// A vacancy in any language must not be able to crash the tailor request or slice a
// spelling in half. Every rune here lowercases to a different byte length.
func TestPreferredFromText_NonASCIIIsSafe(t *testing.T) {
	for _, prefix := range []string{"İstanbul", strings.Repeat("İ", 50), strings.Repeat("Ⱥ", 40), strings.Repeat("K", 30), "ẞ"} {
		text := prefix + " team running Kubernetes with infrastructure as code"
		got := PreferredFromText(text)
		if got["kubernetes"] != "Kubernetes" {
			t.Errorf("prefix %q: kubernetes = %q, want Kubernetes", prefix, got["kubernetes"])
		}
		if got["infrastructure-as-code"] != "infrastructure as code" {
			t.Errorf("prefix %q: iac = %q, want the spelled-out form", prefix, got["infrastructure-as-code"])
		}
	}
}

// The alias tables are many-to-one on purpose: they fold narrower terms onto a broader
// canonical to keep the search facet whole. Inverting them would answer "ruby" with
// "Ruby on Rails" and put a framework on a CV that only claims the language.
func TestPreferredFromText_NarrowerTermsAreNotSpellings(t *testing.T) {
	cases := []struct{ text, canonical string }{
		{"We are a Ruby on Rails shop.", "ruby"},
		{"Java 17 with Spring Boot microservices.", "spring"},
		{"Senior ASP.NET engineer wanted.", "dotnet"},
		{"You will design RESTful APIs.", "rest"},
		{"Embedded C/C++ role.", "cpp"},
		{"Looking for a C developer.", "c"},
		{"T-SQL reporting on SQL Server.", "sql-server"},
		{"Build LLM features with retrieval augmented generation.", "rag"},
		{"Frontend with HTML5 and CSS3.", "html"},
		{"AngularJS legacy plus Angular today.", "angular"},
	}
	for _, tc := range cases {
		if surface, ok := PreferredFromText(tc.text)[tc.canonical]; ok {
			t.Errorf("%q offered %s = %q; that family is not interchangeable", tc.text, tc.canonical, surface)
		}
	}
}

// Every curated family must survive the round trip: each spelling has to resolve back to
// the canonical it is filed under, or alignment would rewrite one skill into another.
func TestInterchangeableSurfaces_ResolveToTheirCanonical(t *testing.T) {
	for canonical, spellings := range interchangeableSurfaces {
		if len(spellings) < 2 {
			t.Errorf("%s lists %d spelling(s); a family with one spelling has nothing to align", canonical, len(spellings))
		}
		for _, s := range spellings {
			got := Canonicalize([]string{s}, WithResumeAcronyms())
			if len(got) != 1 || got[0] != canonical {
				t.Errorf("Canonicalize(%q) = %v, want [%s] — the family would rewrite one skill into another", s, got, canonical)
			}
		}
	}
}

func TestPreferredFromText_WeakSpellingNeedsCorroboration(t *testing.T) {
	if got := PreferredFromText("You must go the extra mile and react to change."); len(got) != 0 {
		t.Errorf("got %#v, want empty — bare English words are not skill spellings", got)
	}
	if got := PreferredFromText("Golang services on Kubernetes."); got["go"] != "Golang" {
		t.Errorf("go = %q, want Golang once a real technology corroborates it", got["go"])
	}
}

func TestProseSurfaces_ExcludeShortTokens(t *testing.T) {
	if got := ProseSurfaces("typescript"); slices.Contains(got, "TS") {
		t.Errorf("ProseSurfaces(typescript) = %v, want no bare two-letter token", got)
	}
	if got := ProseSurfaces("infrastructure-as-code"); len(got) != 2 {
		t.Errorf("ProseSurfaces(infrastructure-as-code) = %v, want both spellings", got)
	}
	if got := ProseSurfaces("ruby"); got != nil {
		t.Errorf("ProseSurfaces(ruby) = %v, want nil — not an interchangeable family", got)
	}
}

func TestPreferredFromText_Empty(t *testing.T) {
	if got := PreferredFromText(""); got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
	if got := PreferredFromText("<p></p>   "); got != nil {
		t.Fatalf("got %#v, want nil", got)
	}
}

func TestPreferredFromText_Deterministic(t *testing.T) {
	text := "IaC with Kubernetes, k8s tooling, React.js and Node.js."
	first := PreferredFromText(text)
	for i := 0; i < 50; i++ {
		if got := PreferredFromText(text); !reflect.DeepEqual(first, got) {
			t.Fatalf("not deterministic on run %d:\n%#v\n%#v", i, first, got)
		}
	}
}
