package userjob

// activeRank is the forward position of each ACTIVE stage. It is NOT the index in
// Stages: accepted, rejected and withdrawn follow offer there but are outcomes an
// application does not pass THROUGH, so they have no rank.
//
// A stage added to Stages without a rank would rank below applied and let a forward
// signal move an application backward; TestEveryStageIsRankedOrTerminal fails then.
// The keys match internal/job/silence's thresholds (a test binds them).
var activeRank = map[string]int{
	"preparing": 0,
	"applied":   1,
	"screening": 2,
	"responded": 3,
	"interview": 4,
	"offer":     5,
}

// terminalStages are the settled outcomes. An automatic advance must never move an
// application out of one (it would resurrect it) nor into one: accepted, rejected and
// withdrawn are the candidate's call, never inferred from mail.
// `expired` is settled too, but is likewise the candidate's own conclusion; nothing
// computes it from a threshold, so silence stays the only automatic reading of a
// quiet employer.
var terminalStages = map[string]bool{
	"accepted":  true,
	"rejected":  true,
	"withdrawn": true,
	"expired":   true,
}

// IsTerminal reports whether stage is a settled outcome.
func IsTerminal(stage string) bool { return terminalStages[stage] }

// Forward reports whether an application at `current` may be advanced to `target` automatically.
//
// Strictly forward, and only between active stages. An empty or unranked `current` ranks below
// every stage, so a first classified email can seed the stage. A target that is not an active
// stage — a terminal outcome, or anything unknown — is never an automatic advance.
func Forward(current, target string) bool {
	if IsTerminal(current) {
		return false
	}
	next, ok := activeRank[target]
	if !ok {
		return false
	}
	return next > activeRank[current]
}

// EmployerMay reports whether an employer's own pipeline move may set the candidate's stage
// from `current` to `target`. `currentByEmployer` is whether the employer set `current`
// themselves (the newest stage event's source).
//
// Unlike Forward, an employer's move is an explicit statement about the application, not an
// inference from mail, so it may settle it: a rejection or a hire is theirs to announce. It
// still never overrides the candidate. A stage the candidate set is moved only forward, and
// one they settled (withdrawn, or an outcome they recorded) is never reopened. A stage the
// employer set is theirs to correct in any direction — a rejection clicked by mistake has to
// be undoable, or the candidate keeps reading a decision nobody made.
func EmployerMay(current string, currentByEmployer bool, target string) bool {
	if current == target || !ValidStage(target) {
		return false
	}
	if currentByEmployer {
		return true
	}
	if IsTerminal(current) {
		return false
	}
	if IsTerminal(target) {
		return true
	}
	return Forward(current, target)
}
