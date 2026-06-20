package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/strelov1/freehire/internal/sources"
)

// ashby resolves Ashby-hosted vacancies. Like Greenhouse it is multi-tenant, so it writes
// the SAME identity the ingest pipeline does (source="ashby", external_id="<board>:<id>")
// to dedup against an already-crawled board and add an unlisted one under the canonical key.
// Ashby's public posting API is per-board (no per-job endpoint and no company name), so the
// adapter fetches the board and finds the linked job; the company is derived from the slug.
type ashby struct {
	http Client
}

// NewAshby builds the Ashby link-source adapter.
func NewAshby(c Client) Source { return ashby{http: c} }

func (ashby) Source() string { return "ashby" }

// ashbyJobPath captures the board and the job's UUID from a job link path
// (jobs.ashbyhq.com/<board>/<uuid>).
var ashbyJobPath = regexp.MustCompile(`^/([^/]+)/([0-9a-fA-F-]{36})/?$`)

// Match handles jobs.ashbyhq.com/<board>/<uuid> links only.
func (ashby) Match(u *url.URL) bool {
	return host(u) == "jobs.ashbyhq.com" && ashbyJobPath.MatchString(u.Path)
}

// Resolve reads the board's public posting API and maps the linked job, mirroring the
// ingest ashby adapter and namespacing the external id by board to match.
func (a ashby) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := ashbyJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	board, id := m[1], m[2]

	var resp struct {
		Jobs []struct {
			ID              string `json:"id"`
			Title           string `json:"title"`
			Location        string `json:"location"`
			JobURL          string `json:"jobUrl"`
			PublishedAt     string `json:"publishedAt"`
			DescriptionHTML string `json:"descriptionHtml"`
			IsRemote        bool   `json:"isRemote"`
		} `json:"jobs"`
	}
	api := fmt.Sprintf("https://api.ashbyhq.com/posting-api/job-board/%s", board)
	if err := a.http.GetJSON(ctx, api, &resp); err != nil {
		return sources.Job{}, false, err
	}

	for _, j := range resp.Jobs {
		if j.ID != id {
			continue
		}
		jobURL := j.JobURL
		if jobURL == "" {
			jobURL = "https://jobs.ashbyhq.com/" + board + "/" + id
		}
		return sources.Job{
			ExternalID:  board + ":" + id,
			URL:         jobURL,
			Title:       j.Title,
			Company:     humanizeBoard(board),
			Location:    j.Location,
			Description: sources.SanitizeHTML(j.DescriptionHTML),
			Remote:      j.IsRemote || sources.IsRemote(j.Location),
			PostedAt:    sources.ParseRFC3339(j.PublishedAt),
		}, true, nil
	}
	return sources.Job{}, false, nil // not on the board anymore (delisted) — skip
}
