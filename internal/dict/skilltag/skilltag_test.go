package skilltag

import (
	"reflect"
	"testing"
)

func TestNormalizeStripsHTMLAndLowercases(t *testing.T) {
	got := normalize("<div><p>Senior <b>Go</b> Engineer</p></div>")
	// Two spaces between words: each surrounding tag is replaced by one space.
	// Separators are NOT collapsed here — the phrase matcher handles that.
	want := "senior  go  engineer"
	if got != want {
		t.Fatalf("normalize = %q, want %q", got, want)
	}
}

func TestWordTokens(t *testing.T) {
	got := wordTokens("go, node.js & c++17")
	want := []string{"go", "node", "js", "c", "17"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("wordTokens = %#v, want %#v", got, want)
	}
}

func TestParse(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"plain alias", "We use Golang and PostgreSQL.", []string{"go", "postgresql"}},
		{"dedup + sort", "react, React.js, REACT", []string{"react"}},
		{"punctuated", "Strong C++ and C# with .NET.", []string{"cpp", "csharp", "dotnet"}},
		{"node variants", "node.js / nodejs / node js", []string{"nodejs"}},
		{"multiword", "React Native and CI/CD pipelines", []string{"ci-cd", "react", "react-native"}},
		{"ambiguous word rejected", "Please go to the careers page in C.", nil},
		{"ambiguous via qualifier", "5y as a C developer", []string{"c"}},
		{"objective-c does not leak bare c", "Objective-C developer", []string{"objective-c"}},
		{"objective-c alongside real C still tags both", "Objective-C and C developer", []string{"c", "objective-c"}},
		{"word boundary", "a reaction to going rusty", nil},
		{"html stripped", "<p>Kubernetes</p><a href='k8s'>k8s</a>", []string{"kubernetes"}},
		{"empty", "", nil},
		{"dotted domain not matched", "see docs at foo.asp.net for details", nil},
		{"sentence-end periods ok", "We use C#. Also ASP.NET.", []string{"csharp", "dotnet"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Separator-insensitive matching (skilltag-matching-fixes): '-'/'_' between
// alphanumerics are equivalent to a space, so hyphenated/underscored multi-word
// terms resolve like their space form — without touching punctuated canonicals.
func TestParse_ITCompanyRoleSkills(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"recruiting", "Sourcing with boolean search and employer branding", []string{"boolean-search", "employer-branding"}},
		{"finance", "Own financial modeling and revenue recognition in NetSuite", []string{"financial-modeling", "netsuite", "revenue-recognition"}},
		{"business analysis", "Requirements gathering, process modeling and user stories", []string{"process-modeling", "requirements-gathering", "user-stories"}},
		{"customer success", "Drive customer onboarding and churn prevention via Gainsight", []string{"churn-prevention", "customer-onboarding", "gainsight"}},
		{"technical writing", "API documentation with a docs-as-code workflow", []string{"api", "api-documentation", "docs-as-code"}},
		{"unknown emits nothing", "great communicator and team player", nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Boilerplate sections — the pay-transparency block, the GDPR notice, the ATS
// footer — appear in postings for EVERY role, so a dictionary term that matches
// them tags the whole corpus instead of describing the job (an ML Platform
// Engineer came back with "compensation-and-benefits"). Such a term is not a
// requirement signal at all, so it is out of the vocabulary entirely; the same
// applies to an ATS name whose bare token doubles as an English word
// ("a key lever", "a flexible workday").
func TestParse_BoilerplateSectionsAreNotSkills(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"pay transparency block", "Compensation and Benefits: we offer a competitive salary and equity.", nil},
		{"privacy notice", "Data Privacy Notice: we process your application data as described here.", nil},
		{"ats footer", "Powered by Greenhouse. Apply via Lever.", nil},
		{"culture blurb", "We invest in employee engagement and do our due diligence.", nil},
		{"boilerplate does not pollute a real stack", "We use Python and Kafka. A flexible workday, applications via Greenhouse.", []string{"kafka", "python"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// TestParse_AccentedWordsAreNotSkills pins the two places a word boundary is
// decided — the phrase pass (wordmatch) and the word pass (wordTokens) — against
// text whose letters are not ASCII. Both read a curated alias out of the middle of
// an inflected foreign word before this: the Hungarian "elkészítése" ("preparing
// it") yielded elk, on 18 of 110 postings sampled from a live Hungarian IT crawl.
// The failure is not Hungarian-specific — any diacritic or non-Latin script ends an
// ASCII-only token the same way.
func TestParse_AccentedWordsAreNotSkills(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"hungarian inflection yields no elk", "A dokumentáció elkészítése a feladatod.", nil},
		{"hungarian inflection beside a real stack", "Rendszertervek elkészítése Python nyelven.", []string{"python"}},
		{"a real ELK mention still tags", "ELK stack üzemeltetése", []string{"elk"}},
		{"lowercase elk as a whole word still tags", "we run elk for logging", []string{"elk"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := Parse(tc.in); !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

func TestParse_SeparatorInsensitive(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want []string
	}{
		{"hyphen multiword", "Experience with distributed-systems at scale.", []string{"distributed-systems"}},
		{"underscore multiword", "distributed_systems everywhere", []string{"distributed-systems"}},
		{"space still resolves", "distributed systems everywhere", []string{"distributed-systems"}},
		{"hyphen and space dedup to one", "distributed-systems and distributed systems", []string{"distributed-systems"}},
		{"machine-learning hyphen", "machine-learning pipelines", []string{"machine-learning"}},
		{"ci-cd hyphen", "CI-CD pipeline", []string{"ci-cd"}},
		{"react-native hyphen", "React-Native apps", []string{"react", "react-native"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := Parse(tc.in)
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("Parse(%q) = %#v, want %#v", tc.in, got, tc.want)
			}
		})
	}
}

// Case-preserving acronym pass (skilltag-matching-fixes): a curated shared tier
// (ML) resolves everywhere; a résumé-scoped tier (RAG) resolves only under the
// résumé option so it never tags job facets ("RAG status").
func TestParse_Acronyms(t *testing.T) {
	has := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}

	// Shared acronym ML → machine-learning, applied by the default Parse.
	if got := Parse("Senior ML Engineer with Python"); !has(got, "machine-learning") {
		t.Errorf("Parse ML = %v, want machine-learning", got)
	}
	// Whole-word only: ML embedded in HTML/HTMl must not fire; lowercase ml must not.
	if got := Parse("We write HTML and CSS"); has(got, "machine-learning") {
		t.Errorf("Parse HTML = %v, must not emit machine-learning", got)
	}
	if got := Parse("Mix 500 ml of solution"); has(got, "machine-learning") {
		t.Errorf("Parse 'ml' lowercase = %v, must not emit machine-learning", got)
	}

	// Résumé-scoped acronym RAG → rag, ONLY under WithResumeAcronyms.
	if got := Parse("Built RAG pipelines over pgvector", WithResumeAcronyms()); !has(got, "rag") {
		t.Errorf("Parse RAG (resume) = %v, want rag", got)
	}
	// Default (job) parsing must NOT tag RAG — this is what protects the existing
	// "RAG status" job guard and keeps job facets unchanged.
	if got := Parse("Built RAG pipelines over pgvector"); has(got, "rag") {
		t.Errorf("Parse RAG (default) = %v, must not emit rag", got)
	}
	if got := Parse("We report RAG status weekly"); has(got, "rag") {
		t.Errorf("Parse 'RAG status' (default) = %v, must not emit rag", got)
	}
}

// Category-scoped acronym pass (refine-ai-role-classification): RAG resolves on
// job text when the caller supplies a job category on the acronym's own
// allow-list (ai_engineering, ml_ai), and does not resolve otherwise — the
// category already evidences an AI-flavored posting, so it substitutes for
// corroboration without reopening the "RAG status" collision catalogue-wide.
func TestParse_CategoryScopedAcronyms(t *testing.T) {
	has := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}

	if got := Parse("Built RAG pipelines over pgvector", WithAcronymCategory("ai_engineering")); !has(got, "rag") {
		t.Errorf("Parse RAG (category ai_engineering) = %v, want rag", got)
	}
	if got := Parse("Built RAG pipelines over pgvector", WithAcronymCategory("ml_ai")); !has(got, "rag") {
		t.Errorf("Parse RAG (category ml_ai) = %v, want rag", got)
	}
	if got := Parse("We report RAG status weekly", WithAcronymCategory("backend")); has(got, "rag") {
		t.Errorf("Parse 'RAG status' (category backend) = %v, must not emit rag", got)
	}
	if got := Parse("Built RAG pipelines over pgvector"); has(got, "rag") {
		t.Errorf("Parse RAG (no category option) = %v, must not emit rag", got)
	}
}

// Agile/PM certification acronyms (role-taxonomy-alias-gaps): CSM, PSM, PMP and
// SAFe each collide with another meaning in general job text (Customer Success
// Manager, common English word, other short-string noise), so they resolve only
// on job text already categorized project_management — the same category-scoped
// shape as RAG. The spelled-out phrase forms are unambiguous and resolve
// regardless of category.
func TestParse_AgilePMCertificationAcronyms(t *testing.T) {
	has := func(hay []string, needle string) bool {
		for _, h := range hay {
			if h == needle {
				return true
			}
		}
		return false
	}

	cases := []struct {
		text      string
		canonical string
	}{
		{"CSM certification required", "certified-scrummaster"},
		{"PSM I certified", "professional-scrum-master"},
		{"PMP holders preferred", "pmp"},
		{"SAFe experience a plus", "safe-agile"},
		{"SAFE experience a plus", "safe-agile"}, // ATS all-caps title rendering
	}
	for _, c := range cases {
		if got := Parse(c.text, WithAcronymCategory("project_management")); !has(got, c.canonical) {
			t.Errorf("Parse(%q, project_management) = %v, want %s", c.text, got, c.canonical)
		}
		if got := Parse(c.text, WithAcronymCategory("backend")); has(got, c.canonical) {
			t.Errorf("Parse(%q, backend) = %v, must not emit %s", c.text, got, c.canonical)
		}
		if got := Parse(c.text); has(got, c.canonical) {
			t.Errorf("Parse(%q, no category) = %v, must not emit %s", c.text, got, c.canonical)
		}
	}

	// Spelled-out phrase forms are unambiguous strong matches, no category needed.
	phraseCases := []struct{ text, canonical string }{
		{"Certified ScrumMaster (CSM)", "certified-scrummaster"},
		{"Certified Scrum Master (CSM)", "certified-scrummaster"},
		{"Holds a Professional Scrum Master certification", "professional-scrum-master"},
		{"Project Management Professional preferred", "pmp"},
		{"Scaled Agile Framework experience", "safe-agile"},
	}
	for _, c := range phraseCases {
		if got := Parse(c.text); !has(got, c.canonical) {
			t.Errorf("Parse(%q) = %v, want %s", c.text, got, c.canonical)
		}
	}
}
