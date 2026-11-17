package job

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/jackc/pgx/v5"

	"github.com/strelov1/freehire/internal/platform/db"
)

// Upsert is the one full write of a posting: the generated UpsertJob, then the observation of the
// posting's job_activity row, in the caller's transaction. Two commands because the insert trigger
// creates the activity row at the end of the INSERT, where no CTE of that statement can see it.
// Stopping after UpsertJob leaves hydration NULL and last_seen_at frozen, so a test allows only
// this file to call it.
//
// The transaction is a pgx.Tx by type: both commands must commit together, and the jobs row is
// locked before the activity row (the one global lock order).
func Upsert(ctx context.Context, tx pgx.Tx, p db.UpsertJobParams) (db.UpsertJobRow, error) {
	q := db.New(tx)
	p, err := ResolveUpsertIdentity(ctx, tx, p)
	if err != nil {
		return db.UpsertJobRow{}, err
	}
	row, err := q.UpsertJob(ctx, p)
	if err != nil {
		return db.UpsertJobRow{}, err
	}
	// A new row always read its body; an existing one did too unless the pipeline substituted the
	// stored body for a failed fetch (nothing was read, so the hydration stamp stays).
	// ResetStrikes is the old row's state under the write's own lock (it was closed).
	if err := Observe(ctx, tx, row.ID, row.Inserted.Bool || !p.BodyFromCatalogue, row.ResetStrikes); err != nil {
		return db.UpsertJobRow{}, err
	}
	return row, nil
}

// Observe records that a crawl saw the posting now: last_seen_at moves forward, hydration moves
// only when the body was read, strikes clear only when asked. Every job has exactly one activity
// row, so any other count is a broken invariant, not a no-op.
func Observe(ctx context.Context, tx pgx.Tx, jobID int64, bodyRead, resetStrikes bool) error {
	n, err := db.New(tx).ObserveJobActivity(ctx, db.ObserveJobActivityParams{
		JobID:        jobID,
		BodyRead:     bodyRead,
		ResetStrikes: resetStrikes,
	})
	if err != nil {
		return fmt.Errorf("observe job %d: %w", jobID, err)
	}
	if n != 1 {
		return fmt.Errorf("observe job %d: %d activity rows, want exactly 1", jobID, n)
	}
	return nil
}

// ResolveUpsertIdentity preserves pre-existing LinkedIn keyword-scoped rows while
// new writes use the globally unique posting ID. Other sources keep their identity.
// The URL lookup includes closed public history, leaving reopening and expiry to
// the ordinary upsert. A caller with a cheap refresh resolves identity before that
// refresh too, so a legacy row does not force a full rewrite on every crawl.
// The transaction holds the posting's identity lock through lookup and commit.
func ResolveUpsertIdentity(ctx context.Context, tx pgx.Tx, p db.UpsertJobParams) (db.UpsertJobParams, error) {
	if p.Source != "linkedin" {
		return p, nil
	}
	u, err := url.Parse(p.URL)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" ||
		(u.Hostname() != "linkedin.com" && !strings.HasSuffix(u.Hostname(), ".linkedin.com")) {
		return p, fmt.Errorf("linkedin: invalid native posting URL")
	}
	id, hasPath := strings.CutPrefix(strings.TrimSuffix(u.Path, "/"), "/jobs/view/")
	if !hasPath || id == "" || strings.IndexFunc(id, func(c rune) bool { return c < '0' || c > '9' }) >= 0 {
		return p, fmt.Errorf("linkedin: invalid native posting ID")
	}
	key := p.ExternalID[strings.LastIndex(p.ExternalID, ":")+1:]
	if key != id {
		return p, fmt.Errorf("linkedin: posting ID does not match its URL")
	}
	p.URL, p.ExternalID = "https://www.linkedin.com/jobs/view/"+id+"/", id
	q := db.New(tx)
	if err := q.LockLinkedInPostingIdentity(ctx, id); err != nil {
		return p, err
	}
	existing, err := q.FindLinkedInJobByURL(ctx, p.URL)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	if err == nil {
		p.ExternalID = existing.ExternalID
	}
	// Public URL matching never selects a private row. Also refuse an exact-key
	// collision, instead of letting ON CONFLICT rewrite another caller's private job.
	exact, err := q.GetJobBySourceExternalID(ctx, db.GetJobBySourceExternalIDParams{Source: p.Source, ExternalID: p.ExternalID})
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return p, err
	}
	if err == nil && exact.IsPrivate {
		return p, fmt.Errorf("linkedin: native posting identity belongs to a private job")
	}
	return p, nil
}
