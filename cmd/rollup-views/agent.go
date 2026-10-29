package main

import (
	"context"
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

// processAgent applies every finished, not-yet-applied day of the agent-view log, then
// removes the files past retention. The cursor is the same content-signature table the
// nginx pass uses, so a re-run applies nothing twice.
func processAgent(ctx context.Context, pool *pgxpool.Pool, dir string, now time.Time) (nFiles, nViews int, err error) {
	files, err := agentview.ListFiles(dir, now)
	if err != nil {
		return 0, 0, err
	}
	q := db.New(pool)
	for _, f := range files {
		counts, sig, err := aggregateAgentFile(f.Path)
		if err != nil {
			return nFiles, nViews, err
		}
		done, err := q.IsViewLogFileProcessed(ctx, sig)
		if err != nil {
			return nFiles, nViews, err
		}
		if done {
			continue
		}
		applied, err := applyWithRetry(ctx, filepath.Base(f.Path), func() (int, error) {
			return applyAgentFile(ctx, pool, q, f.Path, counts, sig)
		})
		if err != nil {
			return nFiles, nViews, err
		}
		nFiles++
		nViews += applied
	}
	for _, f := range files {
		if now.Sub(f.Day) > agentLogRetention {
			if err := os.Remove(f.Path); err != nil {
				log.Printf("rollup-views: remove %s: %v", filepath.Base(f.Path), err)
			}
		}
	}
	return nFiles, nViews, nil
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
		return nil, 0, fmt.Errorf("%s: none of %d lines could be read", filepath.Base(path), stats.Lines)
	}
	if stats.Bad > 0 {
		log.Printf("rollup-views: %s: skipped %d unreadable line(s) of %d", filepath.Base(path), stats.Bad, stats.Lines)
	}
	return counts, int64(h.Sum64()), nil
}

// applyAgentFile applies one file's counts and marks it processed in one transaction.
func applyAgentFile(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, path string, counts map[time.Time]map[int64]int, sig int64) (int, error) {
	params, total := buildAgentParams(counts)
	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)
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
	if err := qtx.MarkViewLogFileProcessed(ctx, db.MarkViewLogFileProcessedParams{
		Signature: sig, Filename: filepath.Base(path),
	}); err != nil {
		return 0, err
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
