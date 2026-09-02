package main

import (
	"context"
	"net/url"
	"strings"
)

// edjoinProber validates an EDJOIN board — a job-type id — by asking the central index how many
// postings that slice holds. It costs ONE request, and it exists because the adapter fallback
// would be actively harmful here rather than merely slow: edjoin.Fetch hydrates a detail page
// for every posting in the probed slice, and EDJOIN's are ~147 KB, so probing a job type like
// "Teacher Assistant / Aide / Paraprof." (2,814 postings) would spend ~400 MB to learn a number
// the listing states for free. That is the exact cost the adapter's board-is-a-job-type design
// exists to avoid, so the tool that proposes boards must not re-introduce it.
//
// It reports no employer name, so the seed's own supplies it: one board is a slice of the whole
// platform spanning hundreds of districts, and no single employer name describes it.
type edjoinProber struct{}

func (edjoinProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	jobType := strings.TrimSpace(board)
	if jobType == "" || strings.Trim(jobType, "0123456789") != "" {
		return "", 0, nil // a board is a numeric job-type id; anything else is not one
	}
	var page struct {
		TotalRecords int `json:"totalRecords"`
	}
	// totalRecords counts the whole slice and does not vary with the page size (verified live
	// at rows=1, 25, 100, 500 and 1000 alike), so one row settles liveness. catID, districtID
	// and recruitmentCenterID must be present and numeric or the endpoint answers a 500 .NET
	// error page — see internal/ingest/sources/edjoin.go. An UNKNOWN job type is answered 200
	// with totalRecords 0, which is exactly the "not a live board" the caller wants.
	q := url.Values{
		"rows": {"1"}, "page": {"1"}, "sort": {"postingDate"}, "sortVal": {"0"},
		"order": {"desc"}, "searchType": {"all"}, "jobTypes": {jobType}, "days": {"0"},
		"catID": {"0"}, "districtID": {"0"}, "recruitmentCenterID": {"0"},
	}
	if err := c.GetJSON(ctx, "https://www.edjoin.org/Home/LoadJobs?"+q.Encode(), &page); err != nil {
		return "", 0, nil
	}
	return "", page.TotalRecords, nil
}
