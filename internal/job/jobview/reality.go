package jobview

import (
	"time"

	"github.com/strelov1/freehire/internal/job/jobreality"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Reality is the served job-reality signal: the class plus the observable facts behind
// it, so the UI states facts ("open 240 days · reposted 6×") rather than a bare
// accusation. Computed at index/read time, never stored; omitted when not computed.
type Reality struct {
	Class            string `json:"class"`
	AgeDays          int    `json:"age_days"`
	RepostCount      int    `json:"repost_count"`
	MassPostingCount int    `json:"mass_posting_count"`
	IsFakeFreshness  bool   `json:"fake_freshness"`
}

// ClassifyReality derives a job's reality signal from its row, the current time, and
// the repost/mass-posting counts of its role cluster (internal/platform/db
// RoleClusterCount). The evergreen-text signal comes from the jobreality dictionary.
//
// It stays row-based, unlike the rest of the package: it reads three raw scalars on an
// index/read hot path that already holds the db.Job, and hydrating the aggregate would
// decode the enrichment JSONB a second time per job.
//
// jobs.created_at is NOT NULL, so CreatedAt.Time is always meaningful; PostedAt.Time is
// honored only under HasPostedAt.
func ClassifyReality(j db.Job, now time.Time, repostCount, massPostingCount int) Reality {
	res := jobreality.Classify(jobreality.Input{
		Now:              now,
		CreatedAt:        j.CreatedAt.Time,
		PostedAt:         j.PostedAt.Time,
		HasPostedAt:      j.PostedAt.Valid,
		RepostCount:      repostCount,
		MassPostingCount: massPostingCount,
		HasEvergreenText: jobreality.HasEvergreenMarker(j.Description),
	})
	return Reality{
		Class:            res.Class,
		AgeDays:          res.Evidence.AgeDays,
		RepostCount:      res.Evidence.RepostCount,
		MassPostingCount: res.Evidence.MassPostingCount,
		IsFakeFreshness:  res.Evidence.IsFakeFreshness,
	}
}
