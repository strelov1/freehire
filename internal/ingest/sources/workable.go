package sources

import (
	"context"
	"fmt"
)

// workableBaseURL is the Workable public job-board widget API root.
const workableBaseURL = "https://apply.workable.com/api/v1/widget/accounts"

// workable adapts the Workable public widget API. With details=true the list endpoint
// carries an inline HTML description, so no per-posting detail request is needed.
type workable struct {
	http JSONGetter
}

type workablePosting struct {
	Title         string `json:"title"`
	Shortcode     string `json:"shortcode"`
	URL           string `json:"url"`
	Description   string `json:"description"`
	PublishedOn   string `json:"published_on"`
	City          string `json:"city"`
	State         string `json:"state"`
	Country       string `json:"country"`
	Telecommuting bool   `json:"telecommuting"`
	Type          string `json:"type"`
}

// NewWorkable builds the Workable adapter over the given HTTP client.
func NewWorkable(c JSONGetter) Source { return workable{http: c} }

func (workable) Provider() string { return "workable" }

// fullBoardListing: Fetch is a single unpaginated request returning the board's whole jobs
// array — no loop that could stop early. See the fullBoardListing interface for the bar.
func (workable) fullBoardListing() {}

// emptyBoardListing: an unknown account answers 404, and an emptied one
// answers 200 with its account object and "jobs": [] (2026-10-29); an object without the array
// fails Fetch. See the emptyBoardListing interface for the bar.
func (workable) emptyBoardListing() {}

func (w workable) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	url := fmt.Sprintf("%s/%s?details=true", workableBaseURL, e.Board)

	var resp struct {
		Jobs []workablePosting `json:"jobs"`
	}
	if err := w.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("workable: fetch board %s: %w", e.Board, err)
	}
	if resp.Jobs == nil {
		return nil, errListingAbsent("workable", e.Board, "jobs")
	}

	// Workable repeats a posting once per location under the same shortcode. Sending
	// those rows to the pipeline separately races identical inserts and overwrites
	// geography. Keep one posting with every location, in first-seen order.
	byShortcode := make(map[string][]workablePosting)
	var order []string
	for _, j := range resp.Jobs {
		if j.Shortcode == "" {
			return nil, fmt.Errorf("workable: listing contains a posting without a shortcode")
		}
		if _, ok := byShortcode[j.Shortcode]; !ok {
			order = append(order, j.Shortcode)
		}
		byShortcode[j.Shortcode] = append(byShortcode[j.Shortcode], j)
	}

	jobs := make([]Job, 0, len(order))
	for _, shortcode := range order {
		rows := byShortcode[shortcode]
		j := rows[0]
		countries := make([]string, 0, len(rows))
		for _, row := range rows {
			countries = append(countries, row.Country)
		}
		jobs = append(jobs, Job{
			ExternalID: j.Shortcode,
			URL:        j.URL,
			Title:      j.Title,
			Company:    e.Company,
			Location: distinctJoin(rows, "; ", func(row workablePosting) string {
				return joinNonEmpty(row.City, row.State, row.Country)
			}),
			Countries:      countriesFromCodes(countries),
			Description:    sanitizeHTML(j.Description),
			Remote:         j.Telecommuting,
			WorkMode:       workModeFromRemote(j.Telecommuting),
			PostedAt:       parseDate(j.PublishedOn),
			EmploymentType: workableEmploymentType(j.Type),
		})
	}
	return jobs, nil
}

// CompanyDescription fetches the account's own "about us" text — a top-level
// description field the same widget endpoint Fetch calls also carries. It
// deliberately omits ?details=true: confirmed live, that flag only controls
// whether each JOB's own full body is inlined (Fetch needs it, this doesn't), and
// dropping it shrinks the response substantially (~11x on a sampled board) while
// leaving the account-level description unaffected — so this is a second request,
// but a cheap one, not a duplicate of Fetch's own (potentially much larger)
// payload. Like the job listing's own description (Fetch, above), this field is
// raw HTML, not entity-encoded, so no html.UnescapeString is needed. An employer
// who has not filled the field in yields ("", nil), not an error.
func (w workable) CompanyDescription(ctx context.Context, e CompanyEntry) (string, error) {
	url := fmt.Sprintf("%s/%s", workableBaseURL, e.Board)

	var resp struct {
		Description string `json:"description"`
	}
	if err := w.http.GetJSON(ctx, url, &resp); err != nil {
		return "", fmt.Errorf("workable: fetch account %s: %w", e.Board, err)
	}
	if resp.Description == "" {
		return "", nil
	}
	return sanitizeHTML(resp.Description), nil
}

// workableEmploymentType maps Workable's widget "type" onto the freehire vocabulary,
// returning "" for "Other"/absent so the description parser decides.
func workableEmploymentType(t string) string {
	switch t {
	case "Full-time":
		return "full_time"
	case "Part-time":
		return "part_time"
	case "Contract", "Temporary":
		return "contract"
	case "Internship":
		return "internship"
	default:
		return ""
	}
}
