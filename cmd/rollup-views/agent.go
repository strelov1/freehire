package main

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/agentview"
	"github.com/strelov1/freehire/internal/platform/db"
)

// agentLogRetention is how long a day's agent-view file is kept after the run that
// applied it — the retention nginx's own logs have.
const agentLogRetention = 14 * 24 * time.Hour

// processAgent applies every finished, not-yet-applied day of the agent-view log and
// removes the applied files past retention. The cursor is the same content-signature
// table the nginx pass uses, claimed inside each file's transaction, so a re-run applies
// nothing twice.
func processAgent(ctx context.Context, pool *pgxpool.Pool, dir string, now time.Time) (nFiles, nViews int, err error) {
	files, err := agentview.ListFiles(dir, now)
	if err != nil {
		return 0, 0, err
	}
	q := db.New(pool)
	// A file that fails is reported and kept, and the files after it still apply: the days
	// are independent, so one unreadable day must not hold every later one back.
	var errs []error
	for _, f := range files {
		applied, err := applyAgentDay(ctx, pool, q, f.Path)
		switch {
		case errors.Is(err, errAlreadyApplied):
		case err != nil:
			errs = append(errs, fmt.Errorf("%s: %w", filepath.Base(f.Path), err))
			continue
		default:
			nFiles++
			nViews += applied
		}
		if now.Sub(f.Day) > agentLogRetention {
			if err := os.Remove(f.Path); err != nil {
				log.Printf("rollup-views: remove %s: %v", filepath.Base(f.Path), err)
			}
		}
	}
	return nFiles, nViews, errors.Join(errs...)
}

// applyAgentDay reads one day file and applies it, retrying a deadlock.
func applyAgentDay(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, path string) (int, error) {
	counts, sig, err := aggregateAgentFile(path)
	if err != nil {
		return 0, err
	}
	return applyWithRetry(ctx, filepath.Base(path), func() (int, error) {
		return applyAgentFile(ctx, pool, q, path, counts, sig)
	})
}

// aggregateAgentFile reads one day file and its cursor signature in one pass. A file
// with lines and not one readable line fails the run: a day that parses as nothing would
// otherwise count nothing in silence — the trap viewlog/AGENTS.md names for nginx.
func aggregateAgentFile(path string) (map[time.Time]map[int64]int, int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, 0, err
	}
	defer func() { _ = f.Close() }()
	h := fnv.New64a()
	counts, stats, err := agentview.Aggregate(io.TeeReader(f, h))
	if err != nil {
		return nil, 0, err
	}
	if stats.Lines > 0 && stats.Bad == stats.Lines {
		return nil, 0, fmt.Errorf("none of %d lines could be read", stats.Lines)
	}
	if stats.Bad > 0 {
		log.Printf("rollup-views: %s: skipped %d unreadable line(s) of %d", filepath.Base(path), stats.Bad, stats.Lines)
	}
	return counts, int64(h.Sum64()), nil
}

// errAlreadyApplied reports a file another run has already claimed.
var errAlreadyApplied = errors.New("agent view file already applied")

// applyAgentFile claims one file's signature and applies its counts in one transaction.
// The claim comes FIRST: a run that loses it to another run waits on that run's row and
// then adds nothing, where a read-then-apply would let both add the day's counts.
func applyAgentFile(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, path string, counts map[time.Time]map[int64]int, sig int64) (int, error) {
	params, total := buildAgentParams(counts)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)
	claimed, err := qtx.MarkViewLogFileProcessed(ctx, db.MarkViewLogFileProcessedParams{
		Signature: sig, Filename: filepath.Base(path),
	})
	if err != nil {
		return 0, err
	}
	if claimed == 0 {
		return 0, errAlreadyApplied
	}
	if len(params) > 0 {
		br := qtx.ApplyDailyAgentView(ctx, params)
		var batchErr error
		br.Exec(func(_ int, e error) {
			if e != nil && batchErr == nil {
				batchErr = e
			}
		})
		if cerr := br.Close(); cerr != nil && batchErr == nil {
			batchErr = cerr
		}
		if batchErr != nil {
			return 0, batchErr
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return total, nil
}

// buildAgentParams flattens counts into the batch in ascending (job, day) order — the
// lock-order rule buildParams documents — and reports the views it carries.
func buildAgentParams(counts map[time.Time]map[int64]int) ([]db.ApplyDailyAgentViewParams, int) {
	var params []db.ApplyDailyAgentViewParams
	total := 0
	for day, perJob := range counts {
		for id, n := range perJob {
			params = append(params, db.ApplyDailyAgentViewParams{
				Day: pgtype.Date{Time: day, Valid: true}, JobID: id, Delta: int32(n),
			})
			total += n
		}
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].JobID != params[j].JobID {
			return params[i].JobID < params[j].JobID
		}
		return params[i].Day.Time.Before(params[j].Day.Time)
	})
	return params, total
}
