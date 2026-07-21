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
