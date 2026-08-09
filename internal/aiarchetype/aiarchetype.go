// Package aiarchetype derives a job's AI skill-signature archetype
// deterministically from its already-resolved skills and category — the same
// dict-only doctrine as internal/classify, internal/roletag, and
// internal/skilltag: it emits one of a curated set of archetype slugs for what
// it can resolve and nothing for what it cannot (it never guesses, never calls
// a model).
//
// The six archetypes approximate the stable clusters a skill-signature k-means
// analysis of the AI engineering job market found (see the field guide this
// package's design is based on): RAG app builder, agent builder, cloud/ML
// platform engineer, ML trainer/researcher, full-stack AI engineer, and
// DevOps/infra engineer. Unlike that clustering, this package is a small
// ordered rule set over internal/skilltag's existing canonical vocabulary — a
// deliberate simplification consistent with the project's dict-only doctrine,
// which forbids a model call for a production facet.
package aiarchetype

// inScopeCategories are the categories aiarchetype evaluates. Every other
// category yields no archetype, regardless of skills — the archetypes were
// derived from AI-titled job postings and are not meaningful outside them.
var inScopeCategories = map[string]bool{
	"ai_engineering": true,
	"ml_ai":          true,
}

// rule is one archetype's skill-signature test. Rules are evaluated in a fixed
// priority order (see rules below): the first whose match reports true wins,
// so an earlier rule's signature must be a superset-safe subset of any later,
// broader rule it could otherwise be swallowed by.
type rule struct {
	archetype string
	match     func(has func(string) bool) bool
}

// rules are evaluated in this order — most AI-distinctive signature first,
// most generic/overlapping last — so a job matching an earlier rule is claimed
// before a later, broader rule sees it. See design.md decision 3 for the
// rationale behind each rule and this ordering.
var rules = []rule{
	{
		"rag_app_builder",
		func(has func(string) bool) bool {
			return has("rag") && (has("langchain") || has("langgraph") || has("llamaindex")) && has("vector-databases")
		},
	},
	{
		"agent_builder",
		func(has func(string) bool) bool {
			return has("agentic-ai") && has("prompt-engineering") && has("rag")
		},
	},
	{
		"cloud_ml_platform_engineer",
		func(has func(string) bool) bool {
			return has("mlops") && has("kubernetes") && (has("pytorch") || has("tensorflow"))
		},
	},
	{
		"ml_trainer_researcher",
		func(has func(string) bool) bool {
			return (has("pytorch") || has("tensorflow")) && !has("rag") && !has("agentic-ai")
		},
	},
	{
		"fullstack_ai_engineer",
		func(has func(string) bool) bool {
			return (has("react") || has("nodejs")) && (has("llm") || has("openai") || has("anthropic"))
		},
	},
	{
		"devops_infra_engineer",
		func(has func(string) bool) bool {
			return has("terraform") && has("kubernetes") && has("docker") && has("ci-cd")
		},
	},
}

// Derive returns the single skill-signature archetype a job's skills and
// category resolve to, or "" if category is out of scope or no rule matches.
// It never returns more than one archetype: the archetypes are the
// mutually-exclusive clusters the field guide's analysis found, not additive
// facets, so the first (highest-priority) matching rule wins.
func Derive(skills []string, category string) string {
	if !inScopeCategories[category] {
		return ""
	}
	set := make(map[string]bool, len(skills))
	for _, s := range skills {
		set[s] = true
	}
	has := func(s string) bool { return set[s] }
	for _, r := range rules {
		if r.match(has) {
			return r.archetype
		}
	}
	return ""
}
