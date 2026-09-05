// Command rollup-views is the standalone view-count aggregation worker. It counts
// job views off the request path by reading nginx access logs: a serving request
// never writes a counter, so the read path stays cheap and cacheable.
//
// Each run lists the rotated (non-live) access-log files, and for every file not
// already applied (tracked by filesystem identity in processed_view_logs) it parses
// the lines, counts unique daily visitors per job — the SSR page GET /jobs/<slug>
// (bot-filtered) and the API read GET /api/v1/jobs/<slug> (not) — and applies the
// per-(day, job) uniques additively into job_daily_views and jobs.view_count. The
// day is taken from each line's timestamp, so a file spanning midnight is bucketed
// correctly, and the additive apply lets a day split across two files sum right.
//
// Without --backfill it processes only uncompressed rotated files (the recent ones,
// a light daily run); with --backfill it also reads the older .gz history to seed
// the baseline. Either way the per-file cursor makes re-runs idempotent. Where no
// log dir exists (local/dev) it is a clean no-op. It is a run-once-and-exit worker
// (cron-scheduled daily), exiting non-zero on failure so cron can alert.
package main

import (
	"context"
	"flag"
	"fmt"
	"hash/fnv"
	"io"
	"log"
	"math/rand/v2"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/application/viewlog"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgerr"
	"github.com/strelov1/freehire/internal/platform/worker"
)

func main() { worker.Main(run) }

func run() int {
	backfill := flag.Bool("backfill", false, "also process older .gz history, not just the recent uncompressed rotated files")
	flag.Parse()

	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	dir := envOr("VIEW_LOG_DIR", "/var/log/nginx")
	base := envOr("VIEW_LOG_BASE", "access.log")

	files, err := viewlog.RotatedFiles(dir, base)
	if err != nil {
		log.Printf("list logs in %s: %v", dir, err)
		return 1
	}
	if !*backfill {
		files = uncompressed(files)
	}
	if len(files) == 0 {
		log.Printf("rollup-views: no rotated logs to process in %s", dir)
		return 0
	}

	nFiles, nViews, err := process(ctx, pool, files)
	if err != nil {
		log.Printf("process: %v", err)
		return 1
	}
	log.Printf("rollup-views: processed %d file(s), applied %d view(s)", nFiles, nViews)
	return 0
}

// process applies every not-yet-processed file, skipping those already in the
// cursor. It returns how many files it applied and the total views added.
func process(ctx context.Context, pool *pgxpool.Pool, files []viewlog.LogFile) (nFiles, nViews int, err error) {
	q := db.New(pool)
	for _, f := range files {
		counts, sig, err := aggregateFile(f)
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
		applied, err := applyWithRetry(ctx, pool, q, f, counts, sig)
		if err != nil {
			return nFiles, nViews, err
		}
		nFiles++
		nViews += applied
	}
	return nFiles, nViews, nil
}

// deadlockRetries is how many times a file's apply is re-attempted after PostgreSQL
// breaks a lock cycle by aborting it. Small on purpose: a deadlock clears as soon as
// the other writer commits, so if three tries in a row lose, something other than
// ordinary contention is wrong and the run should say so rather than grind.
const deadlockRetries = 3

// applyWithRetry runs applyFile, re-attempting it when PostgreSQL aborts the
// transaction to break a deadlock.
//
// A deadlock says nothing was wrong with the work — only that two writers wanted the
// same rows in opposite orders — and applyFile is all-or-nothing: the file's counts
// and its processed-file mark commit together, so an aborted attempt leaves no
// partial state and a retry cannot double-count. Treating 40P01 as fatal is what
// stopped the view rollup for two days in September 2026 while every other worker
// carried on and nothing looked broken.
func applyWithRetry(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, f viewlog.LogFile, counts map[string]map[string]viewlog.Counts, sig int64) (int, error) {
	var err error
	for attempt := 1; attempt <= deadlockRetries; attempt++ {
		var applied int
		applied, err = applyFile(ctx, pool, q, f, counts, sig)
		if err == nil {
			return applied, nil
		}
		if !pgerr.IsDeadlock(err) {
			return 0, err
		}
		log.Printf("rollup-views: %s: deadlock on attempt %d/%d, retrying: %v",
			filepath.Base(f.Path), attempt, deadlockRetries, err)

		// Backs off, and jitters, because the writer we lost to is most likely one of
		// the ingest runs that share this slot — retrying instantly would just re-enter
		// the same cycle.
		delay := time.Duration(attempt) * 2 * time.Second
		select {
		case <-ctx.Done():
			return 0, ctx.Err()
		case <-time.After(delay + time.Duration(rand.Int64N(int64(time.Second)))):
		}
	}
	return 0, fmt.Errorf("apply %s: still deadlocking after %d attempts: %w",
		filepath.Base(f.Path), deadlockRetries, err)
}

// aggregateFile opens a rotated file, aggregates its views, and computes the cursor
// signature (FNV-64 over the decompressed content) in the same pass. The signature
// is stable across rename and gzip, so a re-run recognizes an already-applied file.
func aggregateFile(f viewlog.LogFile) (map[string]map[string]viewlog.Counts, int64, error) {
	rc, err := f.Open()
	if err != nil {
		return nil, 0, err
	}
	defer rc.Close()
	h := fnv.New64a()
	counts, err := viewlog.Aggregate(io.TeeReader(rc, h))
	if err != nil {
		return nil, 0, err
	}
	return counts, int64(h.Sum64()), nil
}

// applyFile applies one file's already-aggregated counts, then marks the file
// processed — all in one transaction, so a crash leaves neither a double-count nor
// a lost mark. It returns the total views applied. A file with no resolvable views
// is still marked (so it is not rescanned).
func applyFile(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, f viewlog.LogFile, counts map[string]map[string]viewlog.Counts, sig int64) (int, error) {
	ids, err := resolveSlugs(ctx, q, counts)
	if err != nil {
		return 0, err
	}

	params, total, err := buildParams(counts, ids)
	if err != nil {
		return 0, err
	}

	tx, err := pool.Begin(ctx)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback(ctx)
	qtx := q.WithTx(tx)

	if len(params) > 0 {
		br := qtx.ApplyDailyView(ctx, params)
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
		Signature: sig, Filename: filepath.Base(f.Path),
	}); err != nil {
		return 0, err
	}
	if err := tx.Commit(ctx); err != nil {
		return 0, err
	}
	return total, nil
}

// buildParams turns a file's aggregated counts into the batch, in a deterministic
// order, and reports the total views it carries. Slugs that resolve to no job are
// dropped.
//
// The ORDER is load-bearing rather than tidiness. counts is a map of maps, and Go
// randomises map iteration, so before this every run took row locks on up to several
// hundred thousand `jobs` rows in a fresh random order, inside one long transaction,
// while several ingest runs wrote to the same table beside it. That is a lock cycle
// waiting to happen, and on 2026-09-05 it happened: 40P01, the run died, and the view
// rollup stopped for two days without anything else noticing.
//
// Ascending job id gives this worker one stable order. Two of its own batches can no
// longer deadlock each other at all, and against another writer an overlap becomes a
// wait rather than a cycle.
func buildParams(counts map[string]map[string]viewlog.Counts, ids map[string]int64) ([]db.ApplyDailyViewParams, int, error) {
	var params []db.ApplyDailyViewParams
	total := 0
	for day, perSlug := range counts {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			return nil, 0, err
		}
		for slug, c := range perSlug {
			id, ok := ids[slug]
			if !ok {
				continue
			}
			params = append(params, db.ApplyDailyViewParams{
				Day:        pgtype.Date{Time: d, Valid: true},
				JobID:      id,
				TotalDelta: int32(c.Total),
				PageDelta:  int32(c.Page),
			})
			// The reported figure stays the total: it is what jobs.view_count accrues
			// and what this worker has always logged.
			total += c.Total
		}
	}

	sort.Slice(params, func(i, j int) bool {
		if params[i].JobID != params[j].JobID {
			return params[i].JobID < params[j].JobID
		}
		return params[i].Day.Time.Before(params[j].Day.Time)
	})
	return params, total, nil
}

// resolveSlugs maps every slug appearing in counts to its job id in one query.
func resolveSlugs(ctx context.Context, q *db.Queries, counts map[string]map[string]viewlog.Counts) (map[string]int64, error) {
	set := make(map[string]struct{})
	for _, perSlug := range counts {
		for slug := range perSlug {
			set[slug] = struct{}{}
		}
	}
	if len(set) == 0 {
		return nil, nil
	}
	slugs := make([]string, 0, len(set))
	for slug := range set {
		slugs = append(slugs, slug)
	}
	rows, err := q.ResolveSlugsToJobIDs(ctx, slugs)
	if err != nil {
		return nil, err
	}
	ids := make(map[string]int64, len(rows))
	for _, r := range rows {
		ids[r.PublicSlug] = r.ID
	}
	return ids, nil
}

// uncompressed drops the .gz history, leaving the recent rotated files for a light
// daily run.
func uncompressed(files []viewlog.LogFile) []viewlog.LogFile {
	out := files[:0:0]
	for _, f := range files {
		if !strings.HasSuffix(f.Path, ".gz") {
			out = append(out, f)
		}
	}
	return out
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
