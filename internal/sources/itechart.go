package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// itechart adapts iTechArt's careers site (itechartgroup.by) — distinct from the Vention
// rebrand. The board is the career-site host. A plain XML sitemap enumerates the vacancies
// and each /job-openings/<slug> page server-renders a schema.org JobPosting ld+json block,
// so this is the SuccessFactors/EPAM shape (sitemap to enumerate, per-job detail fetch over
// the shared ld+json helper).
type itechart struct {
	http itechartHTTP
}

type itechartHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewITechArt builds the iTechArt adapter over the given HTTP client.
func NewITechArt(c itechartHTTP) Source { return itechart{http: c} }

func (itechart) Provider() string { return "itechart" }

func (i itechart) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	sitemapURL := fmt.Sprintf("https://%s/sitemap.xml", e.Board)
	if err := i.http.GetXML(ctx, sitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("itechart: sitemap %s: %w", e.Board, err)
	}

	var urls []string
	for _, u := range sitemap.URLs {
		if itechartJobID(u.Loc) != "" {
			urls = append(urls, u.Loc)
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return i.detail(ctx, e, u)
	}), nil
}

func (i itechart) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := itechartJobID(jobURL)
	if id == "" {
		return Job{}, false
	}
	root, err := i.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p itechartPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.location()
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(p.DatePosted),
	}, true
}

// itechartJobIDPattern captures the slug from a /job-openings/<slug> URL. The slug segment
// is required so the bare /job-openings listing is not mistaken for a vacancy. The slug is
// the dedup id (the JobPosting ld+json carries no identifier).
var itechartJobIDPattern = regexp.MustCompile(`/job-openings/([a-z0-9-]+)(?:[/?#]|$)`)

func itechartJobID(u string) string {
	return firstSubmatch(itechartJobIDPattern, u)
}

// itechartPosting is the schema.org JobPosting on an iTechArt vacancy page. jobLocation is a
// single Place object whose address carries the locality.
type itechartPosting struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

func (p itechartPosting) location() string {
	return joinNonEmpty(p.JobLocation.Address.AddressLocality, p.JobLocation.Address.AddressCountry)
}
