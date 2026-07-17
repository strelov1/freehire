package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"golang.org/x/net/html"
)

// careerplug adapts CareerPlug career sites (<board>.careerplug.com): a server-rendered ATS
// whose /jobs listing paginates with ?page=N and links each posting as /jobs/<id>, and whose
// job pages carry a schema.org JobPosting ld+json. The board is the account subdomain. Like
// the other ld+json detail adapters it fetches each posting's page for the description; the
// employer comes per-posting from the JSON-LD (CareerPlug accounts are often franchises whose
// postings name the individual location), falling back to the configured company. It exposes
// no jobLocationType, so the remote flag falls back to the location text.
type careerplug struct {
	http HTMLGetter
}

// NewCareerPlug builds the CareerPlug adapter over the given HTTP client.
func NewCareerPlug(c HTMLGetter) Source { return careerplug{http: c} }

func (careerplug) Provider() string { return "careerplug" }

// careerplugMaxPages bounds listing pagination so a board that clamps ?page=N to its last
// page (serving the same links forever) cannot loop; the no-new-links check ends it sooner.
const careerplugMaxPages = 100

func (s careerplug) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host := fmt.Sprintf("%s.careerplug.com", e.Board)
	// base carries scheme+host so the listing's relative /jobs/<id> hrefs resolve to
	// fetchable URLs; parsed once rather than per page.
	base, err := url.Parse(fmt.Sprintf("https://%s/", host))
	if err != nil {
		return nil, fmt.Errorf("careerplug: board %q: %w", e.Board, err)
	}

	var urls []string
	seen := make(map[string]bool)
	for page := 1; page <= careerplugMaxPages; page++ {
		listURL := fmt.Sprintf("https://%s/jobs", host)
		if page > 1 {
			listURL = fmt.Sprintf("https://%s/jobs?page=%d", host, page)
		}
		root, err := s.http.GetHTML(ctx, listURL)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("careerplug: listing %s: %w", e.Board, err)
			}
			break // a later page failing ends enumeration with the jobs gathered so far
		}
		newLinks := 0
		for _, link := range careerplugJobLinks(base, root) {
			if !seen[link] {
				seen[link] = true
				urls = append(urls, link)
				newLinks++
			}
		}
		if newLinks == 0 { // empty page, or a board clamping ?page=N past its last page
			break
		}
	}

	return fetchDetails(urls, defaultDetailWorkers, func(u string) (Job, bool) {
		return s.detail(ctx, e, u)
	}), nil
}

// detail fetches one job page and maps its JobPosting ld+json to a Job, returning ok=false
// when the page fetch fails, carries no JobPosting, or has no parseable id, so the caller
// skips just that posting.
func (s careerplug) detail(ctx context.Context, e CompanyEntry, jobURL string) (Job, bool) {
	id := careerplugJobID(jobURL)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	var p careerplugPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}

	location := p.location()
	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       p.Title,
		Company:     firstNonEmpty(p.HiringOrganization.Name, e.Company),
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(p.Description)),
		// No jobLocationType is emitted, so the location text is the only remote signal
		// (never the title, which false-positives on "Remote …" role names).
		Remote:   isRemote(location),
		PostedAt: parseRFC3339(p.DatePosted),
	}, true
}

// careerplugJobIDPattern captures the numeric posting id from a /jobs/<id> URL. The digit
// requirement keeps the bare /jobs listing and its ?page=N variants from being mistaken for
// a job.
var careerplugJobIDPattern = regexp.MustCompile(`/jobs/(\d+)`)

// careerplugJobID extracts the native posting id from a job URL, or "" when absent.
func careerplugJobID(u string) string {
	return firstSubmatch(careerplugJobIDPattern, u)
}

// careerplugJobLinks returns the absolute hrefs of all /jobs/<id> job-page anchors, resolved
// against base so relative hrefs yield fetchable URLs, de-duplicated in first-seen order (a
// card links the same job from its title and other controls).
func careerplugJobLinks(base *url.URL, root *html.Node) []string {
	var out []string
	seen := make(map[string]bool)
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && n.Data == "a" {
			href := attr(n, "href")
			if careerplugJobID(href) == "" {
				return true
			}
			ref, err := url.Parse(href)
			if err != nil {
				return true
			}
			abs := base.ResolveReference(ref).String()
			if !seen[abs] {
				seen[abs] = true
				out = append(out, abs)
			}
		}
		return true
	})
	return out
}

// careerplugPosting is the schema.org JobPosting decoded from a CareerPlug job page's
// ld+json. jobLocation is a single Place (not an array).
type careerplugPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			AddressLocality string `json:"addressLocality"`
			AddressRegion   string `json:"addressRegion"`
			AddressCountry  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// location builds the display location from the posting's address (city, region, country).
func (p careerplugPosting) location() string {
	a := p.JobLocation.Address
	return joinNonEmpty(a.AddressLocality, a.AddressRegion, a.AddressCountry)
}
