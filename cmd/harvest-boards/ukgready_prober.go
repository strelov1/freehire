package main

import (
	"context"
	"fmt"
	"strings"
)

// ukgreadyProber validates a UKG Ready board ("<host>/<tenant>") by asking the tenant's public
// job-requisition listing how many postings it has. It mirrors the URL shape of
// internal/ingest/sources/ukgready.go, which is unexported (this tool lives outside that
// package), and costs one request per candidate — the adapter fallback would instead run a
// whole crawl, and UKG Ready detail bodies run to ~100 KB each.
//
// It reports no employer name, so the seed's own supplies it. The platform does publish one
// (job-search/config's comp_name), but only about half the tenants fill it in and, where they
// do, it is as often the registered legal entity as the brand the postings are filed under
// ("Open Minds" is served by "Multicap Limited"), so wiring it in would reject live boards on
// the name gate and rename the rest to their all-caps legal form.
type ukgreadyProber struct{}

// dedupKey folds a board to its tenant, lower-cased: the tenant is what a board addresses, and
// the several white-label hosts fronting one UKG environment all serve the same tenants, so the
// host is branding rather than identity (see sources.boardIdentity's ukgready entry, which folds
// the same way at load time). Without this a candidate spelled with a different host than the
// board file already holds probes live, passes the gate against its own tenant, and is appended
// a second time — which a candidate for `aus-secure.prd.mykronos.com/6183838` did, against a
// board already held as `secure.workforceready.com.au/6183838`.
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
	// _paging.total counts the tenant's whole live board and does not vary with the page size
	// (verified live against size=1 and size=200), so the first page of one row settles liveness.
	// "offset" is a 1-based page number rather than a row offset — see the adapter's list — which
	// is why this asks for 1 and not 0.
	// A tenant with no career portal answers 410 and the client surfaces that as an error; for
	// harvest that is simply "not a live board" — skip silently, do not propagate.
	url := fmt.Sprintf("https://%s/ta/rest/ui/recruitment/companies/%%7C%s/job-requisitions?offset=1&size=1", host, tenant)
	if err := c.GetJSON(ctx, url, &page); err != nil {
		return "", 0, nil
	}
	return "", page.Paging.Total, nil
}
