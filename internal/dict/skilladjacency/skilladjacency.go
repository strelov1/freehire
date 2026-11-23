// Package skilladjacency is the curated skill-adjacency dictionary: a low-level,
// I/O-free relation between canonical skilltag slugs that are genuinely
// substitutable for one another, plus the one-way "contains" relation (includedIn).
// It is a leaf package so both the market-coverage verdict (internal/job/verdict) and the
// per-job deterministic match (internal/candidate/jobmatch) can share it without a
// low-level package depending up on a feature package.
package skilladjacency

// adjacentTo maps a role skill to the candidate skills genuinely substitutable for it: a
// conservative curated dictionary (only real swaps, so `adjacent` never over-triggers). A role
// skill the CV lacks but has a listed neighbour for reads as `adjacent` rather than `missing`.
// Keys and values are canonical skilltag slugs.
var adjacentTo = map[string][]string{
	// ML frameworks
	"pytorch":    {"tensorflow"},
	"tensorflow": {"pytorch"},
	// Relational databases
	"postgresql": {"mysql", "mariadb"},
	"mysql":      {"postgresql", "mariadb"},
	"mariadb":    {"postgresql", "mysql"},
	// Cloud providers
	"aws":   {"gcp", "azure"},
	"gcp":   {"aws", "azure"},
	"azure": {"aws", "gcp"},
	// Frontend frameworks
	"react":   {"vue", "angular"},
	"vue":     {"react", "angular"},
	"angular": {"react", "vue"},
	// Message queues
	"kafka":    {"rabbitmq", "sqs"},
	"rabbitmq": {"kafka", "sqs"},
	// Backend web frameworks
	"fastapi": {"flask", "django", "express", "nestjs"},
	"flask":   {"fastapi", "django"},
	"django":  {"flask", "fastapi"},
	"express": {"nestjs", "fastify"},
	"nestjs":  {"express", "fastify"},
}

// AdjacentVia returns the first neighbour of `required` present in `held` — the
// close skill that makes `required` count as adjacent rather than missing — or
// ("", false) when none. `held` is a canonical skill-slug set (a flat set of the
// caller's skills), keeping this usable outside the CV declared/body split.
func AdjacentVia(required string, held map[string]bool) (string, bool) {
	for _, adj := range adjacentTo[required] {
		if held[adj] {
			return adj, true
		}
	}
	return "", false
}

// includedIn maps a generic skill to the specific skills that each contain it: whoever holds
// one of them holds the generic one too (PostgreSQL is SQL). The relation is one-way — a
// generic skill never stands in for a specific one, so "sql" does not satisfy "sql-server".
var includedIn = map[string][]string{
	"sql": {"postgresql", "mysql", "mariadb", "sql-server", "oracle", "sqlite", "db2", "sybase", "teradata", "cockroachdb", "plsql"},
	// GitHub, GitLab and Bitbucket are git hosts: working on one is working with git.
	"git":             {"github", "gitlab", "bitbucket"},
	"version-control": {"git", "github", "gitlab", "bitbucket", "svn"},
}

// IncludedVia returns the first held skill that contains `required` (see includedIn), or
// ("", false) when none.
func IncludedVia(required string, held map[string]bool) (string, bool) {
	for _, s := range includedIn[required] {
		if held[s] {
			return s, true
		}
	}
	return "", false
}
