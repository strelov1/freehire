package sources

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strings"
	"sync/atomic"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// beesite adapts BeeSite, milch & zucker's career-site platform behind German corporate job
// portals (jobs.mercedes-benz.com, jobs.porsche.com, jobsearch.daimlertruck.com). Every portal
// searches through a keyless JSON endpoint on its own <name>.app.beesite.de host, and a board
// is that host.
//
// The search answers whole posting records. Some tenants include the body in them
// (Mercedes-Benz's PositionFormattedDescription) and most do not; for those the body is read
// from the posting page's schema.org JobPosting, only for postings the catalogue lacks.
type beesite struct {
	http beesiteHTTP
}

type beesiteHTTP interface {
	JSONGetter
	HTMLGetter
}

const (
	beesitePageSize = 100
	// beesiteMaxPages bounds the walk; reaching it before the stated total fails the crawl.
	beesiteMaxPages = 300
)

// NewBeeSite builds the BeeSite adapter over the shared client.
func NewBeeSite(c beesiteHTTP) Source { return beesite{http: c} }

func (beesite) Provider() string { return "beesite" }

// fullBoardListing: every page states SearchResultCountAll, and the walk fails unless the
// distinct postings read equal it. Any page failing, a full page that adds nothing, and the page
// cap all fail the crawl.
func (beesite) FullBoardListing() {}

type beesiteResponse struct {
	SearchResult struct {
		CountAll *int `json:"SearchResultCountAll"`
		Items    []struct {
			ID         string            `json:"MatchedObjectId"`
			Descriptor beesiteDescriptor `json:"MatchedObjectDescriptor"`
		} `json:"SearchResultItems"`
	} `json:"SearchResult"`
}

type beesiteName struct {
	Name string `json:"Name"`
}

type beesiteDescriptor struct {
	Title       string `json:"PositionTitle"`
	URI         string `json:"PositionURI"`
	Published   string `json:"PublicationStartDate"`
	Description []struct {
		Content        string `json:"Content"`
		Tasks          string `json:"Tasks"`
		Qualifications string `json:"Qualifications"`
	} `json:"PositionFormattedDescription"`
	Locations []struct {
		City        string `json:"CityName"`
		Country     string `json:"CountryName"`
		CountryCode string `json:"CountryCode"`
	} `json:"PositionLocation"`
	Schedule    []beesiteName `json:"PositionSchedule"`
	CareerLevel []beesiteName `json:"CareerLevel"`
}

// beesitePosting is one listed posting.
type beesitePosting struct {
	ID string
	d  beesiteDescriptor
}

func (s beesite) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s beesite) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	host := strings.ToLower(strings.TrimSpace(e.Board))
	if !strings.HasSuffix(host, ".beesite.de") || strings.ContainsAny(host, "/:?#@ ") {
		return nil, fmt.Errorf("beesite: board %q is not a *.beesite.de host", e.Board)
	}
	postings, err := s.list(ctx, host)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, e, postings, seen)
}

func beesiteSearchURL(host string, first int) string {
	// The default order is kept: no sort criterion is sent, since it pages without repeats or gaps.
	q, _ := json.Marshal(map[string]any{
		"LanguageCode":     "EN",
		"SearchParameters": map[string]any{"FirstItem": first, "CountItem": beesitePageSize},
	})
	return "https://" + host + "/search?data=" + url.QueryEscape(string(q))
}

func (s beesite) list(ctx context.Context, host string) ([]beesitePosting, error) {
	var out []beesitePosting
	ids := map[string]bool{}
	for page := 0; page < beesiteMaxPages; page++ {
		var resp beesiteResponse
		if err := s.http.GetJSON(ctx, beesiteSearchURL(host, page*beesitePageSize+1), &resp); err != nil {
			return nil, fmt.Errorf("beesite: page %d: %w", page+1, err)
		}
		sr := resp.SearchResult
		if sr.CountAll == nil {
			return nil, fmt.Errorf("beesite: page %d states no SearchResultCountAll", page+1)
		}
		added := 0
		for _, it := range sr.Items {
			if it.ID == "" || ids[it.ID] {
				continue
			}
			ids[it.ID] = true
			out = append(out, beesitePosting{ID: it.ID, d: it.Descriptor})
			added++
		}
		if len(sr.Items) < beesitePageSize {
			if len(ids) != *sr.CountAll {
				return nil, fmt.Errorf("beesite: read %d distinct postings, the search states %d", len(ids), *sr.CountAll)
			}
			return out, nil
		}
		if added == 0 {
			return nil, fmt.Errorf("beesite: page %d added no posting (%d of %d)", page+1, len(ids), *sr.CountAll)
		}
	}
	return nil, fmt.Errorf("beesite: no end of the listing after %d pages", beesiteMaxPages)
}

// hydrate yields every listed posting. One whose record carries its body needs no request; for
// the rest the posting page is read only when the catalogue lacks the posting, and a page that
// cannot be read defers that posting to a later run rather than storing it body-less. A crawl
// that had to read pages and read none of them fails.
func (s beesite) hydrate(ctx context.Context, e CompanyEntry, postings []beesitePosting, seen func(string) bool) ([]Job, error) {
	var wanted, read atomic.Int64
	jobs := fetchDetails(postings, defaultDetailWorkers, func(p beesitePosting) (Job, bool) {
		job, ok := beesiteJob(p, e)
		if !ok {
			return Job{}, false
		}
		if job.Description != "" {
			return job, true
		}
		if seen(p.ID) {
			return Job{ExternalID: job.ExternalID, URL: job.URL, Title: job.Title, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, job.URL)
		if err != nil {
			if detailUnreadable(err) {
				log.Printf("beesite: %s: %v; deferring", job.URL, err)
			}
			return Job{}, false
		}
		desc := beesitePageDescription(root)
		if desc == "" {
			log.Printf("beesite: %s carries no JobPosting body; deferring", job.URL)
			return Job{}, false
		}
		job.Description = desc
		read.Add(1)
		return job, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("beesite: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// beesiteJob maps a search record. The posting URL is the portal's own page (PositionURI), on
// the employer's domain rather than the beesite host.
func beesiteJob(p beesitePosting, e CompanyEntry) (Job, bool) {
	d := p.d
	title := strings.Join(strings.Fields(html.UnescapeString(d.Title)), " ")
	u, err := url.Parse(strings.TrimSpace(d.URI))
	if title == "" || err != nil || (u.Scheme != "https" && u.Scheme != "http") || u.Host == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID: p.ID,
		URL:        u.String(),
		Title:      title,
		Company:    e.Company,
		PostedAt:   parseDate(d.Published),
	}
	var body strings.Builder
	for _, s := range d.Description {
		body.WriteString(s.Content)
		body.WriteString(s.Tasks)
		body.WriteString(s.Qualifications)
	}
	job.Description = sanitizeHTML(body.String())
	if strings.TrimSpace(job.Description) == "" {
		job.Description = ""
	}
	var places []string
	for _, l := range d.Locations {
		place := strings.TrimSpace(l.City)
		if c := strings.TrimSpace(l.Country); c != "" {
			if place != "" {
				place += ", "
			}
			place += c
		}
		if place != "" && !slices.Contains(places, place) {
			places = append(places, place)
		}
		for _, c := range countryFromCode(l.CountryCode) {
			if !slices.Contains(job.Countries, c) {
				job.Countries = append(job.Countries, c)
			}
		}
	}
	job.Location = strings.Join(places, "; ")
	if len(d.Schedule) > 0 {
		s := strings.ToLower(d.Schedule[0].Name)
		switch {
		case strings.HasPrefix(s, "full"):
			job.EmploymentType = "full_time"
		case strings.HasPrefix(s, "part"):
			job.EmploymentType = "part_time"
		}
	}
	for _, l := range d.CareerLevel {
		if strings.EqualFold(l.Name, "Internship") {
			job.EmploymentType, job.Seniority = "internship", "intern"
		}
	}
	return job, true
}

// beesitePageDescription reads the body from the posting page's schema.org JobPosting, else
// from the page's description container. The
// portal template prints a stray token before the JSON on some tenants (Porsche's block opens
// "entryLevel [{...}]"), so the block is read from its first bracket. The description is
// entity-encoded HTML.
func beesitePageDescription(root *html.Node) string {
	var desc string
	walk(root, func(n *html.Node) bool {
		if desc != "" {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "script" || Attr(n, "type") != "application/ld+json" {
			return true
		}
		raw := flexjson.SanitizeControlChars([]byte(textContent(n)))
		if i := bytes.IndexAny(raw, "[{"); i > 0 {
			raw = raw[i:]
		}
		node, ok := jobPostingNode(raw)
		if !ok {
			return false
		}
		var p struct {
			Description string `json:"description"`
		}
		if json.Unmarshal(node, &p) == nil {
			desc = sanitizeHTML(unescapeEncodedHTML(p.Description))
		}
		return false
	})
	if desc = strings.TrimSpace(desc); desc != "" {
		return desc
	}
	// A posting the employer handles outside the portal (an apply-by-email one, say) renders
	// no JobPosting at all; its page still shows the body in the template's own container.
	return strings.TrimSpace(sanitizeHTML(elementInnerHTMLByClass(root, "div", "jobad-dynamic-html-container")))
}
