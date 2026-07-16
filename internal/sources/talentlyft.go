package sources

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// talentlyft adapts TalentLyft career sites. The board is the tenant subdomain (e.g.
// "inetec"), forming the host "<board>.talentlyft.com". The tenant's sitemap.xml enumerates
// every /jobs/<slug>-<code> posting, and each posting page server-renders a schema.org
// JobPosting ld+json — the dataart/SuccessFactors shape (sitemap to enumerate, per-posting
// detail fetch for the posting) over the shared ld+json helper.
type talentlyft struct {
	http talentlyftHTTP
}

// talentlyftHTTP is the transport talentlyft needs: the XML sitemap plus HTML detail pages.
type talentlyftHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewTalentLyft builds the TalentLyft adapter over the given HTTP client.
func NewTalentLyft(c talentlyftHTTP) Source { return talentlyft{http: c} }

func (talentlyft) Provider() string { return "talentlyft" }

func (s talentlyft) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var sitemap struct {
		URLs []struct {
			Loc string `xml:"loc"`
		} `xml:"url"`
	}
	sitemapURL := fmt.Sprintf("https://%s.talentlyft.com/sitemap.xml", e.Board)
	if err := s.http.GetXML(ctx, sitemapURL, &sitemap); err != nil {
		return nil, fmt.Errorf("talentlyft: sitemap %s: %w", e.Board, err)
	}

	// Keep only posting URLs (the listing root and section pages carry no job code).
	var urls []string
	for _, u := range sitemap.URLs {
		if talentlyftJobID(u.Loc) != "" {
			urls = append(urls, u.Loc)
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return s.detail(ctx, e, u)
	}), nil
}

// detail fetches one posting page and maps its JobPosting ld+json to a Job, returning ok=false
// when the URL has no code, the fetch fails, or the page carries no JobPosting, so the caller
// skips just that posting.
func (s talentlyft) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := talentlyftJobID(jobURL)
	if id == "" {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p talentlyftPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	location := joinNonEmpty(
		p.JobLocation.Address.AddressLocality,
		p.JobLocation.Address.AddressCountry,
	)
	return Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          p.Title,
		Company:        e.Company,
		Location:       location,
		Description:    sanitizeHTML(html.UnescapeString(p.Description)),
		Remote:         isRemote(location),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       parseDate(p.DatePosted),
	}, true
}

// talentlyftJobIDPattern captures the trailing code from a /jobs/<slug>-<code> URL (the
// human slug carries the TalentLyft posting code as its final hyphen segment). The greedy
// prefix backtracks to the LAST hyphen, so a multi-word slug still yields just the code.
var talentlyftJobIDPattern = regexp.MustCompile(`/jobs/.+-([A-Za-z0-9]{3,})$`)

// talentlyftJobID extracts the native posting code from a detail URL, or "" for the listing
// root and section pages that carry no code.
func talentlyftJobID(loc string) string {
	if m := talentlyftJobIDPattern.FindStringSubmatch(trimURLSuffix(loc)); m != nil {
		return m[1]
	}
	return ""
}

// talentlyftPosting is the schema.org JobPosting decoded from a TalentLyft posting page's
// ld+json. jobLocation is a single Place.
type talentlyftPosting struct {
	Title          string `json:"title"`
	Description    string `json:"description"`
	DatePosted     string `json:"datePosted"`
	EmploymentType string `json:"employmentType"`
	JobLocation    struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}
