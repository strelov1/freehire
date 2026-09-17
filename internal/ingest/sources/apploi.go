package sources

import (
	"context"
	"fmt"
)

// apploi adapts the apploi.com job board (jobs.apploi.com), a healthcare-focused ATS. The
// board is the numeric employer id, and the public API is meant to list that employer's
// postings, descriptions inline, at api.apploi.com/v1/jobs?employer=<id> (limit/offset
// paginated) — so a whole board comes from paginated list calls with no per-posting detail
// fetch.
//
// "MEANT TO" is doing real work in that sentence. Measured 2026-09-15, the API stopped
// honouring the parameter entirely: a real id, a nonsense id and no parameter at all returned
// byte-identical pages — the whole global catalogue. This adapter believed it, and each of
// 5,833 boards stored that catalogue under ITS OWN company: 1,565,701 rows standing for 3,024
// real postings, 99.81% duplicates, every posting filed under 3,898 different employers. The
// true employer was never written to any column, so nothing could be repaired afterwards and
// the whole provider had to be retired.
//
// Fetch therefore proves the filter before trusting it — see apploiFilterProbeEmployer.
type apploi struct {
	http JSONGetter
}

const (
	apploiAPI    = "https://api.apploi.com/v1/jobs"
	apploiJobURL = "https://jobs.apploi.com/view/%s"
	// apploiPageSize is the page window; apploiMaxPages caps the walk so a board that never
	// short-returns cannot loop forever (boards run to a few hundred postings at most).
	apploiPageSize = 100
	apploiMaxPages = 100
	// apploiFilterProbeEmployer is an employer id no account can hold, asked for once per
	// Fetch to make the API demonstrate that it filters at all. A working API answers it with
	// nothing. An API ignoring the parameter answers it with the same global catalogue it
	// gives every other id — which is exactly what it did in September 2026, and the reason
	// this provider's whole 1.5M-row footprint was misattributed.
	apploiFilterProbeEmployer = "0"
)

// NewApploi builds the apploi.com adapter over the given JSON client.
func NewApploi(c JSONGetter) Source { return apploi{http: c} }

func (apploi) Provider() string { return "apploi" }

// fullBoardListing: Fetch proves completeness by paginating to a page shorter than
// apploiPageSize (the offset/limit equivalent of a genuinely empty page — a full page can
// never be the API's last one), and treats a page failure or reaching apploiMaxPages as a
// hard Fetch failure. No per-posting detail fetch exists (descriptions are inline in the
// listing), so there is no unreadableDetail concern here. See the fullBoardListing interface
// (source.go) for the bar.
func (apploi) fullBoardListing() {}

// Fetch pages the employer's listing until a page shorter than apploiPageSize proves the
// index has no more rows beyond it — the offset/limit equivalent of a genuinely empty page,
// since a full-size page can never be the last one for this API. Every page failing, and
// reaching apploiMaxPages without ever seeing a short page, are hard Fetch failures rather
// than a partial success — see the fullBoardListing interface (source.go) for the bar.
func (s apploi) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	if err := s.proveEmployerFilter(ctx, e.Board); err != nil {
		return nil, err
	}

	var jobs []Job
	done := false
	for page := 0; page < apploiMaxPages; page++ {
		url := fmt.Sprintf("%s?employer=%s&limit=%d&offset=%d", apploiAPI, e.Board, apploiPageSize, page*apploiPageSize)
		var resp struct {
			Data []apploiJob `json:"data"`
		}
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("apploi: board %q page %d: %w", e.Board, page, err)
		}
		for _, j := range resp.Data {
			// The list carries archived/unpublished/private rows too; keep only the live,
			// publicly searchable postings.
			if !j.Published || j.Archived || j.Private || j.ID == "" {
				continue
			}
			jobs = append(jobs, s.toJob(e, j))
		}
		if len(resp.Data) < apploiPageSize {
			done = true
			break // last (short) page
		}
	}
	if !done {
		return nil, fmt.Errorf("apploi: board %q: reached the %d-page safety ceiling without finding the board's end", e.Board, apploiMaxPages)
	}
	return jobs, nil
}

// proveEmployerFilter refuses the board when the API is answering every employer id with the
// same list.
//
// It asks for a sentinel id no account can hold and compares the first posting it gets back
// with the first posting the real board returns. Identical means the parameter is being
// ignored and every posting this crawl would store belongs to somebody else — the failure
// that cost this provider 1,565,701 misattributed rows.
//
// Three answers are healthy and all of them proceed: the sentinel returns nothing (the API
// filtered, and that id has no postings); the sentinel returns something DIFFERENT (the API
// is filtering, whatever it found); or the board itself is empty, which says nothing about
// the filter either way.
//
// Refusing costs a cooled-down board and a retry. Not refusing costs rows no later crawl can
// repair, because the true employer is never written anywhere — that asymmetry, not a
// preference, is why this returns an error rather than skipping the suspect postings.
//
// Two extra requests per board, both limit=1. That is the price of not having to trust a
// third party's query parameter, and this provider is the reason the price is worth paying.
func (s apploi) proveEmployerFilter(ctx context.Context, board string) error {
	sentinel, err := s.firstPostingID(ctx, apploiFilterProbeEmployer)
	if err != nil {
		return fmt.Errorf("apploi: board %q: probing the employer filter: %w", board, err)
	}
	if sentinel == "" {
		return nil // the API filtered: a non-existent employer has no postings
	}
	mine, err := s.firstPostingID(ctx, board)
	if err != nil {
		return fmt.Errorf("apploi: board %q: reading its first posting: %w", board, err)
	}
	if mine == "" {
		return nil // an empty board proves nothing about the filter
	}
	if mine == sentinel {
		return fmt.Errorf("apploi: board %q: the API ignored ?employer= — it answered the "+
			"same posting %q for employer %q, so every posting here would be attributed to "+
			"the wrong company (see the 2026-09 misattribution)",
			board, mine, apploiFilterProbeEmployer)
	}
	return nil
}

// firstPostingID reads the id of the first posting an employer lists, or "" when it lists
// none. Published/archived/private are deliberately NOT filtered here: the question is what
// the API returns for this id, not what is worth ingesting.
func (s apploi) firstPostingID(ctx context.Context, employer string) (string, error) {
	url := fmt.Sprintf("%s?employer=%s&limit=1&offset=0", apploiAPI, employer)
	var resp struct {
		Data []apploiJob `json:"data"`
	}
	if err := s.http.GetJSON(ctx, url, &resp); err != nil {
		return "", err
	}
	if len(resp.Data) == 0 {
		return "", nil
	}
	return resp.Data[0].ID, nil
}

func (apploi) toJob(e CompanyEntry, j apploiJob) Job {
	location := joinNonEmpty(j.City, j.State, j.Country)
	return Job{
		ExternalID:  j.ID,
		URL:         fmt.Sprintf(apploiJobURL, j.ID),
		Title:       j.Name,
		Company:     firstNonEmpty(e.Company, j.BrandName),
		Location:    location,
		Description: sanitizeHTML(j.Description),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(j.PublishedDate),
	}
}

// apploiJob is one posting in the api.apploi.com/v1/jobs list. Descriptions are inline, so
// no detail fetch is needed.
type apploiJob struct {
	ID            string `json:"id"`
	Name          string `json:"name"`
	Description   string `json:"description"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	PublishedDate string `json:"published_date"`
	BrandName     string `json:"brand_name_with_company_only"`
	Published     bool   `json:"published"`
	Archived      bool   `json:"archived"`
	Private       bool   `json:"private"`
}
