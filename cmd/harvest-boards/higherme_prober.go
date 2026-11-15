package main

import (
	"context"
	"net/url"
)

// highermeProber probes a HigherMe brand (board = the 13-hex brand id from
// app.higherme.com/careers/<brand>) with one request: the brand-filtered listing at page size 1,
// whose meta.total is the brand's posting count. An unknown brand answers an empty listing, so a
// zero total is simply not live, and every error is returned, never read as absence. The API's
// firewall blocks an IP that probes faster than ~0.2 req/s; swallowing that as "0 postings"
// reported live brands as dead.
// The brand resource carries a name, but a brand is the CONCEPT ("La Quinta Inn & Suites")
// while a seed names the operator that runs it, and the two mostly disagree. The prober
// therefore reports no name and the seed's company labels the board, as hiringthingProber does.
type highermeProber struct{}

// highermeProbeURL is the brand's listing at page size 1, encoded the way the ingest adapter
// encodes its own listing request.
func highermeProbeURL(brand string) string {
	q := url.Values{
		"page":              {"1"},
		"limit":             {"1"},
		"filters[brand.id]": {brand},
	}
	return "https://api.higherme.com/classic/jobs?" + q.Encode()
}

func (highermeProber) probe(ctx context.Context, c httpClient, brand string) (string, int, error) {
	var resp struct {
		Meta struct {
			Total int `json:"total"`
		} `json:"meta"`
	}
	if err := c.GetJSON(ctx, highermeProbeURL(brand), &resp); err != nil {
		return "", 0, err
	}
	return "", resp.Meta.Total, nil
}
