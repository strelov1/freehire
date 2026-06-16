package linksource

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// habrCareer resolves Habr Career vacancies. Channels link them through the u.habr.com
// shortener, which 301s to career.habr.com/vacancies/<id>; the canonical id and URL come
// from the resolved location, so the same vacancy linked from several channels dedups to
// one row.
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

// habrPosting selects the JobPosting ld+json fields Habr publishes on a vacancy page.
// datePosted is deliberately absent: Habr's ld+json datePosted is unreliable (it runs ~a
// month ahead of the real publish date), so the posting date is read from the page's
// <time> element instead — see Resolve.
type habrPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	Identifier  struct {
		Value string `json:"value"`
	} `json:"identifier"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation []struct {
		Address habrAddress `json:"address"`
	} `json:"jobLocation"`
}

// habrAddress reads jobLocation.address, which Habr emits as a bare string but schema.org
// types as a PostalAddress object — so it accepts either shape.
type habrAddress struct{ Text string }

func (a *habrAddress) UnmarshalJSON(b []byte) error {
	var s string
	if json.Unmarshal(b, &s) == nil {
		a.Text = s
		return nil
	}
	var o struct {
		AddressLocality string `json:"addressLocality"`
		AddressCountry  string `json:"addressCountry"`
	}
	if json.Unmarshal(b, &o) == nil {
		a.Text = strings.TrimPrefix(strings.TrimSpace(o.AddressLocality+", "+o.AddressCountry), ", ")
		a.Text = strings.TrimSuffix(a.Text, ", ")
	}
	return nil
}

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

	var p habrPosting
	if !sources.LDJobPosting(node, &p) {
		return sources.Job{}, false, fmt.Errorf("linksource: habr vacancy %s has no JobPosting ld+json", id)
	}
	if p.Identifier.Value != "" {
		id = p.Identifier.Value
	}

	var loc string
	if len(p.JobLocation) > 0 {
		loc = p.JobLocation[0].Address.Text
	}
	return sources.Job{
		ExternalID:  id,
		URL:         "https://career.habr.com/vacancies/" + id,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Location:    loc,
		Description: sources.SanitizeHTML(p.Description),
		Remote:      sources.IsRemote(loc),
		// Habr's ld+json datePosted is bogus (~a month ahead of the real date, with a later
		// validThrough), which would pin every Habr job to the top of the freshest-first
		// browse. The trustworthy publish timestamp is the visible <time class="basic-date"
		// datetime> element, so read that; a missing element leaves posted_at unset.
		PostedAt: parseRFC3339(sources.ElementAttr(node, "time", "basic-date", "datetime")),
	}, true, nil
}

// host returns u's lowercased hostname with a leading "www." stripped.
func host(u *url.URL) string {
	return strings.TrimPrefix(strings.ToLower(u.Hostname()), "www.")
}
