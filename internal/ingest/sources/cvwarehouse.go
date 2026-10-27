package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
)

// cvwarehouse adapts CVWarehouse, a Belgian ATS with many customers in Belgium and Portugal
// (IT consultancies such as agap2IT, Adentis, Team IT, KCS IT among them). Every customer's career
// site is one server-rendered page on a shared host, named by the customer's companyGuid:
//
//	https://jobpage.cvwarehouse.com/?companyGuid=<board>
//
// The page redirects to the customer's default language and carries every listed vacancy at
// once: a list item (the vacancy id, and the filters the page's own search reads — country,
// city, region, work type, schedule) and a hidden detail block with the title and the full body.
// So the page IS the listing and the bodies; no request is spent per vacancy.
//
// Two shapes of site:
//   - flat: the home page lists the vacancies.
//   - sectioned: the home page lists only SECTIONS (?section=<guid>), each its own page of
//     vacancies. 40 of 52 customers that looked empty on their home page were this (2026-10-27).
//
// Both are read the same way: the vacancies of the home page plus those of every section it
// links, de-duplicated by id. Any page failing fails the board, so the read is complete.
type cvwarehouse struct {
	http HTMLGetter
}

// NewCVWarehouse builds the CVWarehouse adapter over the given HTML client.
func NewCVWarehouse(c HTMLGetter) Source { return cvwarehouse{http: c} }

func (cvwarehouse) Provider() string { return "cvwarehouse" }

// fullBoardListing: one page (plus its sections) carries every listed vacancy, and any page that
// fails fails the crawl.
func (cvwarehouse) fullBoardListing() {}

// cvwarehouseRequestInterval paces the provider: one host serves every customer's pages, each
// 0.3-0.6 MB, so the whole fleet's run is a few hundred requests and needs no more than this.
const cvwarehouseRequestInterval = 500 * time.Millisecond

// cvwarehouseBoard is a companyGuid.
var cvwarehouseBoard = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// cvwarehouseMaxSections bounds the section fan-out of one site; the largest seen links 20.
const cvwarehouseMaxSections = 100

// cvwarehouseOpenApplication matches the standing "open application" a site lists beside its
// vacancies, in the languages the platform serves.
var cvwarehouseOpenApplication = regexp.MustCompile(`(?i)candidatura[s]? espont|spontane? sollicitatie|spontaneous application|open application|open sollicitatie|candidature spontan|initiativbewerbung`)

// cvwarehouseCountries maps the platform's numeric country filter to ISO codes. The ids are the
// positions of an alphabetical English country list; only ids verified against the cities of real
// vacancies are listed, and an unknown id leaves the country to the location dictionary.
var cvwarehouseCountries = map[string]string{
	"22":  "be", // Antwerpen, Gent, Bruxelles
	"40":  "cv", // Praia
	"51":  "cd", // Bukavu
	"74":  "fr", // Chambéry, Limonest
	"81":  "de", // Altenburg
	"127": "lu", // Capellen
	"154": "nl", // Amsterdam, Den Haag
	"176": "pt", // Lisboa, Porto
	"203": "es", // Madrid, Moguer
	"205": "sd", // Port Sudan
}

func cvwarehouseURL(board string, extra url.Values) string {
	q := url.Values{"companyGuid": {board}}
	for k, v := range extra {
		q[k] = v
	}
	return "https://jobpage.cvwarehouse.com/?" + q.Encode()
}

// Fetch reads the site's pages and maps every listed vacancy.
func (s cvwarehouse) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !cvwarehouseBoard.MatchString(board) {
		return nil, fmt.Errorf("cvwarehouse: board %q is not a companyGuid", e.Board)
	}
	home, err := s.page(ctx, cvwarehouseURL(board, nil))
	if err != nil {
		return nil, err
	}
	company := firstNonEmpty(e.Company, cvwarehouseMeta(home, "author"))
	seen := map[string]bool{}
	jobs := cvwarehouseJobs(home, board, company, seen)
	sections := cvwarehouseSections(home, board)
	if len(sections) > cvwarehouseMaxSections {
		return nil, fmt.Errorf("cvwarehouse: board %s links %d sections, more than %d", board, len(sections), cvwarehouseMaxSections)
	}
	for _, sec := range sections {
		root, err := s.page(ctx, cvwarehouseURL(board, url.Values{"section": {sec}}))
		if err != nil {
			return nil, fmt.Errorf("section %s: %w", sec, err)
		}
		jobs = append(jobs, cvwarehouseJobs(root, board, company, seen)...)
	}
	return jobs, nil
}

// page fetches one of the site's pages and proves it is one: a career page renders <body
// class="home">, while an unknown companyGuid answers 200 with the platform's "not available" page,
// which must fail rather than read as a site with no vacancies.
func (s cvwarehouse) page(ctx context.Context, u string) (*html.Node, error) {
	root, err := s.http.GetHTML(ctx, u)
	if err != nil {
		return nil, fmt.Errorf("cvwarehouse: %s: %w", u, err)
	}
	body := firstByTag(root, "body")
	if body == nil || !HasClass(body, "home") {
		return nil, fmt.Errorf("cvwarehouse: %s is not a career page", u)
	}
	return root, nil
}

// cvwarehouseSections returns the section ids a page links, in order. Only links to the site's
// own listing page count; the apply form also carries a section parameter.
func cvwarehouseSections(root *html.Node, board string) []string {
	var out []string
	seen := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		u, err := url.Parse(Attr(n, "href"))
		if err != nil || (u.Path != "/" && u.Path != "") {
			return true
		}
		q := u.Query()
		sec := strings.ToLower(q.Get("section"))
		if strings.ToLower(q.Get("companyGuid")) != board || !cvwarehouseBoard.MatchString(sec) || seen[sec] {
			return true
		}
		seen[sec] = true
		out = append(out, sec)
		return true
	})
	return out
}

// cvwarehouseJobs maps the vacancies a page lists. A vacancy is LISTED when a list item links it:
// the page also carries hidden detail blocks for vacancies the customer does not list on its site
// (KCS IT: 9 listed, 94 blocks), and those are left alone. seen de-duplicates across sections.
func cvwarehouseJobs(root *html.Node, board, company string, seen map[string]bool) []Job {
	details := map[string]*html.Node{}
	var items []*html.Node
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		if id := Attr(n, "data-jobdetail-job-id"); id != "" {
			if _, dup := details[id]; !dup {
				details[id] = n
			}
			return false
		}
		if Attr(n, "data-item-collection") != "" {
			items = append(items, n)
			return false
		}
		return true
	})
	var jobs []Job
	for _, item := range items {
		link := firstByClass(item, "jobLink")
		if link == nil {
			continue
		}
		id := strings.TrimSpace(Attr(link, "data-jobid"))
		if id == "" || seen[id] {
			continue
		}
		seen[id] = true
		title := strings.Join(strings.Fields(textContent(link)), " ")
		if title == "" || cvwarehouseOpenApplication.MatchString(title) {
			continue
		}
		j := Job{
			ExternalID:     id,
			URL:            cvwarehouseURL(board, url.Values{"job": {id}}),
			Title:          title,
			Company:        company,
			Location:       cvwarehouseLocation(Attr(item, "data-filter-city"), Attr(item, "data-filter-region")),
			EmploymentType: cvwarehouseEmploymentType(Attr(item, "data-filter-worktype"), Attr(item, "data-filter-workschedule")),
		}
		if c := cvwarehouseCountries[strings.TrimSpace(Attr(item, "data-filter-country"))]; c != "" {
			j.Countries = []string{c}
		}
		if d := details[id]; d != nil {
			if body := firstByClass(d, "jobDescriptionText"); body != nil {
				j.Description = sanitizeHTML(innerHTML(body))
			}
			// The canonical names the vacancy's language; its channel parameter is attribution
			// for the customer's own site and is dropped.
			if canon, err := url.Parse(strings.TrimSpace(Attr(d, "data-canonical-url"))); err == nil && canon.Host == "jobpage.cvwarehouse.com" {
				q := canon.Query()
				q.Del("channel")
				canon.RawQuery = q.Encode()
				j.URL = canon.String()
			}
		}
		jobs = append(jobs, j)
	}
	return jobs
}

// cvwarehouseLocation joins the city with the regions the vacancy names, dropping a region that
// repeats the city ("Lisboa, Lisboa").
func cvwarehouseLocation(city, regionsJSON string) string {
	city = strings.TrimSpace(city)
	parts := []string{city}
	var regions []string
	if json.Unmarshal([]byte(regionsJSON), &regions) == nil {
		for _, r := range regions {
			r = strings.TrimSpace(r)
			if r != "" && !strings.EqualFold(r, city) && !strings.EqualFold(r, "hidden region") {
				parts = append(parts, r)
			}
		}
	}
	return joinNonEmpty(parts...)
}

// cvwarehouseEmploymentType reads the work type (freelance, internship) and, failing that, the
// schedule list, in the languages the platform's customers write them in. A schedule naming both
// full and part time says nothing.
func cvwarehouseEmploymentType(workType, schedulesJSON string) string {
	switch strings.ToLower(strings.TrimSpace(workType)) {
	case "freelance", "zelfstandige", "indépendant", "independente", "trabalhador independente":
		return "contract"
	case "stage", "stagiair", "estágio", "estagiário", "internship", "intern":
		return "internship"
	}
	var schedules []string
	if json.Unmarshal([]byte(schedulesJSON), &schedules) != nil {
		return ""
	}
	full, part := false, false
	for _, s := range schedules {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "voltijds", "tempo inteiro", "full time", "full-time", "temps plein", "vollzeit":
			full = true
		case "deeltijds", "halftijds", "tempo parcial", "part time", "part-time", "temps partiel", "mi-temps", "teilzeit":
			part = true
		}
	}
	switch {
	case full && !part:
		return "full_time"
	case part && !full:
		return "part_time"
	}
	return ""
}

// cvwarehouseMeta reads <meta name="<name>" content="...">.
func cvwarehouseMeta(root *html.Node, name string) string {
	var out string
	walk(root, func(n *html.Node) bool {
		if out != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "meta" && Attr(n, "name") == name {
			out = strings.TrimSpace(Attr(n, "content"))
		}
		return true
	})
	return out
}
