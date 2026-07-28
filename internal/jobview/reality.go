package jobview

import (
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/jobreality"
)

// Reality is the served job-reality signal: the class plus the observable facts that
// produced it, so the UI states facts ("open 240 days · reposted 6×") rather than a
// bare accusation. It is computed at index/read time (never stored — it is
// time-dependent) and attached via ClassifyReality; a job with no computed signal
// omits the field.
type Reality struct {
	Class            string `json:"class"`
	AgeDays          int    `json:"age_days"`
	RepostCount      int    `json:"repost_count"`
	MassPostingCount int    `json:"mass_posting_count"`
	IsFakeFreshness  bool   `json:"fake_freshness"`
}

// ClassifyReality derives a job's reality signal from its row, the current time, and
// the repost/mass-posting counts of its role cluster (see internal/db RoleClusterCount).
// The evergreen-text signal is read from the description via the jobreality dictionary.
//
// It deliberately stays row-based while the rest of the package takes the domain
// (FromDomain): the classification reads only three raw scalars (created_at, posted_at
// validity, description), and every caller is an index/read hot path that already holds
// the db.Job. Hydrating the aggregate just for those would decode the enrichment JSONB
// a second time per job (the sibling FromRow/search.FromJob call already does it once)
// and add a decode-failure mode to a step that has nothing to do with enrichment.
//
// The pgtype reads are safe: jobs.created_at is NOT NULL DEFAULT now() (migrations), so
// CreatedAt.Time is always meaningful, and PostedAt.Time is honored only under
// HasPostedAt (jobreality.Classify ignores it otherwise).
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
