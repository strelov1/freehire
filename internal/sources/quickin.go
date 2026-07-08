package sources

import (
	"context"
	"fmt"
	"strings"
)

const (
	quickinBaseURL  = "https://api.quickin.io"
	quickinPageSize = 100
	// quickinMaxPages bounds the page walk; the stop signal is the response's pages
	// count, but this caps a misbehaving API well above any single tenant's postings.
	quickinMaxPages = 200
	// quickinPublished is the publicate value marking a live posting; any other value
	// (draft/paused/closed) is skipped.
	quickinPublished = "published"
)

// quickin adapts Quickin's public careers API (api.quickin.io), a Brazilian ATS whose
// company career sites live at jobs.quickin.io/<slug>. A board is that account slug; it is
// first resolved to the account's opaque id, which keys the paginated jobs listing. The
// listing carries the full posting inline (title, description, requirements, location,
// workplace type), so no per-posting detail request is needed.
type quickin struct {
	http JSONGetter
}

// NewQuickin builds the Quickin adapter over the given HTTP client.
func NewQuickin(c JSONGetter) Source { return quickin{http: c} }

func (quickin) Provider() string { return "quickin" }

// quickinJob is one posting from the listing. publicate gates visibility; description and
// requirements are separate HTML blocks; city/region/country are free-text location parts
// (country is an ISO alpha-2 code); workplace_type is a structured remote/hybrid/onsite enum.
type quickinJob struct {
	ID            string `json:"_id"`
	Publicate     string `json:"publicate"`
	Title         string `json:"title"`
	Description   string `json:"description"`
	Requirements  string `json:"requirements"`
	City          string `json:"city"`
	Region        string `json:"region"`
	Country       string `json:"country"`
	WorkplaceType string `json:"workplace_type"`
	CareerURL     string `json:"career_url"`
	CreatedAt     string `json:"created_at"`
}

func (q quickin) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	accountID, err := q.accountID(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	postings, err := q.list(ctx, e.Board, accountID)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		if p.Publicate != quickinPublished {
			continue
		}
		mode := workplaceTypeMode(p.WorkplaceType)
		jobs = append(jobs, Job{
			ExternalID:  p.ID,
			URL:         firstNonEmpty(p.CareerURL, fmt.Sprintf("https://jobs.quickin.io/%s/jobs/%s", e.Board, p.ID)),
			Title:       strings.TrimSpace(p.Title),
			Company:     e.Company,
			Location:    joinNonEmpty(p.City, p.Region, p.Country),
			Description: quickinBody(p.Description, p.Requirements),
			Remote:      mode == "remote",
			WorkMode:    mode,
			PostedAt:    parseRFC3339(p.CreatedAt),
		})
	}
	return jobs, nil
}

// accountID resolves a board (account slug) to the opaque account id that keys the jobs
// listing.
func (q quickin) accountID(ctx context.Context, board string) (string, error) {
	var acc struct {
		ID string `json:"_id"`
	}
	url := fmt.Sprintf("%s/public/accounts/%s", quickinBaseURL, board)
	if err := q.http.GetJSON(ctx, url, &acc); err != nil {
		return "", fmt.Errorf("quickin: account %s: %w", board, err)
	}
	if acc.ID == "" {
		return "", fmt.Errorf("quickin: account %s: empty id", board)
	}
	return acc.ID, nil
}

// list pages through an account's postings, stopping at the response's pages count. A
// first-page failure aborts (the board yields nothing); a later-page failure after at least
// one page succeeded stops the walk and keeps what was gathered.
func (q quickin) list(ctx context.Context, board, accountID string) ([]quickinJob, error) {
	var postings []quickinJob
	for page := 1; page <= quickinMaxPages; page++ {
		url := fmt.Sprintf("%s/public/%s/jobs?page=%d&limit=%d", quickinBaseURL, accountID, page, quickinPageSize)
		var resp struct {
			Docs  []quickinJob `json:"docs"`
			Pages int          `json:"pages"`
		}
		if err := q.http.GetJSON(ctx, url, &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("quickin: list %s: %w", board, err)
			}
			break // partial: keep what earlier pages yielded
		}
		postings = append(postings, resp.Docs...)
		// Stop at the server's reported last page when it gives one; otherwise (pages
		// absent or 0) fall back to a short page, so a missing pages field can't
		// truncate a full board to its first page.
		if resp.Pages > 0 {
			if page >= resp.Pages {
				break
			}
		} else if len(resp.Docs) < quickinPageSize {
			break
		}
	}
	return postings, nil
}

// quickinBody sanitizes and joins the posting's two HTML blocks (description and
// requirements) into one plain-text body, dropping whichever is empty.
func quickinBody(description, requirements string) string {
	return strings.TrimSpace(sanitizeHTML(description) + "\n\n" + sanitizeHTML(requirements))
}
