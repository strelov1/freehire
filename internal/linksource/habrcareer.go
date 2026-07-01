package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// habrCareer resolves Habr Career vacancies. Channels link them through the u.habr.com
// shortener, which 301s to career.habr.com/vacancies/<id>; the canonical id and URL come
// from the resolved location, so the same vacancy linked from several channels dedups to
// one row. Habr is a boardless source, so the ingest pipeline namespaces its external_id
// with an empty board (":<id>"); the adapter produces the same key via
// sources.NamespaceExternalID so a linked vacancy dedups against the board crawl too.
type habrCareer struct {
	http Client
}

// NewHabrCareer builds the Habr Career link-source adapter.
func NewHabrCareer(c Client) Source { return habrCareer{http: c} }

func (habrCareer) Source() string { return "habr_career" }

// Match handles both the u.habr.com shortener and direct career.habr.com links.
func (habrCareer) Match(u *url.URL) bool {
	switch host(u) {
	case "u.habr.com", "career.habr.com":
		return true
	default:
		return false
	}
}

// habrVacancyPath matches the canonical vacancy path, capturing the numeric id.
var habrVacancyPath = regexp.MustCompile(`^/vacancies/(\d+)/?$`)

// Resolve follows the link to its career.habr.com landing, and when that is a single
// vacancy parses its JobPosting ld+json into a job. A matched link that lands somewhere
// other than /vacancies/<id> (e.g. the "Больше вакансий" index) is skipped (ok=false).
func (h habrCareer) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	node, final, err := h.http.GetHTMLResolved(ctx, raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	u, err := url.Parse(final)
	if err != nil {
		return sources.Job{}, false, err
	}
	// The link came from untrusted post content and the client follows redirects to any
	// host; only parse the page when the shortener landed on Habr itself, so a hijacked
	// short code cannot make us ingest an arbitrary (e.g. internal) host's HTML.
	if host(u) != "career.habr.com" {
		return sources.Job{}, false, fmt.Errorf("linksource: habr link resolved to unexpected host %q", u.Host)
	}
	m := habrVacancyPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil // landed on Habr but not a single vacancy — skip
	}
	id := m[1]

	// The detail-page JobPosting parse is shared with the habr_career board adapter so the two
	// read a Habr vacancy identically.
	p, ok := sources.ParseHabrPosting(node)
	if !ok {
		return sources.Job{}, false, fmt.Errorf("linksource: habr vacancy %s has no JobPosting ld+json", id)
	}
	if p.Identifier != "" {
		id = p.Identifier
	}

	return sources.Job{
		ExternalID:  sources.NamespaceExternalID("", id),
		URL:         "https://career.habr.com/vacancies/" + id,
		Title:       p.Title,
		Company:     p.Company,
		Location:    p.Location,
		Description: sources.SanitizeHTML(p.Description),
		Remote:      sources.IsRemote(p.Location),
		// Habr's ld+json datePosted is bogus (~a month ahead of the real date, with a later
		// validThrough), which would pin every Habr job to the top of the freshest-first
		// browse. The trustworthy publish timestamp is the visible <time class="basic-date"
		// datetime> element, so read that; a missing element leaves posted_at unset.
		PostedAt: sources.ParseRFC3339(sources.ElementAttr(node, "time", "basic-date", "datetime")),
	}, true, nil
}

// host returns u's lowercased hostname with a leading "www." stripped.
func host(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
