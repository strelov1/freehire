package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
)

// readyTimeout bounds the GET /ready that decides whether the retry pass runs at all.
const readyTimeout = 5 * time.Second

// facetdRetryStore is the retry queue's persistence.
type facetdRetryStore interface {
	Claim(ctx context.Context, batch, leaseSeconds int) ([]searchdrain.Claimed, error)
	Jobs(ctx context.Context, ids []int64) ([]db.Job, error)
	Complete(ctx context.Context, outboxIDs []int64) error
	Fail(ctx context.Context, outboxIDs []int64, errMsg string, maxAttempts int) error
	Stats(ctx context.Context) (depth, deadLetters int64, err error)
}

// facetdTarget is the facetd client as the retry pass uses it.
type facetdTarget interface {
	facetdWriter
	Ready(ctx context.Context) error
}

// facetdRetry re-sends what a failed facetd write left in facetd_outbox. It runs after the
// Meilisearch passes, only while facetd is ready, and sends each claimed batch as ONE write
// built from the jobs' current state, not from whatever the failed write carried.
type facetdRetry struct {
	store  facetdRetryStore
	facetd facetdTarget
	// build makes the documents of open jobs and reports the ones search withholds; it is
	// the indexing wave's own code (docBuilder.build).
	build  func(ctx context.Context, jobs []db.Job) ([]search.JobDocument, []int64, error)
	mirror *facetdMirror // for the failure tally
}

// facetdRetryResult is what a pass did, for the log and the metrics.
type facetdRetryResult struct {
	Sent   int
	Failed int
}

// classifyForFacetd splits the queued job ids by what facetd must be told about each, from the
// rows as they stand: gone or private -> delete; closed -> close (and duplicate when it is
// also a repost, as the deletion wave does); an open repost -> duplicate. The open canonical
// jobs are returned for the caller to turn into documents (some may turn out withheld).
func classifyForFacetd(ids []int64, jobs []db.Job) (open []db.Job, w search.FacetdWrite) {
	byID := make(map[int64]db.Job, len(jobs))
	for _, j := range jobs {
		byID[j.ID] = j
	}
	for _, id := range ids {
		j, ok := byID[id]
		switch {
		case !ok || j.IsPrivate:
			w.Delete = append(w.Delete, id)
		case j.ClosedAt.Valid:
			w.Close = append(w.Close, id)
			if j.DuplicateOf.Valid {
				w.Duplicate = append(w.Duplicate, id)
			}
		case j.DuplicateOf.Valid:
			w.Duplicate = append(w.Duplicate, id)
		default:
			open = append(open, j)
		}
	}
	return open, w
}

// Run drains the retry queue while facetd stays ready and healthy. Stop and MaxPerRun are
// the run's, like the Meilisearch passes. A failed batch ends the pass: facetd is unwell, and
// the failure released the lease, so continuing would only reclaim the same entries.
func (r facetdRetry) Run(ctx context.Context, opt searchdrain.RunOptions) facetdRetryResult {
	var res facetdRetryResult
	rctx, cancel := context.WithTimeout(ctx, readyTimeout)
	err := r.facetd.Ready(rctx)
	cancel()
	if err != nil {
		log.Printf("search-drain: facetd retry skipped, facetd not ready: %v", err)
		return res
	}
	claimed := 0
	for ctx.Err() == nil && (opt.Stop == nil || !opt.Stop()) {
		batch := opt.BatchSize
		if opt.MaxPerRun > 0 {
			batch = min(batch, opt.MaxPerRun-claimed)
		}
		if batch < 1 {
			break
		}
		entries, err := r.store.Claim(ctx, batch, opt.LeaseSeconds)
		if err != nil {
			log.Printf("search-drain: facetd retry claim: %v", err)
			break
		}
		if len(entries) == 0 {
			break
		}
		claimed += len(entries)
		sent, rejected, err := r.deliver(ctx, opt, entries)
		res.Sent += sent
		res.Failed += rejected
		if err != nil && ctx.Err() != nil {
			// The run ended (a stop, a SIGTERM), not facetd: spend no attempt, the lease
			// expires and the next run retries the entries.
			log.Printf("search-drain: facetd retry of %d jobs abandoned (%v), left claimed: %v", len(entries), ctx.Err(), err)
			break
		}
		if err != nil {
			res.Failed += len(entries) - sent - rejected
			r.mirror.failures.Add(1)
			log.Printf("search-drain: facetd retry of %d jobs failed: %v", len(entries), err)
			// Detached: a cancelled run must still release the lease and count the attempt.
			// Entries already completed are gone from the table, so failing them is a no-op.
			if ferr := r.store.Fail(context.WithoutCancel(ctx), outboxIDs(entries), err.Error(), opt.MaxAttempts); ferr != nil {
				log.Printf("search-drain: facetd retry record failure: %v", ferr)
			}
			break
		}
		log.Printf("search-drain: facetd retry sent %d jobs (%d rejected)", sent, rejected)
		if len(entries) < batch {
			break
		}
	}
	return res
}

// deliver sends the entries as one write. When facetd rejects the batch (an unreadable
// document) it splits it in halves until the bad entries stand alone, and dead-letters only
// those, so one broken job does not take its whole batch down. Any other error is returned:
// facetd is unwell and the pass stops.
func (r facetdRetry) deliver(ctx context.Context, opt searchdrain.RunOptions, entries []searchdrain.Claimed) (sent, rejected int, err error) {
	err = r.send(ctx, opt, entries)
	switch {
	case err == nil:
		return len(entries), 0, nil
	case !errors.Is(err, search.ErrFacetdRejected):
		return 0, 0, err
	case len(entries) == 1:
		log.Printf("search-drain: facetd rejected job %d, dead-lettering it: %v", entries[0].JobID, err)
		if ferr := r.store.Fail(context.WithoutCancel(ctx), outboxIDs(entries), err.Error(), 1); ferr != nil {
			log.Printf("search-drain: facetd retry record failure: %v", ferr)
		}
		return 0, 1, nil
	}
	mid := len(entries) / 2
	s1, r1, err := r.deliver(ctx, opt, entries[:mid])
	if err != nil {
		return s1, r1, err
	}
	s2, r2, err := r.deliver(ctx, opt, entries[mid:])
	return s1 + s2, r1 + r2, err
}

// send derives the entries' current state and delivers it as one facetd write.
func (r facetdRetry) send(ctx context.Context, opt searchdrain.RunOptions, entries []searchdrain.Claimed) error {
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.JobID
	}
	jobs, err := r.store.Jobs(ctx, ids)
	if err != nil {
		return fmt.Errorf("load jobs: %w", err)
	}
	open, w := classifyForFacetd(ids, jobs)
	if len(open) > 0 {
		docs, withheld, err := r.build(ctx, open)
		if err != nil {
			return fmt.Errorf("build documents: %w", err)
		}
		w.Upserts = docs
		w.Delete = append(w.Delete, withheld...)
	}
	wctx := ctx
	if opt.CallTimeout > 0 {
		var cancel context.CancelFunc
		wctx, cancel = context.WithTimeout(ctx, opt.CallTimeout)
		defer cancel()
	}
	if err := r.facetd.Write(wctx, w); err != nil {
		return err
	}
	return r.store.Complete(ctx, outboxIDs(entries))
}

func outboxIDs(entries []searchdrain.Claimed) []int64 {
	ids := make([]int64, len(entries))
	for i, e := range entries {
		ids[i] = e.OutboxID
	}
	return ids
}

// dbFacetdRetryStore is facetdRetryStore over the generated queries.
type dbFacetdRetryStore struct{ q *db.Queries }

func newDBFacetdRetryStore(pool *pgxpool.Pool) dbFacetdRetryStore {
	return dbFacetdRetryStore{q: db.New(pool)}
}

func (s dbFacetdRetryStore) Claim(ctx context.Context, batch, leaseSeconds int) ([]searchdrain.Claimed, error) {
	rows, err := s.q.ClaimFacetdOutboxBatch(ctx, db.ClaimFacetdOutboxBatchParams{
		LeaseSeconds: clampInt32(leaseSeconds),
		BatchSize:    clampInt32(batch),
	})
	if err != nil {
		return nil, err
	}
	out := make([]searchdrain.Claimed, len(rows))
	for i, r := range rows {
		out[i] = searchdrain.Claimed{OutboxID: r.ID, JobID: r.JobID}
	}
	return out, nil
}

func (s dbFacetdRetryStore) Jobs(ctx context.Context, ids []int64) ([]db.Job, error) {
	return s.q.GetJobsByIDs(ctx, ids)
}

func (s dbFacetdRetryStore) Complete(ctx context.Context, ids []int64) error {
	return s.q.CompleteFacetdOutbox(ctx, ids)
}

func (s dbFacetdRetryStore) Fail(ctx context.Context, ids []int64, errMsg string, maxAttempts int) error {
	_, err := s.q.RecordFacetdOutboxFailure(ctx, db.RecordFacetdOutboxFailureParams{
		LastError:   errMsg,
		MaxAttempts: clampInt32(maxAttempts),
		Ids:         ids,
	})
	return err
}

// clampInt32 narrows an option read from the environment to the int4 the query takes.
func clampInt32(v int) int32 {
	if v > math.MaxInt32 {
		return math.MaxInt32
	}
	if v < math.MinInt32 {
		return math.MinInt32
	}
	return int32(v)
}

func (s dbFacetdRetryStore) Stats(ctx context.Context) (int64, int64, error) {
	row, err := s.q.FacetdOutboxStats(ctx)
	return row.Depth, row.DeadLetters, err
}
