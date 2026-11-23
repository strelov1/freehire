// Command build-suggestions rebuilds the search box's suggestion dictionary.
//
// The dictionary is what the box offers as you type: the posting titles the catalogue carries,
// plus the role, skill, category and company vocabularies, each with the open postings behind
// it. It exists because the facet dictionaries cannot answer what people type (many postings are
// titled "Product Owner", which the role vocabulary only knows as an alias of "Product Manager").
//
// Run-once-and-exit: no incremental path and no outbox. The dictionary is derived wholesale from
// one catalogue pass and replaces the old one through an index swap, so a reader never sees a
// half-built dictionary.
//
// Needs DATABASE_URL, MEILI_URL and MEILI_MASTER_KEY. SUGGEST_TITLE_FLOOR and
// SUGGEST_COMPANY_FLOOR tune how much of the long tail becomes vocabulary.
//
// Meilisearch runs ONE serial task queue, so do not schedule this on top of the jobs reindex:
// the swap would queue behind that rebuild and look like a hang.
package main

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"
)

// pushBatch is how many documents go to Meilisearch per request; it is about request size, not
// pacing (Push does not wait, and Promote awaits every task at once).
const pushBatch = 1000

func main() {
	worker.Main(run)
}

func run() int {
	ctx, cfg, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	if cfg.MeiliKey == "" {
		log.Print("config: MEILI_MASTER_KEY is required")
		return 1
	}
	floors := config.LoadSuggest()

	// This swaps a separate index, and Meilisearch runs ONE serial task queue: a swap queued
	// behind a full jobs rebuild holds this Type=oneshot unit open and looks like a hang.
	// Skipping costs a day of a dictionary that is rebuilt daily.
	releaseLock, gotLock, err := worker.HoldHeavyIndexLock(ctx, pool)
	if err != nil {
		log.Printf("index lock: %v", err)
		return 1
	}
	if !gotLock {
		log.Print("build-suggestions: another catalogue-wide index job is running — skipping this run")
		return 0
	}
	defer releaseLock()

	analytics, err := worker.AnalyticsPool(ctx, pool)
	if err != nil {
		log.Printf("analytics database: %v", err)
		return 1
	}
	defer analytics.Close()

	q := db.New(pool)
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	in, err := gather(ctx, q, db.New(analytics), client, floors)
	if err != nil {
		log.Printf("gather: %v", err)
		return 1
	}
	docs := suggest.Build(in)
	if len(docs) == 0 {
		// An empty dictionary would swap a working one out for nothing: a failed measurement,
		// not an empty catalogue.
		log.Print("build: dictionary is empty; refusing to swap")
		return 1
	}

	if err := writeIndex(ctx, client, docs); err != nil {
		log.Printf("write index: %v", err)
		return 1
	}
	log.Printf("suggestions: %d documents (title floor %d, company floor %d)",
		len(docs), floors.TitleFloor, floors.CompanyFloor)

	// Retention runs AFTER the swap and the table read: a failure costs the sweep, never the
	// dictionary.
	pruneDemand(ctx, q)
	return 0
}

// demandRetention is how long a phrase searched exactly once is kept. The write path already
// refuses non-search phrases, so what accumulates is transient (a typo, a vanished title). The
// `count = 1` condition means a phrase two people searched is never swept.
const demandRetention = 90 * 24 * time.Hour

func pruneDemand(ctx context.Context, q *db.Queries) {
	n, err := q.PruneSearchQueries(ctx, pgconv.Instant(time.Now().Add(-demandRetention)))
	if err != nil {
		log.Printf("prune demand: %v", err)
		return
	}
	if n > 0 {
		log.Printf("demand: pruned %d one-off phrases older than %s", n, demandRetention)
	}
}

// gather collects everything the dictionary is built from. Titles and companies come from
// Postgres; facet counts come from the live search index, the source the filters read, so a
// suggestion's figure and its filter cannot disagree. The title scan is a whole-catalogue
// GROUP BY, so it reads the analytics replica; the rest are small reads of the primary.
func gather(ctx context.Context, q, analytics *db.Queries, client *search.Client, floors config.Suggest) (suggest.Input, error) {
	titles, err := worker.RetryStandbyConflict(ctx, "MineJobTitles", func() ([]db.MineJobTitlesRow, error) {
		return analytics.MineJobTitles(ctx)
	})
	if err != nil {
		return suggest.Input{}, err
	}
	raw := make(map[string]int, len(titles))
	for _, t := range titles {
		raw[t.Title] = int(t.Count)
	}

	companies, err := q.SuggestibleCompanies(ctx, narrow(floors.CompanyFloor))
	if err != nil {
		return suggest.Input{}, err
	}
	firms := make([]suggest.Company, 0, len(companies))
	for _, c := range companies {
		firms = append(firms, suggest.Company{Slug: c.Slug, Name: c.Name, Jobs: int(c.JobCount)})
	}

	// The INDEX attribute names, not the query-parameter names: the engine answers 400 to the
	// latter. Reading them from the filter builder's table stops the two drifting.
	skillAttr, categoryAttr := search.StringFacets["skills"], search.StringFacets["category"]
	counts, err := client.FacetCounts(ctx, search.FacetParams{
		Facets: []string{skillAttr, categoryAttr},
	})
	if err != nil {
		return suggest.Input{}, err
	}

	// Demand, already normalised on the write path so it joins the dictionary's own keys; that
	// shared normalisation is what lets a typed query reach the title it names.
	searched, err := q.SearchQueryCounts(ctx)
	if err != nil {
		return suggest.Input{}, err
	}
	demand := make(map[string]int, len(searched))
	for _, s := range searched {
		demand[s.Query] = int(s.Count)
	}

	return suggest.Input{
		Titles:     raw,
		TitleFloor: floors.TitleFloor,
		Skills:     ints(counts.Facets[skillAttr]),
		Categories: ints(counts.Facets[categoryAttr]),
		Companies:  firms,
		Searches:   demand,
	}, nil
}

// narrow converts a floor to the int32 Postgres takes, checking the bound HERE. config.LoadSuggest
// only validates the floor as a usable number; narrowing a wider value wraps and can come out
// negative, which admits every row instead of excluding the tail.
func narrow(n int) int32 {
	if n > math.MaxInt32 {
		return math.MaxInt32
	}
	if n < 1 {
		return 1
	}
	return int32(n)
}

func ints(m map[string]int64) map[string]int {
	out := make(map[string]int, len(m))
	for k, v := range m {
		out[k] = int(v)
	}
	return out
}

func writeIndex(ctx context.Context, client *search.Client, docs []suggest.Document) error {
	r := client.NewSuggestRebuild()
	if err := r.Prepare(ctx); err != nil {
		return err
	}
	// Cleanup is idempotent and tolerates absence, so a run that aborts before Promote
	// (whose swap-and-drop is the normal teardown) leaves no orphan index behind.
	defer func() { _ = r.Cleanup(ctx) }()

	for start := 0; start < len(docs); start += pushBatch {
		end := min(start+pushBatch, len(docs))
		if err := r.PushAny(ctx, docs[start:end]); err != nil {
			return err
		}
	}
	return r.Promote(ctx)
}
