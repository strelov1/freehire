package verdict

// adjacentTo maps a role skill to the candidate skills that are genuinely
// substitutable/transferable for it — a curated, conservative dictionary (only real
// swaps, so `adjacent` never over-triggers). A role skill the CV lacks but for which
// it holds a listed neighbour reads as `adjacent` (close, reframe-able) rather than a
// hard `missing`. Keys and values are canonical skilltag slugs.
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

// adjacentHeld returns the first neighbour of `roleSkill` that the CV holds (in
// declared or body), or "" when none — i.e. the close skill to reframe around.
func adjacentHeld(roleSkill string, declared, body map[string]bool) string {
	for _, adj := range adjacentTo[roleSkill] {
		if declared[adj] || body[adj] {
			return adj
		}
	}
	return ""
}
