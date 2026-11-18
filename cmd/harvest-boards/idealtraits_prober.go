package main

import (
	"context"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// idealtraitsProber validates an IdealTraits board (the employer's numeric business id) with the
// one request the adapter's crawl starts with: the career site's listing XHR, whose jobcount is
// the board's total (sources.IdealTraitsJobCount). An unknown id answers 403, read as a skip.
// It reports no company name: the XHR carries none, and the career site's slug is a URL form of
// the name. The seed's company labels the board. The host refuses an IP for ~4 minutes after ~120
// quick requests (see sources' idealtraitsRequestInterval), so a large seed wants -pace 1.5.
type idealtraitsProber struct{}

func (idealtraitsProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	n, err := sources.IdealTraitsJobCount(ctx, c, board)
	if err != nil {
		return "", 0, nil
	}
	return "", n, nil
}
