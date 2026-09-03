// Command build-suggestions rebuilds the search box's suggestion dictionary.
//
// The dictionary is what the box offers as you type: the posting titles the catalogue
// actually carries, plus the role, skill, category and company vocabularies, each with
// the open postings behind it. It exists because the facet dictionaries cannot answer
// what people type — 8,680 postings are titled "Product Owner" and the role vocabulary
// has no such role, only an alias folding it into "Product Manager", so the box
// answered a question about real postings by renaming the person asking.
//
// It is a run-once-and-exit worker. There is no incremental path and no outbox: the
// dictionary is derived wholesale from one catalogue pass, and every run replaces it
// through an index swap, so a reader never sees a half-built dictionary.
//
// Needs DATABASE_URL, MEILI_URL and MEILI_MASTER_KEY. SUGGEST_TITLE_FLOOR and
// SUGGEST_COMPANY_FLOOR tune how much of the long tail becomes vocabulary.
//
// Meilisearch runs ONE serial task queue, so this must not be scheduled on top of the
// jobs reindex: the swap would queue behind that rebuild and look like a hang. The
// dictionary is small, so the wait is the whole cost — the build itself is not.
package main

import (
	"context"
	"log"
	"math"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/dict/roletag"
	"github.com/strelov1/freehire/internal/platform/config"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/worker"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/suggest"
)

// pushBatch is how many documents go to Meilisearch per request. The dictionary is
// tens of thousands of small documents, so this is about request size rather than
// pacing — Push does not wait, and Promote awaits every task at once.
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

	q := db.New(pool)
	client := search.NewClient(cfg.MeiliURL, cfg.MeiliKey)

	in, err := gather(ctx, q, client, floors)
	if err != nil {
		log.Printf("gather: %v", err)
		return 1
	}
	docs := suggest.Build(in)
	if len(docs) == 0 {
		// An empty dictionary would swap a working one out for nothing. That is a
		// failed measurement, not a catalogue with nothing in it.
		log.Print("build: dictionary is empty; refusing to swap")
		return 1
	}

	if err := writeIndex(ctx, client, docs); err != nil {
		log.Printf("write index: %v", err)
		return 1
	}
	log.Printf("suggestions: %d documents (title floor %d, company floor %d)",
		len(docs), floors.TitleFloor, floors.CompanyFloor)

	// Retention runs AFTER the swap, and after the build has already read the table:
	// a failure here costs the sweep, never the dictionary.
	pruneDemand(ctx, q)
	return 0
}

// demandRetention is how long a phrase searched exactly once is kept.
//
// The write path already refuses anything that is not a search phrase, so what
// accumulates is real but transient — a one-off typo, a title that no longer exists.
// Ninety days is long enough that a quarterly hiring phrase is still there when it
// comes round, and the `count = 1` condition means a phrase two people searched is
// never swept at all.
const demandRetention = 90 * 24 * time.Hour

func pruneDemand(ctx context.Context, q *db.Queries) {
	n, err := q.PruneSearchQueries(ctx, pgtype.Timestamptz{Time: time.Now().Add(-demandRetention), Valid: true})
	if err != nil {
		log.Printf("prune demand: %v", err)
		return
	}
	if n > 0 {
		log.Printf("demand: pruned %d one-off phrases older than %s", n, demandRetention)
	}
}

// gather collects everything the dictionary is built from. The titles and companies
// come from Postgres; the facet counts come from the live search index, which is the
// same source the filters read — so a suggestion's figure and the filter it applies
// cannot disagree.
func gather(ctx context.Context, q *db.Queries, client *search.Client, floors config.Suggest) (suggest.Input, error) {
	titles, err := q.MineJobTitles(ctx)
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

	counts, err := client.FacetCounts(ctx, search.FacetParams{
		Facets: []string{"role", "skills", "enrichment.category"},
	})
	if err != nil {
		return suggest.Input{}, err
	}

	// Demand. Already normalised on the write path, so it joins the dictionary's own
	// keys without a second pass — that shared normalisation is the whole reason a
	// typed query can reach the title it names.
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
		Roles:      ints(counts.Facets["role"]),
		// The catalogue's own slug→label map, so a suggestion reads exactly as the
		// filter chip it applies. A second label table would drift from the picker's.
		RoleLabels: roletag.Catalog(),
		Skills:     ints(counts.Facets["skills"]),
		Categories: ints(counts.Facets["enrichment.category"]),
		Companies:  firms,
		Searches:   demand,
	}, nil
}

// narrow converts a floor to the int32 Postgres takes, checking the bound HERE rather
// than trusting it from the config.
//
// Two guards for one value, and they answer different questions: config.LoadSuggest
// decides whether the floor is a usable NUMBER (its `int` is what the builder filters
// titles with), while this decides whether it survives the TYPE it crosses into. The
// second is not implied by the first — narrowing a wider value wraps, and a wrapped
// floor can come out negative, which admits every row instead of excluding the tail.
// Keeping it at the conversion is also what makes the check visible to a reader (and
// to a static analyser) standing at the line where the risk is.
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
