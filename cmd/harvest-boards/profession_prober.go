package main

import (
	"context"
	"fmt"
	"maps"
	"slices"
	"sync"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// professionProber harvests Profession.hu, whose boards are the platform's own job categories, not
// employers. It has its own prober rather than falling through to adapterProber:
//
// It DISCOVERS: the platform's sitemap index lists every current category, so there is no seed file.
//
// It probes CHEAPLY: adapterProber would crawl the category (a detail request per posting, ~12k
// postings); counting the URLs in the category's sitemap is one request and answers the same question.
//
// It reads the index ONCE. probeAll runs candidates concurrently, and resolving the index per
// candidate sends the same request many times in parallel; the platform answers the first and closes
// the connection on the rest, which looks like every category being unreachable. The map discovery
// fetched is kept, so probe() is a lookup plus one sitemap request.
//
// It reads over its OWN client: the platform mishandles a connection it already answered on, and the
// shared pooled client would report the catalogue as unreachable after a candidate or two. The cost
// is that its requests fall outside the run's paced, counting client (as with adapterProber).
//
// The board is a category id with no employer name, so the label names the platform and category;
// a curator may rename the row afterwards (cmd/add-board --rename).
//
// Build it with newProfessionProber: the zero value has a nil index and panics on first use.
type professionProber struct {
	index  *professionCategoryIndex
	client sources.XMLGetter
}

func newProfessionProber() professionProber {
	return professionProber{
		index:  &professionCategoryIndex{},
		client: sources.NewSingleUseConnClient(),
	}
}

// professionCategoryIndex memoizes the category-to-sitemap map for one harvest run. Only a
// successful read is kept, so a transient failure on one candidate does not decide the rest.
type professionCategoryIndex struct {
	mu       sync.Mutex
	sitemaps map[string]string
}

func (i *professionCategoryIndex) get(ctx context.Context, c sources.XMLGetter) (map[string]string, error) {
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

// discover returns only the categories the crawl accepts (the platform's two IT sections) that the
// sitemap index still publishes; a renamed section drops out without an error. The adapter refuses
// the other categories (see profession.go), so offering them would offer boards that cannot be crawled.
func (p professionProber) discover(ctx context.Context, _ httpClient) ([]string, error) {
	sitemaps, err := p.index.get(ctx, p.client)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, cat := range slices.Sorted(maps.Keys(sitemaps)) {
		if sources.ProfessionCrawlsCategory(cat) {
			out = append(out, cat)
		}
	}
	return out, nil
}

func (p professionProber) probe(ctx context.Context, _ httpClient, board string) (string, int, error) {
	sitemaps, err := p.index.get(ctx, p.client)
	if err != nil {
		return "", 0, err
	}
	sitemap, ok := sitemaps[board]
	if !ok {
		// The index does not publish this category. Not an empty board: an empty category is live and
		// may fill up again, while this one no longer exists.
		return "", 0, fmt.Errorf("profession: category %s is not in the sitemap index", board)
	}
	open, err := sources.ProfessionSitemapPostings(ctx, p.client, sitemap)
	if err != nil {
		return "", 0, fmt.Errorf("profession: category %s: %w", board, err)
	}
	if open == 0 {
		return "", 0, nil
	}
	return fmt.Sprintf("Profession.hu — %s", board), open, nil
}
