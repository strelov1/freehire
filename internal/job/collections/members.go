package collections

import (
	_ "embed"
	"fmt"
	"slices"
	"sort"

	"github.com/strelov1/freehire/internal/dict/normalize"
)

// AICompanySlugs is a hand-curated list of prominent AI-native companies. "AI company" is a
// fact about the company (all its roles belong here), distinct from the job-level
// `enrichment.category = ml_ai` facet. Entries are canonical company slugs
// (normalize.CompanySlug); commonly variant names are all listed so the match lands whatever
// name our adapters use. Unmatched entries are logged.
var AICompanySlugs = []string{
	// Foundation-model labs.
	"openai", "anthropic", "mistral", "mistral-ai", "cohere",
	"ai21-labs", "ai21", "xai", "stability-ai", "inflection-ai",
	"reka-ai", "contextual-ai", "deepmind",
	// ML infrastructure / platforms / tooling.
	"hugging-face", "scale-ai", "weights-biases", "together-ai", "replicate",
	"pinecone", "anyscale", "baseten", "fireworks-ai", "lambda-labs",
	"langchain", "llamaindex", "modal-labs",
	// Applied-AI products.
	"perplexity", "perplexity-ai", "character-ai", "midjourney", "runway",
	"runway-ml", "elevenlabs", "eleven-labs", "synthesia", "jasper",
	"glean", "harvey", "harvey-ai", "suno", "descript", "cresta",
	"anysphere", "cursor",
}

// AINativeSlugs is the curated AI-native cohort sourced from the remotepilot.dev directory.
// Entries are canonical company slugs (normalize.CompanySlug) verified against the exact name
// our adapters ingest, e.g. "openrouter.ai" → openrouter-ai. Only that form is listed: a
// brand-name variant (openrouter) resolves to a duplicate company row and would double the
// cohort. It overlaps AICompanySlugs by design.
var AINativeSlugs = []string{
	// Model & inference APIs.
	"deepgram", "cohere", "together-ai", "fireworks-ai", "openrouter-ai",
	// Vector databases & data infrastructure.
	"pinecone", "qdrant-tech", "weaviate", "supabase", "planetscale", "tensorwave",
	// Agent & developer tooling.
	"langchain", "composio", "livekit", "linear", "resend",
	// Other AI-native companies.
	"trmlabs", "jasper", "concentrate-ai", "andromeda",
	"infinity-constellation", "vclusterlabs", "urun", "tollbit",
}

// Mag7Slugs is the Magnificent Seven mega-cap tech cohort. Name variants (alphabet/google,
// meta/facebook) are both listed. A deliberate subset of BigTechSlugs.
var Mag7Slugs = []string{
	"apple", "microsoft", "google", "alphabet",
	"amazon", "meta", "facebook", "nvidia", "tesla",
}

// BigTechSlugs is the hand-curated company-slug list for the bigtech collection.
// "Big Tech" is the broad "tech giants" sense: the Magnificent Seven plus the next tier of
// large, established public technology companies. It deliberately excludes high-growth
// startups/unicorns (Stripe, Airbnb, Uber, …), which surface in `yc` instead. Name variants
// are all listed. Entries are canonical company slugs (normalize.CompanySlug), matched against
// the catalogue at import time; unmatched entries are logged.
var BigTechSlugs = []string{
	// Magnificent Seven (+ name variants).
	"apple",
	"microsoft",
	"google",
	"alphabet",
	"amazon",
	"meta",
	"facebook",
	"nvidia",
	"tesla",
	// Established public tech giants.
	"netflix",
	"oracle",
	"salesforce",
	"ibm",
	"intel",
	"adobe",
	"cisco",
	"sap",
	"qualcomm",
	"broadcom",
	"amd",
	"dell",
	"servicenow",
}

// easternRootsData is the embedded membership file for the eastern-roots collection:
// companies with Eastern European / Russian-speaking founding roots that operate
// internationally. It is our own curated fact, so it is embedded rather than fetched. One
// canonical company slug (normalize.CompanySlug) per line; matched against the catalogue at
// import time, unmatched slugs are logged by name.
//
//go:embed eastern_roots.txt
var easternRootsData []byte

// indianRootsData is the same shape for the indian-roots collection (IT-services majors, the
// domestic product ecosystem, and Indian-founder companies headquartered abroad). No
// maintained open dataset of these exists, so it is embedded too.
//
//go:embed indian_roots.txt
var indianRootsData []byte

// MaxLoggedUnmatched bounds the unmatched entries a hand list reports by name, so a
// mis-edited list cannot flood a run's output.
const MaxLoggedUnmatched = 50

// MatchStat is how one collection's candidates fell out: how many companies earned
// the tag, how many candidates matched no company at all, how many matched a company
// but failed the gate, and how many register rows the ambiguity guard dropped.
// Gated and Ambiguous are zero for a gateless editorial collection — they are what a
// dry run is read for.
type MatchStat struct {
	Matched, Unmatched int
	Gated, Ambiguous   int
	// Mismatch counts companies whose name matched a member the source describes as
	// a different organisation — another website (see sameSite) or a headcount the
	// company's own postings outgrow (see plausibleSize).
	Mismatch       int
	UnmatchedNames []string
	// MismatchNames shows what was refused and why, so a run's log lets a person
	// judge whether it refused the right companies.
	MismatchNames []string
}

// Members resolves this collection's candidate records to the company slugs that
// earn its tag, and reports how the candidates fell out. Matched slugs are
// deduplicated and sorted; unmatched names are returned verbatim, in input order,
// for logging.
//
// Both kinds match on normalize.CompanySlug, the catalogue's key. They differ in the
// ambiguity drop: a credential first discards rows whose name identifies more than one
// organisation.
//
// A record the source describes as a different organisation — another website, or a
// headcount the company's postings outgrow — is refused before the gate (see
// mismatch): the name matched, the organisation did not.
//
// A company is tagged when ANY of its records passes the gate: a register lists an
// organisation once per route, so a work-route row must win over an earlier temporary one.
func (c Collection) Members(records []Record, companies map[string]Company) ([]string, MatchStat) {
	var stat MatchStat
	// Both kinds slug the same way, because both look the result up in a map keyed by the
	// CATALOGUE's company slug — and that is normalize.CompanySlug. Editorial matching used
	// normalize.Slug until the catalogue started stripping corporate forms; a dataset naming
	// "Acme Robotics Limited" then asked for a slug ingest can no longer produce, matched
	// nothing, and reported no error. What still separates a credential is the ambiguity drop
	// and the gates, not the spelling.
	if c.Kind == KindCredential {
		before := len(records)
		records = DropAmbiguous(records, c.identityKey())
		stat.Ambiguous = before - len(records)
	}

	admitted := make(map[string]struct{}, len(records))
	gatedOut := make(map[string]struct{}, len(records))
	mismatched := make(map[string]struct{}, len(records))
	for _, r := range records {
		slug := normalize.CompanySlug(r.Name)
		company, known := companies[slug]
		if !known && c.Country != "" {
			if parent := withoutCountrySuffix(slug, c.Country); parent != "" {
				slug = parent
				company, known = companies[slug]
			}
		}
		if slug == "" || !known {
			stat.Unmatched++
			stat.UnmatchedNames = append(stat.UnmatchedNames, r.Name)
			continue
		}
		why := mismatch(r, company)
		if slices.Contains(c.Exclude, slug) {
			why = "excluded by hand"
		}
		if why != "" {
			if _, seen := mismatched[slug]; !seen && len(stat.MismatchNames) < MaxLoggedUnmatched {
				stat.MismatchNames = append(stat.MismatchNames, fmt.Sprintf("%s (%s)", r.Name, why))
			}
			mismatched[slug] = struct{}{}
			continue
		}
		if !c.Admits(company, r) {
			gatedOut[slug] = struct{}{}
			continue
		}
		admitted[slug] = struct{}{}
	}

	matched := make([]string, 0, len(admitted))
	for slug := range admitted {
		matched = append(matched, slug)
	}
	sort.Strings(matched)
	stat.Matched = len(matched)
	for slug := range gatedOut {
		// A company with both an admitted and a rejected row is not gated out.
		if _, ok := admitted[slug]; !ok {
			stat.Gated++
		}
	}
	for slug := range mismatched {
		if _, ok := admitted[slug]; !ok {
			stat.Mismatch++
		}
	}
	return matched, stat
}

// identityKey is this collection's dataset organisation discriminator, if any.
func (c Collection) identityKey() string {
	if c.Dataset == nil {
		return ""
	}
	return c.Dataset.IdentityKey
}

// Reconcile computes a company's new collection set: it removes every tag in
// `managed` from `current` (so a tag the company no longer qualifies for is
// dropped) and adds `want` (the managed tags it now qualifies for, a subset of
// `managed`), preserving any tag in `current` that the registry does not manage.
// The result is deduplicated, sorted, and always non-nil.
func Reconcile(current, managed, want []string) []string {
	isManaged := make(map[string]struct{}, len(managed))
	for _, m := range managed {
		isManaged[m] = struct{}{}
	}
	set := make(map[string]struct{}, len(current)+len(want))
	for _, c := range current {
		if _, m := isManaged[c]; !m {
			set[c] = struct{}{}
		}
	}
	for _, w := range want {
		set[w] = struct{}{}
	}
	out := make([]string, 0, len(set))
	for s := range set {
		out = append(out, s)
	}
	sort.Strings(out)
	return out
}
