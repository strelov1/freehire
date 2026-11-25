// Package inboxdigest delivers an opt-in daily summary of classified hiring mail.
package inboxdigest

import (
	"context"
	"time"

	"github.com/strelov1/freehire/internal/engage/digest"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

const BatchSize = digest.BatchSize

type Candidate = digest.Candidate

type Item struct {
	ID         int64     `json:"id"`
	Subject    string    `json:"subject"`
	Sender     string    `json:"sender"`
	Snippet    string    `json:"snippet"`
	Signal     string    `json:"signal"`
	ReceivedAt time.Time `json:"received_at"`
	Replied    bool      `json:"replied"`
}

type SchedulingActivity struct {
	ID      int64  `json:"id"`
	Subject string `json:"subject"`
	Kind    string `json:"kind"`
}

type Snapshot struct {
	IDs         []int64
	Total       int64
	Items       []Item
	ActivityIDs []int64
	Activity    []SchedulingActivity
}

// Empty reports whether there is nothing to show: no conversation and no scheduling activity.
func (s Snapshot) Empty() bool {
	return len(s.Items) == 0 && len(s.Activity) == 0
}

type Store interface {
	Candidates(context.Context, time.Time, int32) ([]Candidate, error)
	Snapshot(context.Context, Candidate, time.Time) (Snapshot, error)
	Claim(context.Context, Candidate, Snapshot, time.Time) (bool, error)
	MarkSent(context.Context, Candidate, time.Time) error
	ReleasePaused(context.Context, Candidate, time.Time) error
}

// singleBatch takes one page of recipients per run.
type singleBatch struct{ Store }

func (s singleBatch) Candidates(ctx context.Context, at time.Time, _ int64) ([]Candidate, bool, error) {
	rows, err := s.Store.Candidates(ctx, at, BatchSize)
	return rows, false, err
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
	return digest.Run(ctx, now, "inbox-digest", singleBatch{r.store}, r.sender, r.mailer.Render)
}
