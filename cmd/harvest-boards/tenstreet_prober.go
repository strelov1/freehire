package main

import (
	"context"
	"errors"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// tenstreetProber validates a Tenstreet company id with the one request the crawl starts from: the
// carrier's Pulse profile, which names the carrier and links every open posting on one unpaginated
// page. Running the adapter would fetch every posting page (357 postings is 358 requests). The list
// is complete, so it can also answer the expected-id check. Board format: internal/ingest/sources/tenstreet.go.
type tenstreetProber struct{}

func (tenstreetProber) probe(ctx context.Context, c httpClient, companyID string) (string, int, error) {
	l, err := sources.ReadTenstreetListing(ctx, c, companyID)
	if errors.Is(err, sources.ErrBoardGone) {
		return "", 0, nil // an unknown company id: not a live board
	}
	if err != nil {
		return "", 0, err
	}
	return l.Company, len(l.PostingIDs), nil
}

func (tenstreetProber) postingIDs(ctx context.Context, c httpClient, companyID string) ([]string, error) {
	l, err := sources.ReadTenstreetListing(ctx, c, companyID)
	if err != nil {
		return nil, err
	}
	return l.PostingIDs, nil
}
