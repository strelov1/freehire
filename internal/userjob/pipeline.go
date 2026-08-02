package userjob

// activeRank is the forward position of each ACTIVE stage — how far along the pipeline an
// application has got. It is deliberately NOT the index in Stages: accepted, rejected and
// withdrawn sit after offer in the vocabulary but are outcomes, not positions. An application
// does not pass THROUGH them, so they have no rank.
//
// This lives here rather than in the package that reads mail because "how an application may
// move through its stages" is a tracking rule. It was in internal/mailclassify, which welded
// the mail vocabulary to the application state machine and let a real drift through: a stage
// inserted into Stages got rank 0, ranking BELOW applied and absent from the terminal set, so
// any forward signal advanced the application backward out of it. TestEveryStageIsRankedOrTerminal
// is what now fails instead.
//
// The keys are the same five that silenceThresholds keys on, which is not a coincidence: both
// answer "is this application still in flight", and a test binds them.
var activeRank = map[string]int{
	"applied":   1,
	"screening": 2,
	"responded": 3,
	"interview": 4,
	"offer":     5,
}

// terminalStages are the settled outcomes. An automatic advance must never move an application
// out of one — that would resurrect a dead application — and never into one either: deciding an
// application is rejected, accepted or withdrawn is the candidate's call or an explicit action,
// never an inference from a piece of mail.
// `expired` is settled without anyone deciding: the employer never answered, or the posting
// went away. It is the candidate's own conclusion, never an inference — nothing computes it
// from a threshold, so the silence state stays the only automatic reading of a quiet employer.
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
