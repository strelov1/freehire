package linksource

import (
	"context"
	"net/url"

	"github.com/strelov1/freehire/internal/sources"
)

// generic resolves any server-rendered job page that carries a top-level schema.org
// JobPosting ld+json block. Unlike the per-ATS adapters it is NOT host-scoped — it
// matches any http(s) URL and relies on the presence of a JobPosting block to decide
// whether the page is a single vacancy (ok=false when absent). It is therefore the
// last-resort resolver: callers put it AFTER the specific adapters so a known ATS is
// still handled by its richer API-based adapter, and generic only catches the rest
// (e.g. Teamtailor custom domains, Breezy private-link postings) that no board feed
// enumerates.
//
// It is deliberately kept OUT of the shared All() registry: its always-true Match would
// make the Telegram crawl treat every outbound link as a vacancy. Only the operator-run
// cmd/resolve-url appends it, where the URL is a deliberate, trusted input.
type generic struct {
	http Client
}

// NewGeneric builds the last-resort JobPosting-ld+json link resolver.
func NewGeneric(c Client) Source { return generic{http: c} }

// Source is a fixed provenance tag for hand-imported links. It is not in sources.All, so
// these jobs are treated as orphans by the liveness worker (URL-probed and closed when
// dead) — the right lifecycle for a one-off posting that has no board to re-crawl it.
func (generic) Source() string { return "weblink" }

// Match accepts any absolute http(s) URL. The real gate is Resolve finding a JobPosting
// block, so a non-vacancy page returns ok=false rather than a bogus job.
func (generic) Match(u *url.URL) bool {
	return u != nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

// genericPosting selects only string-typed JobPosting fields. Declaring nothing whose
// schema.org shape varies (jobLocation may be an object OR an array; addressCountry may
// be a string OR an object) keeps json.Unmarshal from failing on a shape mismatch and
// dropping an otherwise-valid posting — LDJobPosting requires the whole decode to succeed.
// Geography is left to enrichment, which fills countries/regions for tech jobs anyway.
type genericPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// Resolve fetches the page and maps its JobPosting ld+json. ok=false (no error) when the
// page has no JobPosting block or lacks a title/company — it is simply not a vacancy we
// can store. The canonical id is the final URL without query/fragment, so re-importing
// the same posting (or a tracking-tagged copy of it) dedups on (weblink, that URL).
func (g generic) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	node, final, err := g.http.GetHTMLResolved(ctx, raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	var p genericPosting
	if !sources.LDJobPosting(node, &p) {
		return sources.Job{}, false, nil // not a JobPosting page — skip, not an error
	}
	if p.Title == "" || p.HiringOrganization.Name == "" {
		return sources.Job{}, false, nil // missing the minimum identity to store
	}

	canonical := final
	if fu, err := url.Parse(final); err == nil {
		fu.RawQuery = ""
		fu.Fragment = ""
		canonical = fu.String()
	}
	return sources.Job{
		ExternalID:  canonical,
		URL:         canonical,
		Title:       p.Title,
		Company:     p.HiringOrganization.Name,
		Description: sources.SanitizeHTML(p.Description),
		Remote:      isTelecommute(p.JobLocationType),
		PostedAt:    sources.ParseDate(p.DatePosted),
	}, true, nil
}
