// Package applicationdigest delivers an opt-in weekly summary of application activity.
package applicationdigest

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/engage/digest"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

const BatchSize = digest.BatchSize

type Candidate = digest.Candidate

type Followup struct {
	ID                                     int64
	RoleTitle, CompanySlug, JobSlug, Stage string
	DaysSilent                             int
	FollowedUpAt                           time.Time
}
type Snapshot struct {
	ActivityCount                                          int64
	AsOf                                                   time.Time
	AppliedCount, RepliesCount, InterviewCount, OfferCount int64
	StageCounts                                            map[string]int64
	Followups                                              []Followup
}

func (s Snapshot) Empty() bool {
	return s.ActivityCount+s.AppliedCount+s.RepliesCount+s.InterviewCount+s.OfferCount == 0 && len(s.Followups) == 0
}

type Store interface {
	Candidates(context.Context, time.Time, int64, int32) ([]Candidate, error)
	Snapshot(context.Context, Candidate, time.Time) (Snapshot, error)
	Claim(context.Context, Candidate, Snapshot, time.Time) (bool, error)
	MarkSent(context.Context, Candidate, time.Time) error
	ReleasePaused(context.Context, Candidate, time.Time) error
}

// paged adapts the keyset-paged store to the runner: a full page is followed by another.
type paged struct{ Store }

func (s paged) Candidates(ctx context.Context, at time.Time, after int64) ([]Candidate, bool, error) {
	rows, err := s.Store.Candidates(ctx, at, after, BatchSize)
	return rows, len(rows) == int(BatchSize), err
}

type Stats = digest.Stats

type Runner struct {
	store  Store
	mailer *Mailer
	sender emailnotify.Sender
}

func New(store Store, mailer *Mailer, sender emailnotify.Sender) *Runner {
	return &Runner{store: store, mailer: mailer, sender: sender}
}

func (r *Runner) Run(ctx context.Context, now time.Time) (Stats, error) {
	return digest.Run(ctx, now, "application-digest", paged{r.store}, r.sender, r.mailer.Render)
}
