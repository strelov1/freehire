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
// table the nginx pass uses, with the same chunked apply and resume cursor, so a re-run
// applies nothing twice.
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

// applyAgentDay reads one day file and applies it in chunks (see applyChunked).
func applyAgentDay(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, path string) (int, error) {
	counts, sig, err := aggregateAgentFile(path)
	if err != nil {
		return 0, err
	}
	done, err := q.IsViewLogFileProcessed(ctx, sig)
	if err != nil {
		return 0, err
	}
	if done {
		return 0, errAlreadyApplied
	}
	return applyChunked(ctx, pool, q, sig, filepath.Base(path), buildAgentParams(counts),
		func(p db.ApplyDailyAgentViewParams) viewKey { return viewKey{jobID: p.JobID, day: p.Day} },
		func(p db.ApplyDailyAgentViewParams) int { return int(p.Delta) },
		func(ctx context.Context, qtx *db.Queries, chunk []db.ApplyDailyAgentViewParams) error {
			return execBatch(qtx.ApplyDailyAgentView(ctx, chunk))
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

// buildAgentParams flattens counts into the batch in ascending (job, day) order — the
// lock-order rule buildParams documents, which is also the order applyChunked's cursor
// walks.
func buildAgentParams(counts map[time.Time]map[int64]int) []db.ApplyDailyAgentViewParams {
	var params []db.ApplyDailyAgentViewParams
	for day, perJob := range counts {
		for id, n := range perJob {
			params = append(params, db.ApplyDailyAgentViewParams{
				Day: pgtype.Date{Time: day, Valid: true}, JobID: id, Delta: int32(n),
			})
		}
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].JobID != params[j].JobID {
			return params[i].JobID < params[j].JobID
		}
		return params[i].Day.Time.Before(params[j].Day.Time)
	})
	return params
}
