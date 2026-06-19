package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// epam adapts EPAM's careers site (careers.epam.com). EPAM's job-search API
// (www.epam.com/api/jobs/search) is behind a Cloudflare challenge, but the gzip sitemap
// enumerates every vacancy and each vacancy page server-renders a schema.org JobPosting
// ld+json block (not Cloudflare-gated), so this adapter is the SuccessFactors shape —
// sitemap to enumerate, per-job detail fetch for the posting — over the shared ld+json
// helper. The board is the career-site host.
type epam struct {
	http epamHTTP
}

// epamHTTP is the transport epam needs: the gzip XML sitemap (the Go transport
// transparently decodes the Content-Encoding: gzip response) plus HTML detail pages.
type epamHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewEPAM builds the EPAM adapter over the given HTTP client.
func NewEPAM(c epamHTTP) Source { return epam{http: c} }

func (epam) Provider() string { return "epam" }

func (e epam) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	sitemapURL := fmt.Sprintf("https://%s/sitemap.xml.gz", ce.Board)
	if err := e.http.GetXML(ctx, sitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("epam: sitemap %s: %w", ce.Board, err)
	}

	// Keep only English vacancy pages (epamJobID is empty for the listing, language roots,
	// and the /uk//de/… localisations, so each vacancy is ingested once under its English url).
	var urls []string
	for _, u := range sitemap.URLs {
		if epamJobID(u.Loc) != "" {
			urls = append(urls, u.Loc)
		}
	}

	// Each job's posting comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return e.detail(ctx, ce, u)
	}), nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job, returning
// ok=false when the URL has no parseable id, the fetch fails, or the page carries no
// JobPosting, so the caller skips just that posting.
func (e epam) detail(ctx context.Context, ce CompanyEntry, jobURL string) (Job, bool) {
	id := epamJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := e.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p epamPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.location()
	// jobLocationType is EPAM's only structured work-arrangement signal: TELECOMMUTE means
	// remote. Absent → leave WorkMode empty and fall back to the location text for remote.
	remote := p.JobLocationType == "TELECOMMUTE"
	workMode := ""
	if remote {
		workMode = "remote"
	}
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     ce.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      remote || isRemote(location),
		WorkMode:    workMode,
		PostedAt:    parseDate(p.DatePosted),
	}, true
}

// epamJobIDPattern captures the Contentstack vacancy uid (e.g. "blt01b3u51rnautbmxq") from
// an /en/vacancy/<slug>-<uid>_en URL. Restricting to /en/vacancy/ and the _en suffix both
// filters the sitemap to English vacancies and yields the dedup id in one match.
var epamJobIDPattern = regexp.MustCompile(`/en/vacancy/[^/?#]*-(blt[a-z0-9]+)_en(?:[/?#]|$)`)

// epamJobID extracts the vacancy uid from an English vacancy URL, returning "" for the
// listing, language roots, and non-English vacancy localisations.
func epamJobID(u string) string {
	if m := epamJobIDPattern.FindStringSubmatch(u); m != nil {
		return m[1]
	}
	return ""
}

// epamPosting is the schema.org JobPosting decoded from an EPAM vacancy page's ld+json
// block. EPAM emits no jobLocation; the location is built from applicantLocationRequirements
// (an array of Country) and the work arrangement from jobLocationType.
type epamPosting struct {
	Title                         string        `json:"title"`
	Description                   string        `json:"description"`
	DatePosted                    string        `json:"datePosted"`
	JobLocationType               string        `json:"jobLocationType"`
	ApplicantLocationRequirements []epamCountry `json:"applicantLocationRequirements"`
}

type epamCountry struct {
	Name string `json:"name"`
}

// location joins the applicant-location countries (the only location signal EPAM exposes
// in the JobPosting), so a job open to several countries lists them all.
func (p epamPosting) location() string {
	names := make([]string, 0, len(p.ApplicantLocationRequirements))
	for _, c := range p.ApplicantLocationRequirements {
		names = append(names, c.Name)
	}
	return joinNonEmpty(names...)
}
