package verdict

// Input carries everything Compute needs, all derived from real data: the role's
// open-vacancy total, the uncovered set (total + per-skill distribution), the full
// role skill distribution (skill slug → vacancies listing it), and the CV's parsed
// declared/body skill sets.
//
// This is a handler-side argument struct, not part of the wire contract — it lives
// in its own file so cmd/gen-contracts (which reads verdict.go only) does not emit
// it to the frontend TypeScript.
type Input struct {
	Total           int64
	UncoveredTotal  int64
	UncoveredSkills map[string]int64
	RoleSkills      map[string]int64
	Declared        []string
	Body            []string
}
