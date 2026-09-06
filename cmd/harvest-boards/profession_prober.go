package main

import (
	"context"
	"fmt"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// professionProber harvests Profession.hu, whose boards are not employers but the
// platform's own job categories. Two things follow, and both are why it has a prober of
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
// The board it yields is a category id, and no employer name exists to report, so the
// label names the platform and the category — the shape the two IT rows already carry. A
// curator who wants the platform's own wording for a category renames the row afterwards
// (cmd/add-board --rename); nothing about the crawl depends on it.
type professionProber struct{}

func (professionProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return sources.ProfessionCategories(ctx, c)
}

func (professionProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	open, err := sources.ProfessionCategorySize(ctx, c, board)
	if err != nil {
		return "", 0, err
	}
	if open == 0 {
		return "", 0, nil
	}
	return fmt.Sprintf("Profession.hu — %s", board), open, nil
}
