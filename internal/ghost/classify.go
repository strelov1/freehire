// Package ghost derives a hedged "is this posting real" verdict for a job from
// evidence of two different natures, and states which criteria produced it.
//
// The tiers are the point of the package, not an implementation detail.
// STRUCTURAL criteria describe the SHAPE of a posting — how long it has been up,
// whether the employer's own board carries it. OUTCOME criteria describe what
// happened to somebody who applied. Only the second kind can witness that a
// posting is not being worked, which is why no quantity of the first kind may
// produce the stronger claim (see Classify).
//
// Like internal/jobreality it is a pure classifier over scalars — no database, no
// clock of its own — and like it the verdict is TIME-DEPENDENT and therefore
// computed on read, never stored.
package ghost

import (
	"time"

	"github.com/strelov1/freehire/internal/jobreality"
)

// Ghost levels. The vocabulary is deliberately hedged: the system observes facts
// about a posting, never an employer's intent, so the strongest thing it may say
// is that a posting is likely inactive.
const (
	// LevelNone is a posting with too little evidence to say anything.
	LevelNone = "none"
	// LevelPossible is convergent evidence that has not been witnessed by people.
	LevelPossible = "possible"
	// LevelLikely is convergent evidence corroborated by applicants.
	LevelLikely = "likely"
)

// Criterion codes, in the order Classify reports them: structural first, then
// outcome. The order is fixed so a served payload is stable between reads.
const (
	CriterionEvergreenPosting   = "evergreen_posting"
	CriterionATSAbsent          = "ats_absent"
	CriterionSilentApplications = "silent_applications"
	CriterionUserReports        = "user_reports"
)

// CriterionCodes is the vocabulary as an ordered value, in the same fixed order.
// It exists because the interface has to RENDER the criteria, not merely receive
// them: cmd/gen-contracts emits it into the web contracts, and the SPA builds its
// checklist rows from a map keyed by the generated union. A criterion added here
// with no row over there is then a type error rather than a gauge segment coloured
// above a list that cannot account for it.
//
// The consts above stay the way Go code refers to a criterion — a bare index into
// this array at a call site would say nothing about which criterion it meant.
var CriterionCodes = [...]string{
	CriterionEvergreenPosting,
	CriterionATSAbsent,
	CriterionSilentApplications,
	CriterionUserReports,
}

// CriteriaTotal is the denominator of the scale the interface shows ("2 of 4").
// It counts every criterion the classifier considers, including the ones that
// had no data — a reader must be able to see how much is unknown.
//
// Derived rather than written down: the denominator is a fact about the vocabulary,
// and a hand-kept 4 beside a list of five is a scale that lies to every reader.
const CriteriaTotal = len(CriterionCodes)

// ContributorGate is how many DISTINCT people must have contributed outcome
// evidence before the stronger claim is available. It is exported because the
// serving layer withholds the counts below the same threshold — one number, two
// consequences, and letting them drift apart would serve a count that identifies
// a single applicant to the employer.
//
// Two independent constraints land on the same number, and both must hold. A
// count of one identifies the single applicant to the employer, so a served
// count would deanonymise them. And one account must not be able to mark an
// honest posting on its own. Lowering this breaks a privacy guarantee and an
// abuse guarantee at the same time.
const ContributorGate = 2

// absenceStampMaxAgeDays is how old a cross-check stamp may be and still count.
// The worker re-stamps on every run, so a stamp older than this means the worker
// has stopped — and a stopped worker must fall silent rather than keep accusing
// the catalogue from a frozen snapshot. The value is the LAST TOLERATED day, not
// the first offending one, matching internal/userjob's silence ladder.
const absenceStampMaxAgeDays = 14

// convergence is how many criteria must fire before any level is assigned. One
// signal is weak on its own: a genuinely hard-to-fill senior role stays open a
// long time, and a company can be absent from our board coverage for reasons of
// its own.
const convergence = 2

// Input is the evidence a classification reads. Contributors is the number of
// DISTINCT people behind the outcome criteria, counted across both channels
// together — a person who both applied here and filed a report is one witness,
// not two. SilentApplications and Reports are row counts, used only to decide
// whether their criterion fires.
type Input struct {
	Now                time.Time
	RealityClass       string
	ATSAbsentAt        time.Time
	HasATSAbsent       bool
	Contributors       int
	SilentApplications int
	Reports            int
}

// Result is a level and the criteria that fired, in the fixed order above. The
// criteria travel with the level so the interface can state facts rather than a
// bare accusation — and so a reader can see WHY a level is not higher.
type Result struct {
	Level    string
	Criteria []string
}

// Classify assigns a ghost level from the evidence.
//
// Two gates, and they are not the same gate. `convergence` asks whether enough
// independent criteria fired for the system to say anything at all.
// `ContributorGate` asks whether real people witnessed it, and is the only route
// to LevelLikely — so structural evidence, however much of it converges, can
// never produce the stronger claim.
//
// Outcome evidence from enough people reaches LevelPossible even as a lone
// criterion: two strangers independently reporting that nobody answered is a
// stronger fact than any two artifacts of posting shape, and requiring it to be
// corroborated by structure would make the outcome tier — the only one that
// observes reality — unable to mark anything by itself.
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
