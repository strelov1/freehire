package jobview

import (
	"time"

	"github.com/strelov1/freehire/internal/ghost"
)

// GhostInput is what a ghost classification reads. It takes scalars rather than a
// db.Job because the signal is served from two different shapes: the Postgres read
// paths, which hold the row, and the search path, whose hits come back from
// Meilisearch. One function over scalars keeps a single rule; a row-shaped one would
// have forced the search path to reconstruct a row it never had.
type GhostInput struct {
	Now          time.Time
	Closed       bool
	RealityClass string
	ATSAbsentAt  time.Time
	HasATSAbsent bool
	Evidence     ghost.Evidence
}

// ClassifyGhost derives a job's ghost signal. It returns nil when there is nothing
// to say — level `none` — so the field is omitted rather than served empty.
//
// A CLOSED job carries no signal at all, whatever its evidence: the posting is
// already down, so there is nobody left to warn and no decision left to inform.
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
	// Served only when the criterion actually fired, so the checklist can say "checked
	// N days ago" beside a criterion that is standing on that check — and says nothing
	// where the cross-check never ran, rather than implying a check that did not happen.
	for _, c := range res.Criteria {
		if c == ghost.CriterionATSAbsent {
			checked := in.ATSAbsentAt
			out.ATSCheckedAt = rfc3339Ptr(&checked)
			break
		}
	}
	return out
}
