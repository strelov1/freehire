package ghost

import (
	"time"

	"github.com/strelov1/freehire/internal/job/silence"
)

// Application is one candidate application whose owner has a connected mailbox; the
// database applies that gate (a join). LastActivityAt is the apply date or the newest
// linked message, whichever is later, as the tracking board derives it.
type Application struct {
	JobID                int64
	UserID               int64
	Stage                string
	LastActivityAt       time.Time
	HasPendingSuggestion bool
}

// Report is one non-retracted ghost report. AppliedOn is the stated apply date; the claim
// is not evidence until it has aged past the threshold below.
type Report struct {
	JobID     int64
	UserID    int64
	AppliedOn time.Time
}

// Evidence is the per-job outcome tally Classify reads. Contributors counts DISTINCT people
// across both channels, so it is not the sum of the other fields: applying and reporting
// is one witness.
type Evidence struct {
	Contributors       int
	SilentApplications int
	Reports            int
}

// Aggregate tallies outcome evidence per job. The returned map holds only jobs with at
// least one unit of evidence, so a catalogue-wide lookup scales with the evidence.
//
// Both channels use internal/job/silence's ladder, so this cannot disagree with the
// personal tracking board about the same application.
func Aggregate(now time.Time, apps []Application, reports []Report) map[int64]Evidence {
	evidence := make(map[int64]Evidence)
	contributors := make(map[int64]map[int64]struct{})

	record := func(jobID, userID int64, count func(*Evidence)) {
		ev := evidence[jobID]
		count(&ev)
		evidence[jobID] = ev

		if contributors[jobID] == nil {
			contributors[jobID] = make(map[int64]struct{})
		}
		contributors[jobID][userID] = struct{}{}
	}

	for _, app := range apps {
		// StateFor is the whole judgement (terminal stages, unconfirmed-mail softening,
		// per-stage thresholds). Only an outright `silent` is a fact.
		state := silence.StateFor(app.Stage, silence.Days(now, app.LastActivityAt), app.HasPendingSuggestion)
		if state != silence.Silent {
			continue
		}
		record(app.JobID, app.UserID, func(ev *Evidence) { ev.SilentApplications++ })
	}

	for _, report := range reports {
		threshold, ok := appliedThresholdDays()
		if !ok || silence.Days(now, report.AppliedOn) <= threshold {
			continue
		}
		record(report.JobID, report.UserID, func(ev *Evidence) { ev.Reports++ })
	}

	for jobID, people := range contributors {
		ev := evidence[jobID]
		ev.Contributors = len(people)
		evidence[jobID] = ev
	}
	return evidence
}

// appliedThresholdDays is the span a stated claim must clear before it counts. A report
// that nobody answered makes the same assertion as a tracked application, so it clears
// the same silence-ladder bar.
func appliedThresholdDays() (int, bool) {
	return silence.ThresholdDays("applied")
}
