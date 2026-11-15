package main

import (
	"context"
	"fmt"
	"strings"
)

// ukgreadyProber validates a UKG Ready board ("<host>/<tenant>") by asking the tenant's public
// job-requisition listing how many postings it has. It mirrors the URL shape of the unexported
// internal/ingest/sources/ukgready.go and costs one request per candidate; the adapter fallback would
// crawl everything (detail bodies run ~100 KB).
//
// It reports no employer name, so the seed supplies it. job-search/config's comp_name is filled in by
// only about half the tenants and is often the legal entity, not the brand ("Open Minds" is served
// by "Multicap Limited"), so using it would reject live boards on the name gate.
type ukgreadyProber struct{}

// dedupKey folds a board to its lower-cased tenant: the several white-label hosts fronting one UKG
// environment serve the same tenants, so the host is branding, not identity (same fold as
// sources.boardIdentity's ukgready entry). Without it a candidate on a different host than the one
// already held probes live and is appended a second time.
func (ukgreadyProber) dedupKey(board string) string {
	if _, tenant, ok := strings.Cut(board, "/"); ok {
		return strings.ToLower(tenant)
	}
	return strings.ToLower(board)
}

func (ukgreadyProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	host, tenant, ok := strings.Cut(board, "/")
	if !ok || host == "" || tenant == "" || strings.Contains(tenant, "/") {
		return "", 0, nil
	}
	var page struct {
		Paging struct {
			Total int `json:"total"`
		} `json:"_paging"`
	}
	// _paging.total counts the tenant's whole live board regardless of page size, so one row settles
	// liveness. "offset" is a 1-based page number, hence 1 and not 0.
	// A tenant with no career portal answers 410; harvest skips it silently.
	url := fmt.Sprintf("https://%s/ta/rest/ui/recruitment/companies/%%7C%s/job-requisitions?offset=1&size=1", host, tenant)
	if err := c.GetJSON(ctx, url, &page); err != nil {
		return "", 0, nil
	}
	return "", page.Paging.Total, nil
}
