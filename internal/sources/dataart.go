package sources

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// dataart adapts DataArt's careers site (www.dataart.team), a single-company source with no
// per-tenant board id (boardless). DataArt runs a custom careers SPA fronting no supported ATS,
// but its sitemap enumerates every vacancy and each vacancy page server-renders a schema.org
// JobPosting ld+json block, so this adapter is the EPAM/SuccessFactors shape — sitemap to
// enumerate, per-vacancy detail fetch for the posting — over the shared ld+json helper.
type dataart struct {
	http dataartHTTP
}

// dataartHTTP is the transport dataart needs: the XML sitemap plus HTML detail pages.
type dataartHTTP interface {
	XMLGetter
	HTMLGetter
}

const dataartSitemapURL = "https://www.dataart.team/sitemap.xml"

// NewDataArt builds the DataArt adapter over the given HTTP client.
func NewDataArt(c dataartHTTP) Source { return dataart{http: c} }

func (dataart) Provider() string { return "dataart" }

// dataart is single-company, so its config entries carry no board.
func (dataart) boardless() {}

func (d dataart) Fetch(ctx context.Context, ce CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	if err := d.http.GetXML(ctx, dataartSitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("dataart: sitemap: %w", err)
	}

	// Keep only canonical English vacancy pages: the listing root, unrelated pages, and the
	// /xx/vacancies/… localisations yield no code, so each vacancy is ingested once.
	var urls []string
	for _, u := range sitemap.URLs {
		if dataartVacancyCode(u.Loc) != "" {
			urls = append(urls, u.Loc)
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return d.detail(ctx, ce, u)
	}), nil
}

// detail fetches one vacancy page and maps its JobPosting ld+json to a Job, returning ok=false
// when the URL has no code, the fetch fails, or the page carries no JobPosting, so the caller
// skips just that posting.
func (d dataart) detail(ctx context.Context, ce CompanyEntry, jobURL string) (Job, bool) {
	id := dataartVacancyCode(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := d.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p dataartPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	location := p.location()
	// A "Remote.*" tag is DataArt's structured remote-option signal, so it sets WorkMode
	// (clean provenance); other jobs leave WorkMode empty and the pipeline parses the location.
	remote := p.remote()
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

// dataartVacancyCodePattern captures the vacancy code from a canonical English vacancy URL
// (https://www.dataart.team/vacancies/<code>). Anchoring the host directly before /vacancies/
// excludes the /xx/vacancies/… localisations; requiring a code segment excludes the listing root.
var dataartVacancyCodePattern = regexp.MustCompile(`^https?://www\.dataart\.team/vacancies/([a-z0-9]+)/?$`)

// dataartVacancyCode extracts the vacancy code from a canonical English vacancy URL, returning
// "" for the listing root, localisations, and unrelated pages.
func dataartVacancyCode(u string) string {
	return firstSubmatch(dataartVacancyCodePattern, u)
}

// dataartPosting is the schema.org JobPosting decoded from a DataArt vacancy page's ld+json.
type dataartPosting struct {
	Title       string         `json:"title"`
	Description string         `json:"description"`
	DatePosted  string         `json:"datePosted"`
	JobLocation []dataartPlace `json:"jobLocation"`
}

type dataartPlace struct {
	Address dataartAddress `json:"address"`
}

type dataartAddress struct {
	Country  string `json:"addressCountry"`
	Locality string `json:"addressLocality"`
}

// location joins each jobLocation place as "City, Country" (falling back to whichever part is
// present), deduped and separated by "; ", so a job open in several cities lists them all. It
// drops DataArt's internal "Remote.*" region codes (see dataartRealPlace) so they don't leak
// into the user-visible location; a place left with no real part is skipped.
func (p dataartPosting) location() string {
	var out []string
	seen := make(map[string]struct{})
	for _, pl := range p.JobLocation {
		s := joinNonEmpty(dataartRealPlace(pl.Address.Locality), dataartRealPlace(pl.Address.Country))
		if s == "" {
			continue
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return strings.Join(out, "; ")
}

// remote reports whether any jobLocation carries DataArt's "Remote.*" tag — its explicit
// remote-option marker, which location() strips from the visible text.
func (p dataartPosting) remote() bool {
	for _, pl := range p.JobLocation {
		if strings.HasPrefix(pl.Address.Country, "Remote.") || strings.HasPrefix(pl.Address.Locality, "Remote.") {
			return true
		}
	}
	return false
}

// dataartRealPlace returns a place-name component unless it is one of DataArt's internal
// "Remote.*" region codes (e.g. "Remote.LATAM-country", "Remote.AR"), which are not real places.
func dataartRealPlace(s string) string {
	if strings.HasPrefix(s, "Remote.") {
		return ""
	}
	return s
}
