// Package ghost derives a hedged "is this posting real" verdict for a job from two kinds
// of evidence, and states which criteria produced it.
//
// STRUCTURAL criteria describe the shape of a posting (how long it has been up, whether
// the employer's own board carries it). OUTCOME criteria describe what happened to
// somebody who applied. Only outcomes can witness that a posting is not being worked, so
// no quantity of structural evidence may produce the stronger claim (see Classify).
//
// Like internal/job/jobreality it is a pure classifier over scalars, and the verdict is
// time-dependent, so it is computed on read, never stored.
package ghost

import (
	"time"

	"github.com/strelov1/freehire/internal/job/jobreality"
)

// Ghost levels. The vocabulary is hedged: the system observes a posting, never an
// employer's intent, so the strongest claim is that it is likely inactive.
const (
	// LevelNone is a posting with too little evidence to say anything.
	LevelNone = "none"
	// LevelPossible is convergent evidence that has not been witnessed by people.
	LevelPossible = "possible"
	// LevelLikely is convergent evidence corroborated by applicants.
	LevelLikely = "likely"
)

// Criterion codes, in the order Classify reports them: structural first, then outcome.
const (
	CriterionEvergreenPosting   = "evergreen_posting"
	CriterionATSAbsent          = "ats_absent"
	CriterionSilentApplications = "silent_applications"
	CriterionUserReports        = "user_reports"
)

// CriterionCodes is the vocabulary as an ordered value. cmd/gen-contracts emits it into
// the web contracts, so a criterion added here with no SPA checklist row is a type error.
// Go code refers to criteria by the consts above, not by index.
var CriterionCodes = [...]string{
	CriterionEvergreenPosting,
	CriterionATSAbsent,
	CriterionSilentApplications,
	CriterionUserReports,
}

// CriteriaTotal is the denominator of the scale the interface shows ("2 of 4"). It counts
// every criterion, including those with no data, so a reader sees how much is unknown.
const CriteriaTotal = len(CriterionCodes)

// ContributorGate is how many DISTINCT people must have contributed outcome evidence
// before the stronger claim is available. It is exported because the serving layer
// withholds counts below the same threshold.
//
// Two constraints share the number: a count of one would deanonymise the single applicant
// to the employer, and one account must not be able to mark an honest posting alone.
// Lowering it breaks both.
const ContributorGate = 2

// absenceStampMaxAgeDays is how old a cross-check stamp may be and still count. The worker
// re-stamps every run, so an older stamp means it stopped, and it must then fall silent
// rather than accuse from a frozen snapshot. It is the last tolerated day, matching
// internal/job/silence's ladder.
const absenceStampMaxAgeDays = 14

// convergence is how many criteria must fire before any level is assigned. One signal is
// weak: a hard-to-fill role stays open long, and a company can be missing from our boards.
const convergence = 2

// Input is the evidence a classification reads. Contributors counts DISTINCT people behind
// the outcome criteria across both channels (applying and reporting is one witness).
// SilentApplications and Reports are row counts, used only to decide whether they fire.
type Input struct {
	Now                time.Time
	RealityClass       string
	ATSAbsentAt        time.Time
	HasATSAbsent       bool
	Contributors       int
	SilentApplications int
	Reports            int
}

// Result is a level and the criteria that fired, in the fixed order above, so the
// interface can state facts and show why a level is not higher.
type Result struct {
	Level    string
	Criteria []string
}

// Classify assigns a ghost level from the evidence.
//
// Two different gates: `convergence` asks whether enough criteria fired to say anything,
// `ContributorGate` asks whether real people witnessed it and is the only route to
// LevelLikely, so structural evidence alone never produces the stronger claim.
//
// Outcome evidence from enough people reaches LevelPossible even as a lone criterion:
// requiring structural corroboration would leave the only tier that observes reality
// unable to mark anything by itself.
func Classify(in Input) Result {
	criteria := make([]string, 0, CriteriaTotal)
	if in.RealityClass == jobreality.ClassLikelyEvergreen {
		criteria = append(criteria, CriterionEvergreenPosting)
	}
	if in.HasATSAbsent && !expired(in.Now, in.ATSAbsentAt) {
		criteria = append(criteria, CriterionATSAbsent)
	}
	if in.SilentApplications > 0 {
		criteria = append(criteria, CriterionSilentApplications)
	}
	if in.Reports > 0 {
		criteria = append(criteria, CriterionUserReports)
	}

	converged := len(criteria) >= convergence
	witnessed := in.Contributors >= ContributorGate

	level := LevelNone
	switch {
	case converged && witnessed:
		level = LevelLikely
	case converged || witnessed:
		level = LevelPossible
	}

	return Result{Level: level, Criteria: criteria}
}

// expired reports whether a cross-check stamp has aged past what still counts.
// Whole days elapsed, so a part-day never tips the boundary.
func expired(now, stampedAt time.Time) bool {
	return int(now.Sub(stampedAt).Hours()/24) > absenceStampMaxAgeDays
}
