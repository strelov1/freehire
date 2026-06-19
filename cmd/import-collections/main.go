// Command import-collections populates the curated-collection membership defined
// in internal/collections. For each collection it resolves the member companies —
// a static hand list (e.g. bigtech) or a remote name dataset (e.g. yc, unicorn),
// matched to our companies by normalized name — writes companies.collections
// (reconciling only the tags it manages so any other tags survive), then
// denormalizes the result onto jobs.collections. It is a run-once-and-exit worker;
// a search reindex is required afterwards to surface the changes in the facet index.
//
// Idempotent: re-running with the same inputs writes the same membership and
// changes nothing on the second pass. If any collection's dataset cannot be
// resolved the run aborts before writing — a partial resolve would otherwise drop
// that collection's tags from every company.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

// fetchTimeout bounds each dataset download so a stalled endpoint can't hang the run.
const fetchTimeout = 60 * time.Second

func main() {
	os.Exit(run())
}

func run() int {
	ctx, _, pool, cleanup, err := worker.Bootstrap(context.Background())
	if err != nil {
		log.Printf("database: %v", err)
		return 1
	}
	defer cleanup()

	// Resolve every collection's candidate company names/slugs up front. A failure
	// here aborts before any write: proceeding with a collection missing would
	// reconcile its tag off every company (a transient fetch error must not wipe
	// membership).
	resolved, err := resolveAll(ctx)
	if err != nil {
		log.Printf("import-collections: %v", err)
		return 1
	}

	q := db.New(pool)
	rows, err := q.ListCompanyCollections(ctx)
	if err != nil {
		log.Printf("import-collections: list companies: %v", err)
		return 1
	}

	p := plan(rows, resolved)
	for _, w := range p.writes {
		if err := q.SetCompanyCollections(ctx, w); err != nil {
			log.Printf("import-collections: set %q: %v", w.Slug, err)
			return 1
		}
	}

	propagated, err := q.PropagateCollectionsToJobs(ctx)
	if err != nil {
		log.Printf("import-collections: propagate to jobs: %v", err)
		return 1
	}

	for _, c := range collections.All {
		s := p.stats[c.Slug]
		log.Printf("import-collections: %s matched=%d unmatched=%d", c.Slug, s.matched, s.unmatched)
		// For a hand list the unmatched entries are actionable (a typo'd slug, or a
		// marquee company we don't ingest yet), so list them. Datasets have thousands
		// of unmatched names — only their count is logged, above.
		if len(s.unmatchedNames) > 0 {
			log.Printf("import-collections: %s unmatched entries: %s", c.Slug, strings.Join(s.unmatchedNames, ", "))
		}
	}
	log.Printf("import-collections done: companies updated=%d, jobs updated=%d", len(p.writes), propagated)
	log.Printf("import-collections: run `make reindex` to surface jobs.collections in the search index")
	return 0
}

// resolveAll returns, per collection slug, its candidate company names (or slugs
// for a hand list). A static-list collection resolves locally; a dataset
// collection is fetched and parsed. Any resolution failure is returned (the caller
// aborts rather than write a partial membership).
func resolveAll(ctx context.Context) (map[string][]string, error) {
	resolved := make(map[string][]string, len(collections.All))
	for _, c := range collections.All {
		switch {
		case c.Slugs != nil:
			resolved[c.Slug] = c.Slugs
		case c.Dataset != nil:
			names, err := fetchDataset(ctx, datasetURL(c), c.Dataset.Parse)
			if err != nil {
				return nil, fmt.Errorf("resolve %q: %w", c.Slug, err)
			}
			resolved[c.Slug] = names
		default:
			return nil, fmt.Errorf("collection %q has no membership source", c.Slug)
		}
	}
	return resolved, nil
}

// matchStat is the per-collection match outcome, logged at the end of a run.
// unmatchedNames holds the verbatim unmatched entries for hand-list collections
// (small and actionable); it is left empty for datasets (their unmatched set runs
// to thousands, so only the count is kept).
type matchStat struct {
	matched, unmatched int
	unmatchedNames     []string
}

// planResult is the computed membership change plus the per-collection match stats.
type planResult struct {
	writes []db.SetCompanyCollectionsParams
	stats  map[string]matchStat
}

// plan computes the membership change for every company: it matches each
// collection's candidates against the existing companies, then reconciles each
// company's managed tags (preserving any unmanaged ones), emitting a write only for
// the companies whose set actually changes. It is pure — all I/O lives in run.
// `resolved` maps a collection slug to its candidate company names/slugs.
func plan(rows []db.ListCompanyCollectionsRow, resolved map[string][]string) planResult {
	existing := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		existing[r.Slug] = struct{}{}
	}

	want := make(map[string][]string)
	stats := make(map[string]matchStat, len(resolved))
	for _, c := range collections.All {
		matched, unmatched := collections.Match(resolved[c.Slug], existing)
		s := matchStat{matched: len(matched), unmatched: len(unmatched)}
		if c.Slugs != nil { // hand list: keep the unmatched entries for diagnostics
			s.unmatchedNames = unmatched
		}
		stats[c.Slug] = s
		for _, slug := range matched {
			want[slug] = append(want[slug], c.Slug)
		}
	}

	managed := collections.Slugs()
	var writes []db.SetCompanyCollectionsParams
	for _, r := range rows {
		next := collections.Reconcile(r.Collections, managed, want[r.Slug])
		if !slices.Equal(next, normalizedCurrent(r.Collections)) {
			writes = append(writes, db.SetCompanyCollectionsParams{Slug: r.Slug, Collections: next})
		}
	}

	return planResult{writes: writes, stats: stats}
}

// normalizedCurrent sorts a copy of the stored collections so the change check
// compares against Reconcile's sorted output (membership is a set; column order is
// not meaningful and must not trigger a spurious write).
func normalizedCurrent(current []string) []string {
	cp := slices.Clone(current)
	slices.Sort(cp)
	return cp
}

// datasetURL returns a collection's dataset URL, honouring a <SLUG>_DATASET_URL
// environment override (e.g. YC_DATASET_URL, UNICORN_DATASET_URL).
func datasetURL(c collections.Collection) string {
	env := strings.ToUpper(c.Slug) + "_DATASET_URL"
	if u := os.Getenv(env); u != "" {
		return u
	}
	return c.Dataset.URL
}

// fetchDataset downloads a dataset URL and runs its parser. The URLs are constants
// we control (not user input), so a plain client is appropriate.
func fetchDataset(ctx context.Context, url string, parse func([]byte) ([]string, error)) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("dataset %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	return parse(body)
}
