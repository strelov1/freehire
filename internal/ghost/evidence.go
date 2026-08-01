package ghost

import (
	"time"

	"github.com/strelov1/freehire/internal/userjob"
)

// Application is one candidate application: an application whose owner has a
// connected mailbox, selected by the database. The mailbox gate is the database's
// job because it is a join; everything below is a rule, and rules belong where
// their single source is.
//
// LastActivityAt is the application's apply date or the newest message linked to
// it, whichever is later — the same derivation the tracking board reads, not a
// second definition of it.
type Application struct {
	JobID                int64
	UserID               int64
	Stage                string
	LastActivityAt       time.Time
	HasPendingSuggestion bool
}

// Report is one non-retracted ghost report. AppliedOn is the date the reporter
// states they applied; the claim is not evidence until that date has aged past
// the threshold below.
type Report struct {
	JobID     int64
	UserID    int64
	AppliedOn time.Time
}

// Evidence is the per-job outcome tally Classify reads. Contributors counts
// DISTINCT people across both channels, which is why it cannot be derived from
// the other two fields: one person filing a report about a job they also applied
// to here is one witness, and the sum would say two.
type Evidence struct {
	Contributors       int
	SilentApplications int
	Reports            int
}

// Aggregate tallies outcome evidence per job. The returned map holds only jobs
// with at least one unit of evidence — sparse by construction rather than by the
// caller filtering, so a catalogue-wide lookup stays proportional to the evidence
// rather than to the catalogue.
//
// Both channels are judged by internal/userjob's silence ladder rather than by
// numbers of their own. Restating the ladder here would let a change to it
// disagree silently with the personal tracking board — the same application
// judged twice, by two ladders, with nothing binding them.
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
		// SilenceStateFor is the whole judgement: it returns no state for a
		// terminal stage, reads an unset stage as `applied`, softens a silence
		// into a question when unconfirmed mail contradicts it, and applies the
		// stage's own threshold. Only an outright `silent` is a fact.
		state := userjob.SilenceStateFor(app.Stage, userjob.DaysSilent(now, app.LastActivityAt), app.HasPendingSuggestion)
		if state != userjob.SilenceSilent {
			continue
		}
		record(app.JobID, app.UserID, func(ev *Evidence) { ev.SilentApplications++ })
	}

	for _, report := range reports {
		threshold, ok := appliedThresholdDays()
		if !ok || userjob.DaysSilent(now, report.AppliedOn) <= threshold {
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

// appliedThresholdDays is the span a stated claim must clear before it counts,
// read from the silence ladder rather than restated. A person reporting that
// nobody answered is making the same assertion the board makes about a tracked
// application, so it must clear the same bar — measured at 21 days over 92
// observed applications.
func appliedThresholdDays() (int, bool) {
	return userjob.SilenceThresholdDays("applied")
}
