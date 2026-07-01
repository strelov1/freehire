package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// workable resolves Workable-hosted vacancies. Workable is multi-tenant, so a TG link points
// at an arbitrary account's board. The adapter writes the SAME identity the ingest pipeline
// would (source="workable", external_id "<account>:<shortcode>" via sources.NamespaceExternalID,
// the URL's account being the board), so UpsertJob's ON CONFLICT dedups against an
// already-crawled account rather than writing a thin telegram duplicate.
type workable struct {
	http Client
}

// NewWorkable builds the Workable link-source adapter.
func NewWorkable(c Client) Source { return workable{http: c} }

func (workable) Source() string { return "workable" }

// workableJobPath captures the account and posting shortcode from a job link
// (apply.workable.com/<account>/j/<shortcode>, optionally trailing /apply).
var workableJobPath = regexp.MustCompile(`^/([^/]+)/j/([0-9A-Za-z]+)(?:/apply)?/?$`)

// Match handles apply.workable.com posting links only — an account's board (no /j/<code>) is
// left for the ingest adapter.
func (workable) Match(u *url.URL) bool {
	return host(u) == "apply.workable.com" && workableJobPath.MatchString(u.Path)
}

// Resolve reads the account's public widget board (the only public endpoint; it carries
// every job inline with details=true) and returns the job whose shortcode the link names,
// mapped exactly as the ingest workable adapter does. A shortcode no longer on the board
// (closed/removed) yields ok=false.
func (w workable) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := workableJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	account, shortcode := m[1], m[2]

	api := fmt.Sprintf("https://apply.workable.com/api/v1/widget/accounts/%s?details=true", account)
	var resp struct {
		Jobs []struct {
			Title         string `json:"title"`
			Shortcode     string `json:"shortcode"`
			URL           string `json:"url"`
			Description   string `json:"description"`
			PublishedOn   string `json:"published_on"`
			City          string `json:"city"`
			State         string `json:"state"`
			Country       string `json:"country"`
			Telecommuting bool   `json:"telecommuting"`
		} `json:"jobs"`
	}
	if err := w.http.GetJSON(ctx, api, &resp); err != nil {
		return sources.Job{}, false, err
	}

	for _, j := range resp.Jobs {
		if j.Shortcode != shortcode {
			continue
		}
		return sources.Job{
			ExternalID:  sources.NamespaceExternalID(account, j.Shortcode),
			URL:         j.URL,
			Title:       j.Title,
			Company:     humanizeBoard(account), // the widget API carries no company name
			Location:    joinNonEmpty(j.City, j.State, j.Country),
			Description: sources.SanitizeHTML(j.Description),
			Remote:      j.Telecommuting,
			PostedAt:    sources.ParseDate(j.PublishedOn),
		}, true, nil
	}
	return sources.Job{}, false, nil // shortcode not on the board — closed/removed
}

// joinNonEmpty joins the non-blank parts with ", " — Workable splits location across
// city/state/country, any of which may be empty.
func joinNonEmpty(parts ...string) string {
	var kept []string
	for _, p := range parts {
		if strings.TrimSpace(p) != "" {
			kept = append(kept, p)
		}
	}
	return strings.Join(kept, ", ")
}
