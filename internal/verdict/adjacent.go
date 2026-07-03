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
