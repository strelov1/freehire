package jobview

import (
	"time"

	"github.com/strelov1/freehire/internal/job/ghost"
)

// GhostInput is what a ghost classification reads. It takes scalars rather than a
// db.Job because the signal is served from both Postgres rows and Meilisearch hits.
type GhostInput struct {
	Now          time.Time
	Closed       bool
	RealityClass string
	ATSAbsentAt  time.Time
	HasATSAbsent bool
	Evidence     ghost.Evidence
}

// ClassifyGhost derives a job's ghost signal. It returns nil when there is nothing
// to say (level `none`) so the field is omitted.
//
// A CLOSED job carries no signal at all, whatever its evidence.
func ClassifyGhost(in GhostInput) *Ghost {
	if in.Closed {
		return nil
	}
	ev := in.Evidence

	res := ghost.Classify(ghost.Input{
		Now:                in.Now,
		RealityClass:       in.RealityClass,
		ATSAbsentAt:        in.ATSAbsentAt,
		HasATSAbsent:       in.HasATSAbsent,
		Contributors:       ev.Contributors,
		SilentApplications: ev.SilentApplications,
		Reports:            ev.Reports,
	})
	if res.Level == ghost.LevelNone {
		return nil
	}

	out := &Ghost{
		Level:         res.Level,
		Criteria:      res.Criteria,
		CriteriaTotal: ghost.CriteriaTotal,
	}
	if ev.Contributors >= ghost.ContributorGate {
		contributors := ev.Contributors
		out.Contributors = &contributors
	}
	// Served only when the criterion fired, so the checklist never implies a check
	// that did not happen.
	for _, c := range res.Criteria {
		if c == ghost.CriterionATSAbsent {
			checked := in.ATSAbsentAt
			out.ATSCheckedAt = rfc3339Ptr(&checked)
			break
		}
	}
	return out
}
