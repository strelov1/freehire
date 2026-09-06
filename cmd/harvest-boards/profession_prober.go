package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// professionProber harvests Profession.hu, whose boards are not employers but the
// platform's own job categories. Three things follow, and they are why it has a prober of
// its own rather than falling through to adapterProber.
//
// It DISCOVERS. The platform publishes one sitemap index naming every category it
// currently has, which is the complete and authoritative board list — there is no seed
// file to write and nothing to keep in step by hand. A category the platform adds appears
// on the next harvest; one it retires stops being offered.
//
// It probes CHEAPLY. adapterProber measures a board by running the source adapter over it,
// which for this provider means crawling the category: a detail request per posting,
// against ~12k postings across the categories. Counting the URLs a category's sitemap
// lists costs one request and answers the same question, because liveness here is exactly
// "does this category still list postings".
//
// It reads the index ONCE. probeAll runs candidates concurrently, so a prober that
// resolved the index per candidate would ask for the same document 23 times in parallel.
// Measured live on 2026-09-06 the platform answered the first such request and closed the
// connection on the rest — which arrives as every category being unreachable, and looks
// nothing like the crawl asking too often. The map discovery already fetched is therefore
// kept and probe() is a lookup plus one request for the category's own sitemap.
//
// The board it yields is a category id, and no employer name exists to report, so the
// label names the platform and the category — the shape the two IT rows already carry. A
// curator who wants the platform's own wording for a category renames the row afterwards
// (cmd/add-board --rename); nothing about the crawl depends on it.
type professionProber struct{ index *professionCategoryIndex }

// professionCategoryIndex memoizes the category-to-sitemap map for one harvest run. Only a
// successful read is kept: a transient failure on the first candidate must not decide the
// other 22.
type professionCategoryIndex struct {
	mu       sync.Mutex
	sitemaps map[string]string
}

func (i *professionCategoryIndex) get(ctx context.Context, c httpClient) (map[string]string, error) {
	i.mu.Lock()
	defer i.mu.Unlock()
	if i.sitemaps != nil {
		return i.sitemaps, nil
	}
	sitemaps, err := sources.ProfessionCategorySitemaps(ctx, c)
	if err != nil {
		return nil, err
	}
	i.sitemaps = sitemaps
	return sitemaps, nil
}

func (p professionProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	sitemaps, err := p.index.get(ctx, c)
	if err != nil {
		return nil, err
	}
	return slices.Sorted(maps.Keys(sitemaps)), nil
}

func (p professionProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	sitemaps, err := p.index.get(ctx, c)
	if err != nil {
		return "", 0, err
	}
	sitemap, ok := sitemaps[board]
	if !ok {
		// The index does not publish this category. That is not an empty board and must
		// not be reported as one: an empty category is live and may fill up again, while
		// this one no longer exists.
		return "", 0, fmt.Errorf("profession: category %s is not in the sitemap index", board)
	}
	open, err := sources.ProfessionSitemapPostings(ctx, c, sitemap)
	if err != nil {
		return "", 0, fmt.Errorf("profession: category %s: %w", board, err)
	}
	if open == 0 {
		return "", 0, nil
	}
	return fmt.Sprintf("Profession.hu — %s", board), open, nil
}
