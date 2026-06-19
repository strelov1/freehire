// Command import-collections populates the curated-collection membership defined
// in internal/collections. For each collection it resolves the member companies
// (yc from the open yc-oss dataset, matched by normalized name; bigtech from a
// hand-coded slug list), writes companies.collections — reconciling only the tags
// it manages so any other tags survive — then denormalizes the result onto
// jobs.collections. It is a run-once-and-exit worker; a search reindex is required
// afterwards to surface the changes in the facet index.
//
// Idempotent: re-running with the same dataset writes the same membership and
// changes nothing on the second pass.
package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"slices"
	"time"

	"github.com/strelov1/freehire/internal/collections"
	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/worker"
)

// defaultYCDatasetURL is the open yc-oss mirror of the YC company directory (a JSON
// array of company objects). Overridable via YC_DATASET_URL for pinning or testing.
const defaultYCDatasetURL = "https://yc-oss.github.io/api/companies/all.json"

// fetchTimeout bounds the dataset download so a stalled endpoint can't hang the run.
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

	ycNames, err := fetchYC(ctx, ycDatasetURL())
	if err != nil {
		log.Printf("import-collections: fetch yc dataset: %v", err)
		return 1
	}

	q := db.New(pool)
	rows, err := q.ListCompanyCollections(ctx)
	if err != nil {
		log.Printf("import-collections: list companies: %v", err)
		return 1
	}

	p := plan(rows, ycNames)
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

	log.Printf("import-collections done: yc matched=%d unmatched=%d, bigtech matched=%d unmatched=%d; companies updated=%d, jobs updated=%d",
		p.ycMatched, p.ycUnmatched, p.bigMatched, p.bigUnmatched, len(p.writes), propagated)
	log.Printf("import-collections: run `make reindex` to surface jobs.collections in the search index")
	return 0
}

// planResult is the computed membership change plus the match diagnostics logged
// at the end of a run.
type planResult struct {
	writes       []db.SetCompanyCollectionsParams
	ycMatched    int
	ycUnmatched  int
	bigMatched   int
	bigUnmatched int
}

// plan computes the membership change for every company: it matches the yc names
// and the bigtech slug list against the existing companies, then reconciles each
// company's managed tags (preserving any unmanaged ones), emitting a write only for
// the companies whose set actually changes. It is pure — all I/O lives in run.
func plan(rows []db.ListCompanyCollectionsRow, ycNames []string) planResult {
	existing := make(map[string]struct{}, len(rows))
	for _, r := range rows {
		existing[r.Slug] = struct{}{}
	}

	ycMatched, ycUnmatched := collections.Match(ycNames, existing)
	bigMatched, bigUnmatched := collections.Match(collections.BigTechSlugs, existing)

	want := make(map[string][]string, len(ycMatched)+len(bigMatched))
	for _, slug := range ycMatched {
		want[slug] = append(want[slug], "yc")
	}
	for _, slug := range bigMatched {
		want[slug] = append(want[slug], "bigtech")
	}

	managed := collections.Slugs()
	var writes []db.SetCompanyCollectionsParams
	for _, r := range rows {
		next := collections.Reconcile(r.Collections, managed, want[r.Slug])
		if !slices.Equal(next, normalizedCurrent(r.Collections)) {
			writes = append(writes, db.SetCompanyCollectionsParams{Slug: r.Slug, Collections: next})
		}
	}

	return planResult{
		writes:       writes,
		ycMatched:    len(ycMatched),
		ycUnmatched:  len(ycUnmatched),
		bigMatched:   len(bigMatched),
		bigUnmatched: len(bigUnmatched),
	}
}

// normalizedCurrent sorts a copy of the stored collections so the change check
// compares against Reconcile's sorted output (membership is a set; column order is
// not meaningful and must not trigger a spurious write).
func normalizedCurrent(current []string) []string {
	cp := slices.Clone(current)
	slices.Sort(cp)
	return cp
}

// ycDatasetURL returns the dataset URL, honouring a YC_DATASET_URL override.
func ycDatasetURL() string {
	if u := os.Getenv("YC_DATASET_URL"); u != "" {
		return u
	}
	return defaultYCDatasetURL
}

// fetchYC downloads the yc-oss dataset and extracts the company names. The URL is a
// constant we control (not user input), so a plain client is appropriate.
func fetchYC(ctx context.Context, url string) ([]string, error) {
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
	return collections.ParseYC(body)
}
