// Package aiarchetype derives a job's AI skill-signature archetype from its resolved
// skills and category. Dict-only like internal/dict/classify and skilltag: it emits
// one of a curated set of slugs for what it can resolve, nothing otherwise, and never
// calls a model.
//
// The six archetypes (RAG app builder, agent builder, cloud/ML platform engineer, ML
// trainer/researcher, full-stack AI engineer, DevOps/infra engineer) approximate
// clusters found in AI-job skill data; here they are a small ordered rule set over
// internal/dict/skilltag's canonical vocabulary.
package aiarchetype

// inScopeCategories are the categories evaluated; the archetypes come from AI-titled
// postings and mean nothing elsewhere.
var inScopeCategories = map[string]bool{
	"ai_engineering": true,
	"ml_ai":          true,
}

// rule is one archetype's skill-signature test. The first matching rule in priority
// order wins, so an earlier signature must not be swallowed by a later, broader one.
type rule struct {
	archetype string
	match     func(has func(string) bool) bool
}

// rules run most AI-distinctive signature first, most generic last.
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

// Archetypes returns every archetype slug the rule table can derive, in priority order.
func Archetypes() []string {
	out := make([]string, len(rules))
	for i, r := range rules {
		out[i] = r.archetype
	}
	return out
}

// Derive returns the single skill-signature archetype a job's skills and
// category resolve to, or "" if category is out of scope or no rule matches.
// It never returns more than one: archetypes are mutually exclusive, so the first
// matching rule wins.
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
