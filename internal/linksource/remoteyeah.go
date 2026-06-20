package linksource

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// remoteYeah resolves RemoteYeah vacancies. Posts link directly to
// remoteyeah.com/jobs/<slug>; the slug is the stable id.
type remoteYeah struct {
	http Client
}

// NewRemoteYeah builds the RemoteYeah link-source adapter.
func NewRemoteYeah(c Client) Source { return remoteYeah{http: c} }

func (remoteYeah) Source() string { return "remoteyeah" }

// Match handles remoteyeah.com/jobs/<slug> links only — the homepage and other paths are
// not vacancies.
func (remoteYeah) Match(u *url.URL) bool {
	return host(u) == "remoteyeah.com" && strings.HasPrefix(u.Path, "/jobs/") &&
		strings.TrimPrefix(u.Path, "/jobs/") != ""
}

// remoteYeahPosting selects the JobPosting ld+json fields RemoteYeah publishes.
type remoteYeahPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary monetaryAmount `json:"baseSalary"`
}

// Resolve fetches the job page and parses its JobPosting ld+json. The slug from the link
// path is the id; the page carries no native identifier.
func (r remoteYeah) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	slug := strings.Trim(strings.TrimPrefix(u.Path, "/jobs/"), "/")
	if slug == "" {
		return sources.Job{}, false, nil
	}

	node, final, err := r.http.GetHTMLResolved(ctx, raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	// The link came from untrusted post content and the client follows redirects to
	// any host; only parse the page when it stayed on RemoteYeah, so a hijacked link
	// or open redirect cannot make us ingest an arbitrary (e.g. internal) host's HTML.
	if fu, err := url.Parse(final); err != nil || host(fu) != "remoteyeah.com" {
		return sources.Job{}, false, fmt.Errorf("linksource: remoteyeah link resolved to unexpected host %q", final)
	}
	var p remoteYeahPosting
	if !sources.LDJobPosting(node, &p) {
		return sources.Job{}, false, fmt.Errorf("linksource: remoteyeah job %s has no JobPosting ld+json", slug)
	}

	desc := sources.SanitizeHTML(p.Description)
	if salary := salaryParagraph(p.BaseSalary); salary != "" {
		// Sanitize the salary fragment too: its currency is third-party JSON-LD text and
		// the description is rendered with {@html}, so an unsanitized prefix is stored XSS.
		desc = sources.SanitizeHTML(salary) + desc
	}
	return sources.Job{
		ExternalID:  slug,
		URL:         "https://remoteyeah.com/jobs/" + slug,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Description: desc,
		Remote:      isTelecommute(p.JobLocationType),
		PostedAt:    sources.ParseRFC3339(p.DatePosted),
	}, true, nil
}
