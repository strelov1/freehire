package main

import (
	"context"
	"net/url"
	"strings"
)

// edjoinProber validates an EDJOIN board (a job-type id) by asking the central index how many
// postings that slice holds, in ONE request. The adapter fallback would hydrate a ~147 KB detail
// page per posting in the slice (thousands for a large job type) to learn a number the listing
// states for free. It reports no employer name, so the seed's supplies it: one board is a slice
// spanning hundreds of districts.
type edjoinProber struct{}

func (edjoinProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	jobType := strings.TrimSpace(board)
	if jobType == "" || strings.Trim(jobType, "0123456789") != "" {
		return "", 0, nil // a board is a numeric job-type id; anything else is not one
	}
	var page struct {
		TotalRecords int `json:"totalRecords"`
	}
	// totalRecords counts the whole slice regardless of page size, so one row settles liveness.
	// catID, districtID and recruitmentCenterID must be present and numeric or the endpoint answers
	// a 500 .NET error page (see internal/ingest/sources/edjoin.go). An UNKNOWN job type answers 200
	// with totalRecords 0, which is the "not a live board" the caller wants.
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
