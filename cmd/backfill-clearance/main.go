// Command backfill-clearance fills jobs.requires_clearance for the rows that predate
// the column, then exits.
//
// Every write path fills it going forward (jobderive, through the job aggregate); this
// is the one-off pass for the catalogue already in the table.
//
// It does NOT walk the catalogue. A `description` predicate over the whole table
// de-TOASTs the column for all 8M rows to find the ~38k that mention a clearance, and
// the existing cmd/backfill-derive — which re-derives every deterministic column and
// would pick this one up — runs about 15 hours. Meilisearch already indexes the
// description, so it names the candidates in seconds and this pass reads only their
// bodies.
//
// Over-fetching candidates is free. The search query is a RECALL device with no
// precision obligation: the dictionary decides, and a row it declines simply keeps
// NULL. That is why the queries below are broad single tokens rather than an attempt to
// restate the phrase list in Meilisearch's query language — a second, drifting copy of
// the dictionary is exactly what this design avoids.
//
// Run it repeatedly and it costs nothing: the UPDATE is guarded by IS DISTINCT FROM, so
// rows already correct are not rewritten and produce no dead tuples. Stopping it and
// re-running is the expected way to work through the backlog.
//
// Needs DATABASE_URL, MEILI_URL and MEILI_MASTER_KEY.
package main

import (
	"context"
	"log"
	"os"
	"strconv"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// candidateQueries are the search terms that gather rows worth re-deriving. They are
// deliberately broader and shorter than the dictionary's phrases: "clearance" alone
// catches every phrase containing the word, and the rest cover the anchors that do not
// contain it at all.
//
// Meilisearch's quoted-phrase syntax does NOT phrase-match on this index — "public
// trust" and "trust public" return identical counts — so there is no point writing
// phrases here even if precision were wanted. Single tokens say what these are.
var candidateQueries = []string{"clearance", "ts sci", "polygraph", "bpss", "vetting", "agsva"}

// pageSize is how many hits one search request returns. The index's maxTotalHits is
// 10M, so deep paging is available and the only cost of a small page is more requests.
const pageSize = 1000

// readBatch is how many descriptions one database round trip fetches. Descriptions are
// TOASTed, so this bounds peak memory more than it bounds query time.
const readBatch = 500

// pauseBetweenBatches lets the host breathe. The pass competes with ingest and with
// whatever reindex is running, and it is never urgent: until it completes the facet
// simply marks fewer postings, never the wrong ones.
const pauseBetweenBatches = 100 * time.Millisecond

func maxPerRun() int {
	if v, err := strconv.Atoi(os.Getenv("BACKFILL_CLEARANCE_MAX")); err == nil && v > 0 {
		return v
	}
	return 0 // unbounded
}

func main() { worker.Main(run) }

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	sc := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	ids, err := candidateIDs(ctx, sc)
	if err != nil {
		log.Printf("backfill-clearance: gather candidates: %v", err)
		return 1
	}
	if len(ids) == 0 {
		log.Print("backfill-clearance: no candidates")
		return 0
	}
	if n := maxPerRun(); n > 0 && len(ids) > n {
		log.Printf("backfill-clearance: capping this run at %d of %d candidates", n, len(ids))
		ids = ids[:n]
	}
	log.Printf("backfill-clearance: %d candidates", len(ids))

	q := db.New(pool)
	var read, marked, written int64
	lastLog := time.Now()
	for start := 0; start < len(ids); start += readBatch {
		end := min(start+readBatch, len(ids))
		rows, err := q.JobDescriptionsByIDs(ctx, ids[start:end])
		if err != nil {
			log.Printf("backfill-clearance: read %d..%d after %d written: %v", start, end, written, err)
			return 1
		}
		for _, r := range rows {
			read++
			// Only a positive is ever written. A row the dictionary declines keeps
			// whatever it had, which for an unvisited row is NULL — the honest "not
			// stated". Writing false here would assert the posting promises no
			// clearance, which is a different claim.
			if !location.RequiresClearanceFromDescription(r.Description) {
				continue
			}
			marked++
			n, err := q.SetJobRequiresClearance(ctx, db.SetJobRequiresClearanceParams{
				ID:                r.ID,
				RequiresClearance: pgconv.Bool(boolPtr(true)),
			})
			if err != nil {
				log.Printf("backfill-clearance: write id=%d after %d written: %v", r.ID, written, err)
				return 1
			}
			written += n
		}
		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-clearance: progress read=%d marked=%d written=%d of %d",
				read, marked, written, len(ids))
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-clearance: cancelled after %d written, resume by re-running", written)
			return 1
		case <-time.After(pauseBetweenBatches):
		}
	}
	// written < marked on a re-run: the guard skipped the rows already correct. That
	// gap is the signal the pass is converging, so both numbers are reported.
	log.Printf("backfill-clearance: done, read=%d marked=%d written=%d", read, marked, written)
	return 0
}

// candidateIDs gathers the union of ids matching any candidate query. Deduplicated
// across queries, because the terms overlap heavily — a TS/SCI posting almost always
// says "clearance" too — and a row derived twice would only waste a read.
func candidateIDs(ctx context.Context, sc *search.Client) ([]int64, error) {
	seen := make(map[int64]struct{})
	var ids []int64
	for _, query := range candidateQueries {
		for offset := 0; ; offset += pageSize {
			res, err := sc.Search(ctx, search.SearchParams{Query: query, Limit: pageSize, Offset: offset})
			if err != nil {
				return nil, err
			}
			for _, hit := range res.Hits {
				if _, dup := seen[hit.ID]; dup {
					continue
				}
				seen[hit.ID] = struct{}{}
				ids = append(ids, hit.ID)
			}
			if len(res.Hits) < pageSize {
				break
			}
		}
		log.Printf("backfill-clearance: after %q, %d distinct candidates", query, len(ids))
	}
	return ids, nil
}

func boolPtr(b bool) *bool { return &b }
