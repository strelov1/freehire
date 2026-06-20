package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/strelov1/freehire/internal/sources"
)

// geekjob resolves Geekjob.ru vacancies. Posts link directly to
// geekjob.ru/vacancy/<id>; the id from the URL is the stable canonical key (the JSON-LD
// also carries a separate internal numeric id, which we ignore so the stored id matches
// the link and dedups across channels).
type geekjob struct {
	http Client
}

// NewGeekjob builds the Geekjob link-source adapter.
func NewGeekjob(c Client) Source { return geekjob{http: c} }

func (geekjob) Source() string { return "geekjob" }

// geekjobVacancyPath matches the canonical vacancy path, capturing the id.
var geekjobVacancyPath = regexp.MustCompile(`^/vacancy/([0-9a-zA-Z]+)/?$`)

// Match handles geekjob.ru/vacancy/<id> links only.
func (geekjob) Match(u *url.URL) bool {
	return host(u) == "geekjob.ru" && geekjobVacancyPath.MatchString(u.Path)
}

// geekjobPosting selects the JobPosting ld+json fields Geekjob publishes.
type geekjobPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary monetaryAmount `json:"baseSalary"`
}

// Resolve fetches the vacancy page and parses its JobPosting ld+json. The id comes from the
// URL path (the page's internal identifier differs and is ignored).
func (g geekjob) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := geekjobVacancyPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	id := m[1]

	node, final, err := g.http.GetHTMLResolved(ctx, raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	// The link came from untrusted post content and the client follows redirects to
	// any host; only parse the page when it stayed on Geekjob, so a hijacked link or
	// open redirect cannot make us ingest an arbitrary (e.g. internal) host's HTML.
	if fu, err := url.Parse(final); err != nil || host(fu) != "geekjob.ru" {
		return sources.Job{}, false, fmt.Errorf("linksource: geekjob link resolved to unexpected host %q", final)
	}
	var p geekjobPosting
	if !sources.LDJobPosting(node, &p) {
		return sources.Job{}, false, fmt.Errorf("linksource: geekjob vacancy %s has no JobPosting ld+json", id)
	}

	desc := sources.SanitizeHTML(p.Description)
	if salary := salaryParagraph(p.BaseSalary); salary != "" {
		desc = sources.SanitizeHTML(salary) + desc
	}
	return sources.Job{
		ExternalID:  id,
		URL:         "https://geekjob.ru/vacancy/" + id,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Description: desc,
		Remote:      isTelecommute(p.JobLocationType),
		PostedAt:    sources.ParseDate(p.DatePosted),
	}, true, nil
}
