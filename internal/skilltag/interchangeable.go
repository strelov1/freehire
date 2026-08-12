package skilltag

import (
	"cmp"
	"slices"
	"strings"
	"unicode/utf8"
)

// interchangeableSurfaces lists, per canonical skill, the spellings that name the SAME
// skill and may therefore be written over one another when a CV is aligned to a vacancy's
// wording.
//
// It is deliberately a fraction of the alias tables, and it is a separate table rather
// than an inversion of them. Those tables are many-to-one ON PURPOSE: "spring boot",
// "ruby on rails", "asp.net", "c developer" and "restful apis" are folded onto a broader
// canonical to keep the SEARCH FACET un-fragmented, not because they mean the same thing.
// Rewriting a CV through them puts a technology on the page the candidate never claimed —
// a "Ruby" chip becomes "Ruby on Rails", a "Spring" chip becomes "Spring Boot", and a "C"
// chip becomes the job-title phrase "C developer".
//
// A canonical belongs here only when every spelling below is one skill written another
// way: an acronym, a plural, a vendor rename, or a British/American spelling. Version
// numbers (html/html5, oauth/oauth2, ga4) and near-neighbours (angular/angularjs,
// vbnet/visual basic, ci/cd vs continuous integration) are NOT interchangeable and stay
// out. When in doubt, leave it out — the cost of omission is a CV that keeps the
// candidate's own wording, which is the safe direction.
//
// The first spelling is not privileged; what a match writes is the spelling as listed
// here, never a span sliced out of the job description — so a SHOUTY requirements heading
// or a line break inside a phrase cannot reach the page.
var interchangeableSurfaces = map[string][]string{
	"accessibility":             {"accessibility", "a11y"},
	"ab-testing":                {"A/B testing", "AB testing"},
	"containerization":          {"containerization", "containerisation"},
	"cryptocurrency":            {"cryptocurrency", "cryptocurrencies"},
	"data-modeling":             {"data modeling", "data modelling"},
	"data-pipelines":            {"data pipelines", "data pipeline"},
	"data-visualization":        {"data visualization", "data visualisation"},
	"data-warehousing":          {"data warehousing", "data warehouse"},
	"design-patterns":           {"design patterns", "design pattern"},
	"dimensional-modeling":      {"dimensional modeling", "dimensional modelling"},
	"docs-as-code":              {"docs as code", "docs-as-code"},
	"ecommerce":                 {"ecommerce", "e commerce"},
	"event-driven-architecture": {"event driven architecture", "event driven"},
	"express":                   {"Express", "Express.js", "ExpressJS"},
	"firewall":                  {"firewalls", "firewall"},
	"fsharp":                    {"F#", "FSharp"},
	"gcp":                       {"Google Cloud Platform", "Google Cloud", "GCP"},
	"generative-ai":             {"generative AI", "GenAI", "gen AI"},
	"go":                        {"Go", "Golang"},
	"google-ads":                {"Google Ads", "Google AdWords", "AdWords"},
	"google-search-console":     {"Google Search Console", "Search Console"},
	"hyper-v":                   {"Hyper-V", "Hyper V"},
	"infrastructure-as-code":    {"infrastructure as code", "IaC"},
	"iso-27001":                 {"ISO 27001", "ISO27001"},
	"kubernetes":                {"Kubernetes", "K8s"},
	"link-building":             {"link building", "linkbuilding"},
	"llamaindex":                {"LlamaIndex", "Llama Index"},
	"llm":                       {"LLMs", "LLM"},
	"looker-studio":             {"Looker Studio", "Data Studio"},
	"machine-learning":          {"machine learning", "ML"},
	"meta-ads":                  {"Meta Ads", "Facebook Ads"},
	"microservices":             {"microservices", "microservice"},
	"microsoft-access":          {"Microsoft Access", "MS Access"},
	"mitre-attack":              {"MITRE ATT&CK", "MITRE Attack"},
	"mongodb":                   {"MongoDB", "Mongo"},
	"neural-networks":           {"neural networks", "neural network"},
	"newrelic":                  {"New Relic", "NewRelic"},
	"nextjs":                    {"Next.js", "NextJS"},
	"nlp":                       {"natural language processing", "NLP"},
	"nodejs":                    {"Node.js", "NodeJS", "Node JS"},
	"opentelemetry":             {"OpenTelemetry", "Open Telemetry"},
	"openid":                    {"OpenID", "OIDC"},
	"penetration-testing":       {"penetration testing", "pentesting", "pentest"},
	"plsql":                     {"PL/SQL", "PLSQL"},
	"postgresql":                {"PostgreSQL", "Postgres"},
	"powerapps":                 {"Power Apps", "PowerApps"},
	"powerbi":                   {"Power BI", "PowerBI"},
	"pre-sales":                 {"pre-sales", "presales"},
	"predictive-modeling":       {"predictive modeling", "predictive modelling"},
	"react":                     {"React", "React.js", "ReactJS"},
	"recommendation-systems":    {"recommendation systems", "recommendation system"},
	"scikit-learn":              {"scikit-learn", "scikit"},
	"smart-contracts":           {"smart contracts", "smart contract"},
	"tcp-ip":                    {"TCP/IP", "TCP IP"},
	"tdd":                       {"test driven development", "TDD"},
	"test-automation":           {"test automation", "automated testing"},
	"threejs":                   {"Three.js", "ThreeJS"},
	"typescript":                {"TypeScript", "TS"},
	"unit-testing":              {"unit testing", "unit test"},
	"virtualization":            {"virtualization", "virtualisation"},
	"vue":                       {"Vue", "Vue.js", "VueJS"},
	"wasm":                      {"Web Assembly", "Wasm"},
	"wireframing":               {"wireframing", "wireframes"},
}

// surfaceVariant is one curated spelling, compiled once: the form written into a CV, the
// matcher that finds it in a vacancy, and the two judgements about how safe that spelling
// is out of context.
type surfaceVariant struct {
	display   string
	matcher   phraseMatcher
	weak      bool // needs another technology named nearby before it counts as a hit
	proseSafe bool // may be rewritten inside a sentence, not just inside a skill chip
}

// newSurfaceVariant compiles one spelling. The two judgements it makes differ, and the
// difference is the point: "F#" is unmistakable in a sentence yet too short to trust as
// the only skill a vacancy named, while "react" is long enough to be a hit but is also
// ordinary English, so rewriting every "react" in a bullet would turn "we react to
// incidents" into a framework.
func newSurfaceVariant(canonical, display string) surfaceVariant {
	lower := strings.ToLower(display)
	english := ambiguousWords[lower]
	// Lowercased job text cannot tell "ML" the field from "ml" the unit.
	short := utf8.RuneCountInString(lower) <= 2
	punctuated := strings.ContainsAny(lower, " \t_-/.#+&")
	return surfaceVariant{
		display:   display,
		matcher:   compilePhraseMatcher(canonical, lower),
		weak:      english || short,
		proseSafe: !english && (punctuated || !short),
	}
}

// surfaceIndex compiles interchangeableSurfaces once at startup, and proseDisplays is the
// ProseSurfaces answer materialised alongside it — that one is read per canonical per prose
// string during an alignment, and rebuilding the slice there was the dominant cost of the
// whole pass.
//
// Each canonical's variants are ordered longest-first, so a vacancy that writes both
// "Kubernetes" and "k8s" resolves to the fuller spelling, and prose rewriting cannot match
// a short spelling inside a longer one.
var surfaceIndex, proseDisplays = func() (map[string][]surfaceVariant, map[string][]string) {
	index := make(map[string][]surfaceVariant, len(interchangeableSurfaces))
	prose := make(map[string][]string, len(interchangeableSurfaces))
	for canonical, displays := range interchangeableSurfaces {
		variants := make([]surfaceVariant, 0, len(displays))
		for _, d := range displays {
			variants = append(variants, newSurfaceVariant(canonical, d))
		}
		slices.SortFunc(variants, func(a, b surfaceVariant) int {
			return cmp.Or(
				cmp.Compare(utf8.RuneCountInString(b.display), utf8.RuneCountInString(a.display)),
				strings.Compare(a.display, b.display),
			)
		})
		index[canonical] = variants
		for _, v := range variants {
			if v.proseSafe {
				prose[canonical] = append(prose[canonical], v.display)
			}
		}
	}
	return index, prose
}()

// ProseSurfaces returns the curated spellings of canonical that may be rewritten inside
// summary and bullet prose, longest first. A skill chip needs no such list — Canonicalize
// already resolves whatever the chip says. The result is shared and must not be modified.
func ProseSurfaces(canonical string) []string {
	return proseDisplays[canonical]
}
