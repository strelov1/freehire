package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// lever resolves Lever-hosted vacancies. Lever is multi-tenant, so a TG link points at an
// arbitrary company's board — many of which sources/lever.yml does not list. The adapter
// writes the SAME identity the ingest pipeline would (source="lever", external_id=<posting
// id>), so UpsertJob's ON CONFLICT dedups against an already-crawled company and a
// not-yet-crawled one is added under the canonical key rather than a thin telegram dup.
type lever struct {
	http Client
}

// NewLever builds the Lever link-source adapter.
func NewLever(c Client) Source { return lever{http: c} }

func (lever) Source() string { return "lever" }

// leverJobPath captures the company and posting UUID from a job link
// (jobs.lever.co/<company>/<uuid>, optionally trailing /apply).
var leverJobPath = regexp.MustCompile(`^/([^/]+)/([0-9a-fA-F-]{36})(?:/apply)?/?$`)

// Match handles jobs.lever.co posting links only — a board listing (no posting id) is left
// for the ingest adapter.
func (lever) Match(u *url.URL) bool {
	return host(u) == "jobs.lever.co" && leverJobPath.MatchString(u.Path)
}

// Resolve reads the public per-posting API for the linked company+id and maps it exactly as
// the ingest lever adapter does. The posting id is globally unique, so external_id is the
// bare id (no board prefix), matching the ingest key.
func (l lever) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := leverJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	company, id := m[1], m[2]

	api := fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s?mode=json", company, id)
	var p struct {
		ID          string `json:"id"`
		Text        string `json:"text"`
		HostedURL   string `json:"hostedUrl"`
		CreatedAt   int64  `json:"createdAt"`
		Description string `json:"description"`
		Additional  string `json:"additional"`
		Lists       []struct {
			Text    string `json:"text"`
			Content string `json:"content"`
		} `json:"lists"`
		Categories struct {
			Location string `json:"location"`
		} `json:"categories"`
	}
	if err := l.http.GetJSON(ctx, api, &p); err != nil {
		return sources.Job{}, false, err
	}
	if p.ID == "" {
		return sources.Job{}, false, nil // not a live posting (closed/removed) — skip
	}

	// Lever splits the body across description + lists (each a heading and its HTML items) +
	// additional; assemble them into one document, mirroring the ingest adapter.
	var body strings.Builder
	body.WriteString(p.Description)
	for _, list := range p.Lists {
		if list.Text != "" {
			body.WriteString("<h3>")
			body.WriteString(list.Text)
			body.WriteString("</h3>")
		}
		body.WriteString(list.Content)
	}
	body.WriteString(p.Additional)

	return sources.Job{
		ExternalID:  p.ID,
		URL:         p.HostedURL,
		Title:       p.Text,
		Company:     humanizeBoard(company), // the per-posting API carries no company name
		Location:    p.Categories.Location,
		Description: sources.SanitizeHTML(body.String()),
		Remote:      sources.IsRemote(p.Categories.Location),
		PostedAt:    parseEpochMillis(p.CreatedAt),
	}, true, nil
}
