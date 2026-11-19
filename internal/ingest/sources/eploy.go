package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"
)

// eploy adapts Eploy, a UK ATS whose career sites are server-rendered ASP.NET, usually on the
// employer's own domain (jobs.ocs.com, careers.allpay.net) and always also on
// <tenant>.eploy.net. A board is that careers HOST.
//
// The list is the site's own /live-jobs.xml, a sitemap of every live vacancy in one document.
// That avoids the results pager, a Web Forms postback. The sitemap carries no title and no
// body, so this is a HydratingSource over the vacancy page.
type eploy struct {
	http eployHTTP
}

type eployHTTP interface {
	XMLGetter
	HTMLGetter
}

// NewEploy builds the Eploy adapter over the shared client.
func NewEploy(c eployHTTP) Source { return eploy{http: c} }

func (eploy) Provider() string { return "eploy" }

// fullBoardListing: /live-jobs.xml is the whole live inventory in one unpaginated document,
// so reading it IS reaching the natural end. A failed or undecodable sitemap fails the crawl,
// and so does a sitemap INDEX, which would mean the inventory had been split across documents
// this adapter does not read.
func (eploy) FullBoardListing() {}

// eployVacancyPath is a vacancy page's path: /vacancies/<id>/<slug>.html.
var eployVacancyPath = regexp.MustCompile(`^/vacancies/(\d+)/[^/]+\.html$`)

// eployListing is one vacancy the sitemap names.
type eployListing struct {
	ID, URL string
}

func (s eploy) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s eploy) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	host := strings.ToLower(strings.TrimSpace(e.Board))
	if host == "" || !strings.Contains(host, ".") || strings.ContainsAny(host, "/:?#@ ") {
		return nil, fmt.Errorf("eploy: board %q is not a careers host", e.Board)
	}
	doc, err := getSitemap(ctx, s.http, "https://"+host+"/live-jobs.xml")
	if err != nil {
		return nil, fmt.Errorf("eploy: live-jobs.xml: %w", err)
	}
	if len(doc.Sitemaps) > 0 {
		return nil, fmt.Errorf("eploy: live-jobs.xml is a sitemap index (%d sitemaps)", len(doc.Sitemaps))
	}
	list := eployListings(doc, host)
	return s.hydrate(ctx, e, list, seen)
}

// eployListings reads the vacancies the sitemap names. A <loc> is accepted on the board's own
// host or on an *.eploy.net host: a branded site's sitemap commonly names the tenant's
// eploy.net page (careers.pei.group lists peigroupweb.eploy.net), which is the same vacancy.
func eployListings(doc sitemapDoc, host string) []eployListing {
	var out []eployListing
	ids := map[string]bool{}
	for _, u := range doc.URLs {
		loc, err := url.Parse(strings.TrimSpace(u.Loc))
		if err != nil || loc.Scheme != "https" {
			continue
		}
		h := strings.ToLower(loc.Hostname())
		if h != host && !strings.HasSuffix(h, ".eploy.net") {
			continue
		}
		m := eployVacancyPath.FindStringSubmatch(loc.Path)
		if m == nil || ids[m[1]] {
			continue
		}
		ids[m[1]] = true
		out = append(out, eployListing{ID: m[1], URL: loc.String()})
	}
	return out
}

// hydrate reads the vacancy page of each vacancy the catalogue lacks. A page that could not be
// read is deferred to a later run rather than stored body-less; the sitemap lags the site, so a
// vacancy it still names can already answer 410 Gone. A crawl that listed new vacancies and
// read none of them fails.
func (s eploy) hydrate(ctx context.Context, e CompanyEntry, list []eployListing, seen func(string) bool) ([]Job, error) {
	var wanted, read atomic.Int64
	jobs := fetchDetails(list, defaultDetailWorkers, func(l eployListing) (Job, bool) {
		if seen(l.ID) {
			return Job{ExternalID: l.ID, URL: l.URL, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, l.URL)
		if err != nil {
			if detailUnreadable(err) {
				log.Printf("eploy: %s: %v; deferring", l.URL, err)
			}
			return Job{}, false
		}
		job, ok := parseEployVacancy(root, l)
		if !ok {
			log.Printf("eploy: %s carries no vacancy; deferring", l.URL)
			return Job{}, false
		}
		job.Company = e.Company
		read.Add(1)
		return job, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("eploy: listed %d new vacancies and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// eployPosting is the subset of the vacancy page's schema.org JobPosting the adapter maps. Not
// every tenant renders one (HomeServe's template does not), so it only adds to what the page's
// markup states.
type eployPosting struct {
	DatePosted     string `json:"datePosted"`
	EmploymentType string `json:"employmentType"`
	JobLocation    struct {
		Address struct {
			Locality string `json:"addressLocality"`
			Region   string `json:"addressRegion"`
			Country  string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// parseEployVacancy maps a vacancy page. The title is the page's h1 and the body is its
// vac-details__description block; every Eploy template measured carries both, with or without
// a JobPosting block.
func parseEployVacancy(root *html.Node, l eployListing) (Job, bool) {
	var title string
	walk(root, func(n *html.Node) bool {
		if title != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "h1" && strings.HasSuffix(Attr(n, "id"), "h1JobTitle") {
			title = strings.Join(strings.Fields(textContent(n)), " ")
			return false
		}
		return true
	})
	desc := sanitizeHTML(elementInnerHTMLByClass(root, "div", "vac-details__description"))
	if title == "" || strings.TrimSpace(desc) == "" {
		return Job{}, false
	}
	job := Job{ExternalID: l.ID, URL: l.URL, Title: title, Description: desc}
	var p eployPosting
	if ldJobPosting(root, &p) {
		job.PostedAt = parseDate(p.DatePosted)
		a := p.JobLocation.Address
		var parts []string
		for _, s := range []string{a.Locality, a.Region, a.Country} {
			if s = strings.TrimSpace(s); s != "" {
				parts = append(parts, s)
			}
		}
		job.Location = strings.Join(parts, ", ")
		job.EmploymentType = eployEmploymentType(p.EmploymentType)
	}
	return job, true
}

// eployEmploymentType maps the tenant-authored employment type. Most tenants say "Permanent",
// which states tenure, not hours, so only an explicit schedule or contract term is read.
func eployEmploymentType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full time", "full-time", "fulltime":
		return "full_time"
	case "part time", "part-time", "parttime":
		return "part_time"
	case "contract", "fixed term", "fixed-term", "temporary":
		return "contract"
	case "internship", "intern":
		return "internship"
	}
	return ""
}
