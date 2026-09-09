// Command backfill-remote-perk-false-positive corrects jobs.work_mode for the rows
// that freehire#2696 already mis-set to "remote" before the fix landed, then exits.
//
// The obvious tool, cmd/backfill-derive, cannot reach these rows: it feeds a row's
// CURRENTLY STORED work_mode back into jobderive.Derive as the structured-signal input
// (cmd/backfill-derive/main.go, "preserves a set work_mode"), which is correct for its
// own purpose — never clobber a genuinely structured ATS signal — but means a value the
// OLD, buggy WorkModeFromDescription wrote is indistinguishable from a real one and gets
// carried forward unchanged. Ordinary re-ingest does not reach these rows either: once a
// posting is stored with a description, ingest's seen-set only confirms liveness on
// later crawls (see AGENTS.md, "BODY_REFRESH_DAYS"), so the description is never re-read
// and jobderive never re-runs against it.
//
// This pass re-derives ONLY the geography-marker and description-phrase layers of the
// work-mode precedence (recomputeWorkMode) — deliberately WITHOUT a structured-signal
// input, since the whole point is to stop trusting whatever is currently stored — and
// writes the result only when it differs from "remote" AND the stored value is still
// "remote". A row whose CURRENT work_mode is remote because of a real, structured ATS
// signal (lost the moment it was folded into the one work_mode column, and not
// recoverable from stored fields) cannot be told apart here from one the bug produced,
// so such a row is a false correction in principle. It is not a false correction for
// long: the next ordinary crawl of that posting passes ingest the ADAPTER's live
// structured signal directly (internal/ingest/pipeline.normalizeJob), which overwrites
// whatever this pass wrote — so a wrongly-cleared row self-heals on its own, and a
// genuinely wrongly-tagged row (the actual bug) does NOT self-heal, because nothing else
// ever revisits it. That asymmetry is why erring toward correction here is the right
// side to be wrong on.
//
// Candidates come from Meilisearch (work_mode = "remote" and the text query "work from
// anywhere"), the same reasoning cmd/backfill-clearance gives for the same choice: a
// `description` predicate over the whole table de-TOASTs the column for every row it
// examines, and the search index already holds the text. Over-fetching is free — the
// recompute decides, and a declined row simply keeps its stored value.
//
// Idempotent (SetJobWorkMode is IS DISTINCT FROM-guarded): a re-run writes nothing for a
// row already corrected, so stopping the pass mid-way costs nothing to resume.
//
// Needs no reindex: work_mode is not part of content_hash, so a corrected row's facet
// only reaches Meilisearch on a full `make reindex` — the same gap
// cmd/backfill-clearance documents; follow this pass with one.
//
// Needs DATABASE_URL, MEILI_URL and MEILI_MASTER_KEY.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// candidateQuery is the one broad text query that gathers rows worth re-deriving,
// combined with the work_mode filter below. Meilisearch's quoted-phrase syntax does not
// phrase-match on this index (cmd/backfill-clearance's own comment establishes this), so
// there is no precision to gain from a longer or more exact query — the recompute below
// is where precision comes from.
const candidateQuery = "work from anywhere"

// pageSize is how many hits one search request returns.
const pageSize = 1000

// readBatch is how many rows one database round trip fetches. Descriptions are TOASTed,
// so this bounds peak memory more than it bounds query time.
const readBatch = 500

// pauseBetweenBatches lets the host breathe, the same reasoning and value
// cmd/backfill-clearance uses: this pass is never urgent, and it competes with ingest and
// whatever reindex is running.
const pauseBetweenBatches = 100 * time.Millisecond

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Unset means unbounded; a value that fails to parse is an error, not a silent
	// fallback — the same EnvInt64 contract every one-off pass's cap now goes through.
	maxPerRun, err := worker.EnvInt64("BACKFILL_REMOTE_PERK_MAX", 0)
	if err != nil {
		log.Printf("backfill-remote-perk-false-positive: %v", err)
		return 1
	}

	sc := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	ids, err := candidateIDs(ctx, sc)
	if err != nil {
		log.Printf("backfill-remote-perk-false-positive: gather candidates: %v", err)
		return 1
	}
	if len(ids) == 0 {
		log.Print("backfill-remote-perk-false-positive: no candidates")
		return 0
	}
	if maxPerRun > 0 && int64(len(ids)) > maxPerRun {
		log.Printf("backfill-remote-perk-false-positive: capping this run at %d of %d candidates", maxPerRun, len(ids))
		ids = ids[:maxPerRun]
	}
	log.Printf("backfill-remote-perk-false-positive: %d candidates", len(ids))

	q := db.New(pool)
	var read, corrected, written int64
	lastLog := time.Now()
	for start := 0; start < len(ids); start += readBatch {
		end := min(start+readBatch, len(ids))
		rows, err := q.JobsForWorkModeRecheckByIDs(ctx, ids[start:end])
		if err != nil {
			log.Printf("backfill-remote-perk-false-positive: read %d..%d after %d written: %v", start, end, written, err)
			return 1
		}
		for _, r := range rows {
			read++
			// Meilisearch's own work_mode filter already scoped the candidates to
			// "remote", but the index can lag Postgres by however long the last
			// search-drain cycle took — re-check against the row actually read so a
			// row already corrected (by search or by a fresh crawl) since the
			// candidate query ran is never rewritten.
			if r.WorkMode != "remote" {
				continue
			}
			recomputed := recomputeWorkMode(r.Location, r.Description)
			if recomputed == "remote" {
				continue
			}
			corrected++
			n, err := q.SetJobWorkMode(ctx, db.SetJobWorkModeParams{ID: r.ID, WorkMode: recomputed})
			if err != nil {
				log.Printf("backfill-remote-perk-false-positive: write id=%d after %d written: %v", r.ID, written, err)
				return 1
			}
			written += n
		}
		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-remote-perk-false-positive: progress read=%d corrected=%d written=%d of %d",
				read, corrected, written, len(ids))
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-remote-perk-false-positive: cancelled after %d written, resume by re-running", written)
			return 1
		case <-time.After(pauseBetweenBatches):
		}
	}
	log.Printf("backfill-remote-perk-false-positive: done, read=%d corrected=%d written=%d (follow with a reindex)",
		read, corrected, written)
	return 0
}

// recomputeWorkMode re-derives a work mode from a job's location and description alone —
// the location-marker and description-phrase steps of jobderive's precedence chain, with
// NO structured-signal input. That omission is deliberate: the whole point of this pass
// is to stop trusting a row's currently-stored work_mode, which is exactly what
// cmd/backfill-derive does trust (see this package's doc comment).
func recomputeWorkMode(loc, description string) string {
	if geo := location.Parse(loc); geo.WorkMode != "" {
		return geo.WorkMode
	}
	workMode := location.WorkModeFromDescription(description)
	if workMode == "remote" && location.RemoteContradicted(description) {
		return "onsite"
	}
	return workMode
}

// candidateIDs gathers the ids of open-index jobs matching candidateQuery, restricted to
// the ones currently marked remote — the only population this pass could ever change.
func candidateIDs(ctx context.Context, sc *search.Client) ([]int64, error) {
	var ids []int64
	filter := search.Filter([]string{search.Eq("work_mode", "remote")})
	for offset := 0; ; offset += pageSize {
		res, err := sc.Search(ctx, search.SearchParams{
			Query:  candidateQuery,
			Filter: filter,
			Limit:  pageSize,
			Offset: offset,
		})
		if err != nil {
			return nil, err
		}
		for _, hit := range res.Hits {
			ids = append(ids, hit.ID)
		}
		if len(res.Hits) < pageSize {
			break
		}
	}
	return ids, nil
}
