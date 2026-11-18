package main

import (
	"context"
	"fmt"
	"regexp"
)

// hcshiringProber validates an HCS Hiring board (the <tenant> of <tenant>.hcshiring.com) by
// reading meta.totalJobs from the tenant's public listing, one request per candidate. It
// mirrors the URL shape of internal/ingest/sources/hcshiring.go, which is unexported; the
// adapter fallback would walk every ten-row page and hydrate every posting's body. It reports no
// employer name, so the seed's supplies it: the platform's /api/jobSearches companyName often
// differs from the seed's employer (a tenant is as often one community as the operator), so
// using it would reject live boards on the name gate.
type hcshiringProber struct{}

// hcsTenantPattern is one DNS label: the only board shape the adapter addresses. Anything else
// would build a URL on some other host.
var hcsTenantPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]*[a-z0-9])?$`)

func (hcshiringProber) probe(ctx context.Context, c httpClient, tenant string) (string, int, error) {
	if !hcsTenantPattern.MatchString(tenant) {
		return "", 0, nil
	}
	var page struct {
		Meta struct {
			TotalJobs int `json:"totalJobs"`
		} `json:"meta"`
	}
	// An unknown tenant answers 404 ("The account does not exist.") and the client surfaces it
	// as an error; for harvest that is simply "not a live board" — skip, do not propagate.
	if err := c.GetJSON(ctx, fmt.Sprintf("https://%s.hcshiring.com/api/jobs?page=1", tenant), &page); err != nil {
		return "", 0, nil
	}
	return "", page.Meta.TotalJobs, nil
}
