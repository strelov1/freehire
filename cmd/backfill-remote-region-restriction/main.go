// Command backfill-remote-region-restriction corrects jobs.countries/regions for the
// rows internal/dict/location.Parse's now-removed bare-remote-to-global default mis-set
// before the fix landed (see AGENTS.md's job-geography notes and the moderation report
// that found it: freehire job_reports 29-34).
//
// The obvious tool, cmd/backfill-derive, cannot reach these rows: it feeds a row's
// CURRENTLY STORED countries/regions back into jobderive.Derive as the structured-signal
// input (jobderive.go, "An explicit country/region signal is authoritative"), which is
// correct for its own purpose — never clobber a genuinely structured ATS signal — but
// means a stored `global` the old bug wrote is indistinguishable from a real one and gets
// carried forward unchanged. Ordinary re-ingest does not reach these rows either: once a
// posting is stored with a description, ingest's seen-set only confirms liveness on later
// crawls (see AGENTS.md, "BODY_REFRESH_DAYS"), so the row is never re-derived.
//
// This pass re-derives geography from a row's title/location/description ALONE — via the
// same jobderive.Derive precedence chain the fix now uses, deliberately WITHOUT a
// structured Countries/Regions input, since the whole point is to stop trusting whatever
// is currently stored — and writes the result only when it differs from the stored value.
// A row whose current `global` came from a real ATS signal that also happens to match the
// candidate query below is a false correction in principle, but self-heals on its next
// ordinary crawl, which passes ingest the adapter's live structured signal directly; a
// genuinely wrongly-tagged row has no such backstop, which is why erring toward correction
// here is deliberate — the same asymmetry cmd/backfill-remote-perk-false-positive documents
// for the analogous work_mode bug.
//
// Candidates come from Meilisearch (regions = "global"), the same reasoning
// cmd/backfill-clearance gives for the same choice: a `description` predicate over the
// whole table de-TOASTs the column for every row it examines, and the search index already
// holds the text. Over-fetching is free — the recompute decides, and a declined row simply
// keeps its stored value.
//
// Idempotent (SetJobGeography is IS DISTINCT FROM-guarded): a re-run writes nothing for a
// row already corrected, so stopping the pass mid-way costs nothing to resume.
//
// Needs a full `make reindex` afterward: countries/regions are not part of content_hash,
// so a corrected row's facet only reaches Meilisearch on a full rebuild — the same gap
// cmd/backfill-clearance documents.
//
// Needs DATABASE_URL, MEILI_URL and MEILI_MASTER_KEY.
package main

import (
	"context"
	"log"
	"time"

	"github.com/strelov1/freehire/internal/job/jobderive"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
)

// candidateQuery restricts the region filter below to a broad, over-fetching text query,
// the same shape cmd/backfill-remote-perk-false-positive and cmd/backfill-clearance use —
// there is no phrase-match precision to gain here, since the recompute is where precision
// comes from.
const candidateQuery = "remote"

// pageSize is how many hits one search request returns.
const pageSize = 1000

// readBatch is how many rows one database round trip fetches. Descriptions are TOASTed,
// so this bounds peak memory more than it bounds query time.
const readBatch = 500

// pauseBetweenBatches lets the host breathe, the same reasoning and value
// cmd/backfill-clearance and cmd/backfill-remote-perk-false-positive use: this pass is
// never urgent, and it competes with ingest and whatever reindex is running.
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
	maxPerRun, err := worker.EnvInt64("BACKFILL_REMOTE_REGION_MAX", 0)
	if err != nil {
		log.Printf("backfill-remote-region-restriction: %v", err)
		return 1
	}

	sc := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	ids, err := candidateIDs(ctx, sc)
	if err != nil {
		log.Printf("backfill-remote-region-restriction: gather candidates: %v", err)
		return 1
	}
	if len(ids) == 0 {
		log.Print("backfill-remote-region-restriction: no candidates")
		return 0
	}
	if maxPerRun > 0 && int64(len(ids)) > maxPerRun {
		log.Printf("backfill-remote-region-restriction: capping this run at %d of %d candidates", maxPerRun, len(ids))
		ids = ids[:maxPerRun]
	}
	log.Printf("backfill-remote-region-restriction: %d candidates", len(ids))

	q := db.New(pool)
	var read, corrected, written int64
	lastLog := time.Now()
	for start := 0; start < len(ids); start += readBatch {
		end := min(start+readBatch, len(ids))
		rows, err := q.JobsForGeographyRecheckByIDs(ctx, ids[start:end])
		if err != nil {
			log.Printf("backfill-remote-region-restriction: read %d..%d after %d written: %v", start, end, written, err)
			return 1
		}
		for _, r := range rows {
			read++
			// Meilisearch's own regions filter already scoped the candidates to
			// "global", but the index can lag Postgres by however long the last
			// search-drain cycle took — re-check against the row actually read so a
			// row already corrected (by this pass or by a fresh crawl) since the
			// candidate query ran is never rewritten.
			if !isBareGlobal(r.Countries, r.Regions) {
				continue
			}
			countries, regions := recomputeGeography(r.Title, r.Location, r.Description)
			if isBareGlobal(countries, regions) {
				continue
			}
			corrected++
			n, err := q.SetJobGeography(ctx, db.SetJobGeographyParams{ID: r.ID, Countries: countries, Regions: regions})
			if err != nil {
				log.Printf("backfill-remote-region-restriction: write id=%d after %d written: %v", r.ID, written, err)
				return 1
			}
			written += n
		}
		if time.Since(lastLog) >= time.Minute {
			log.Printf("backfill-remote-region-restriction: progress read=%d corrected=%d written=%d of %d",
				read, corrected, written, len(ids))
			lastLog = time.Now()
		}
		select {
		case <-ctx.Done():
			log.Printf("backfill-remote-region-restriction: cancelled after %d written, resume by re-running", written)
			return 1
		case <-time.After(pauseBetweenBatches):
		}
	}
	log.Printf("backfill-remote-region-restriction: done, read=%d corrected=%d written=%d (follow with a reindex)",
		read, corrected, written)
	return 0
}

// isBareGlobal reports whether a geography is exactly the bare-remote-with-no-signal
// shape this pass exists to correct: no countries, and regions holding nothing but
// "global". A row with any other region alongside global, or a real country, is left
// alone — this pass only ever moves a row OUT of the unqualified global bucket.
func isBareGlobal(countries, regions []string) bool {
	return len(countries) == 0 && len(regions) == 1 && regions[0] == "global"
}

// recomputeGeography re-derives countries/regions from a job's title, location and
// description alone, via the same jobderive.Derive precedence chain the fix now uses —
// deliberately with NO structured Countries/Regions input, since the whole point of this
// pass is to stop trusting a row's currently-stored geography (see this package's doc
// comment). jobderive is pure, so this is safe to call concurrently.
func recomputeGeography(title, loc, description string) (countries, regions []string) {
	d := jobderive.Derive(jobderive.Input{
		Title:       title,
		Location:    loc,
		Description: description,
	})
	return d.Countries, d.Regions
}

// candidateIDs gathers the ids of open-index jobs matching candidateQuery, restricted to
// the ones currently marked with the bare global region — the only population this pass
// could ever change.
func candidateIDs(ctx context.Context, sc *search.Client) ([]int64, error) {
	var ids []int64
	filter := search.Filter([]string{search.Eq("regions", "global")})
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
