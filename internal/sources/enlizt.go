package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// enlizt adapts Enlizt career sites (<board>.enlizt.me), a Brazilian ATS. The board is the
// tenant subdomain; its root page server-renders a list of /vagas/<slug> postings, and each
// detail page carries a schema.org JobPosting ld+json with the full body and a stable UUID
// identifier. Description and structured fields come from a per-posting detail fetch,
// bounded-concurrency, like the other JSON-LD detail adapters (careerspage/icims).
type enlizt struct {
	http HTMLGetter
}

// NewEnlizt builds the Enlizt adapter over the given HTML client.
func NewEnlizt(c HTMLGetter) Source { return enlizt{http: c} }

func (enlizt) Provider() string { return "enlizt" }

func (s enlizt) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base, err := url.Parse(fmt.Sprintf("https://%s.enlizt.me/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("enlizt: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("enlizt: listing %s: %w", e.Board, err)
	}

	// Each posting's fields come from its own detail fetch, fanned out under a bounded pool.
	locs := jobLinks(base, root, enliztIsVaga)
	return fetchDetails(locs, defaultDetailWorkers, func(loc string) (Job, bool) {
		return s.detail(ctx, e, loc)
	}), nil
}

// detail fetches one posting's detail page and maps its JobPosting ld+json to a Job,
// returning ok=false when the fetch fails, the page carries no JobPosting, or no stable id
// can be derived (which would collide on the dedup key), so the caller skips just that one.
func (s enlizt) detail(ctx context.Context, e CompanyEntry, loc string) (Job, bool) {
	root, err := s.http.GetHTML(ctx, loc)
	if err != nil {
		return Job{}, false
	}
	var p enliztPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	id := firstNonEmpty(p.Identifier.Value, enliztSlug(loc))
	if id == "" {
		return Job{}, false
	}

	location := p.JobLocation.Address.Location()
	mode := ""
	if strings.EqualFold(p.JobLocationType, "TELECOMMUTE") {
		mode = "remote" // schema.org's structured remote signal
	}

	return Job{
		ExternalID:  id,
		URL:         loc,
		Title:       strings.TrimSpace(p.Title),
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      mode == "remote" || isRemote(location),
		WorkMode:    mode,
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// enliztPosting is the schema.org JobPosting decoded from an Enlizt detail page's ld+json.
// identifier is a PropertyValue whose value is the stable posting UUID; jobLocation is a
// single Place (not an array).
type enliztPosting struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	DatePosted      string `json:"datePosted"`
	JobLocationType string `json:"jobLocationType"`
	Identifier      struct {
		Value string `json:"value"`
	} `json:"identifier"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlace `json:"jobLocation"`
}

// enliztVagaPattern matches a /vagas/<slug> posting path: a leading path boundary, one
// non-empty slug segment, then end-of-string or a query/fragment. Anchoring the end (like
// careerspage's id pattern) excludes /vagas/<slug>/<action> sub-links (apply/refer) so they
// are not crawled as duplicate postings; tolerating a missing leading slash accepts a
// root-relative href. The listing predicate uses it to collect detail links only.
var enliztVagaPattern = regexp.MustCompile(`(?:^|/)vagas/[^/?#]+(?:$|[?#])`)

// enliztIsVaga reports whether a listing href points at a /vagas/<slug> posting.
func enliztIsVaga(href string) bool { return enliztVagaPattern.MatchString(href) }

// enliztSlug extracts the <slug> from a /vagas/<slug> URL, the fallback dedup id when the
// JobPosting carries no identifier. It strips the /vagas/ prefix and any trailing query/
// fragment the end-anchored pattern may have captured.
func enliztSlug(loc string) string {
	m := enliztVagaPattern.FindString(loc)
	m = strings.TrimPrefix(strings.TrimPrefix(m, "/"), "vagas/")
	return strings.TrimRight(m, "?#")
}
