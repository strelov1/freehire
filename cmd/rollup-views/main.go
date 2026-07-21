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
	"hash/fnv"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/viewlog"
	"github.com/strelov1/freehire/internal/worker"
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
		applied, err := applyFile(ctx, pool, q, f, counts, sig)
		if err != nil {
			return nFiles, nViews, err
		}
		nFiles++
		nViews += applied
	}
	return nFiles, nViews, nil
}

// aggregateFile opens a rotated file, aggregates its views, and computes the cursor
// signature (FNV-64 over the decompressed content) in the same pass. The signature
// is stable across rename and gzip, so a re-run recognizes an already-applied file.
func aggregateFile(f viewlog.LogFile) (map[string]map[string]int, int64, error) {
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
func applyFile(ctx context.Context, pool *pgxpool.Pool, q *db.Queries, f viewlog.LogFile, counts map[string]map[string]int, sig int64) (int, error) {
	ids, err := resolveSlugs(ctx, q, counts)
	if err != nil {
		return 0, err
	}

	var params []db.ApplyDailyViewParams
	total := 0
	for day, perSlug := range counts {
		d, err := time.Parse("2006-01-02", day)
		if err != nil {
			return 0, err
		}
		for slug, n := range perSlug {
			id, ok := ids[slug]
			if !ok {
				continue
			}
			params = append(params, db.ApplyDailyViewParams{
				Day: pgtype.Date{Time: d, Valid: true}, JobID: id, Delta: int32(n),
			})
			total += n
		}
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

// resolveSlugs maps every slug appearing in counts to its job id in one query.
func resolveSlugs(ctx context.Context, q *db.Queries, counts map[string]map[string]int) (map[string]int64, error) {
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
