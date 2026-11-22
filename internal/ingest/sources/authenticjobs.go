package sources

import (
	"context"
	"fmt"
	"html"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/platform/flexjson"
)

// authenticjobs adapts Authentic Jobs (authenticjobs.com), a design, creative and developer job
// board built on WordPress with the WP Job Manager plugin. Boardless and an aggregator: the
// employer is each posting's own company name.
//
// robots.txt disallows every URL carrying a query string (`/*?`) and asks for a 3 s crawl delay,
// which rules out the usual WordPress routes: REST pagination is `?page=N`, the RSS feed is
// `/feed/`, and the site publishes no sitemap. The adapter therefore uses two query-free routes:
// the plugin's own listing endpoint (POST /jm-ajax/get_listings/, one page for the whole
// catalogue) for the posting ids, and the REST item route (/wp-json/wp/v2/job-listings/<id>) for
// each posting's body and fields. See providers/authenticjobs.md.
type authenticjobs struct {
	http    authenticjobsHTTP
	limiter waiter
}

type authenticjobsHTTP interface {
	FormJSONPoster
	JSONGetter
}

const (
	authenticjobsBase = "https://authenticjobs.com"
	// authenticjobsRequestInterval honours robots.txt's Crawl-delay: 3.
	authenticjobsRequestInterval = 3 * time.Second
	// authenticjobsPerPage is far above the catalogue (42 postings on 2026-11-22), so the listing
	// is one page; a second page would mean the catalogue outgrew it and fails the crawl.
	authenticjobsPerPage = 200
)

// NewAuthenticJobs builds the Authentic Jobs adapter over the shared client, paced to the
// site's crawl delay across every request of a run.
func NewAuthenticJobs(c authenticjobsHTTP) Source {
	return authenticjobs{http: c, limiter: rate.NewLimiter(rate.Every(authenticjobsRequestInterval), 1)}
}

func (authenticjobs) Provider() string { return "authenticjobs" }
func (authenticjobs) Boardless()       {}
func (authenticjobs) Aggregator()      {}

// authenticjobsListing is the plugin's listing answer: the cards as an HTML fragment.
type authenticjobsListing struct {
	FoundJobs   bool   `json:"found_jobs"`
	MaxNumPages int    `json:"max_num_pages"`
	HTML        string `json:"html"`
}

// authenticjobsCard matches a card's <li class="post-<id> job_listing ..."> and its link.
var authenticjobsCard = regexp.MustCompile(`<li class="post-(\d+) job_listing[^>]*>\s*<a href="([^"]+)"`)

// authenticjobsCardRef is one listed posting: its id and page URL.
type authenticjobsCardRef struct{ ID, URL string }

// authenticjobsPosting is one REST job-listings item, reduced to what the adapter reads.
type authenticjobsPosting struct {
	DateGMT string `json:"date_gmt"`
	Link    string `json:"link"`
	Title   struct {
		Rendered string `json:"rendered"`
	} `json:"title"`
	Content struct {
		Rendered string `json:"rendered"`
	} `json:"content"`
	Meta struct {
		Location string        `json:"_job_location"`
		Company  string        `json:"_company_name"`
		Filled   flexjson.Bool `json:"_filled"`
		Remote   flexjson.Bool `json:"_remote_position"`
	} `json:"meta"`
	Categories []int    `json:"job-categories"`
	Classes    []string `json:"class_list"`
}

// authenticjobsEmploymentTypes maps the job-type term slug (a `job-type-<slug>` class on the
// item) to freehire's vocabulary.
var authenticjobsEmploymentTypes = map[string]string{
	"full-time":  "full_time",
	"part-time":  "part_time",
	"freelance":  "contract",
	"internship": "internship",
}

// authenticjobsTechCategories are the site's own categories that are technical by name.
var authenticjobsTechCategories = map[string]bool{
	"Artificial Intelligence (AI)": true,
	"Back-end Engineering":         true,
	"Front-end Engineering":        true,
}

func (s authenticjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	categories := map[int]string{}
	var jobs []Job
	read, unreadable := 0, 0
	for _, c := range cards {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		j, ok, err := s.detail(ctx, c.ID, categories)
		switch {
		case err != nil && detailUnreadable(err):
			unreadable++
			jobs = append(jobs, unreadableDetail(c.ID, c.URL, e.Company))
		case err != nil, !ok:
			// 404/410 or a filled position: the platform's own "gone".
		default:
			read++
			jobs = append(jobs, j)
		}
	}
	// A crawl that listed postings and read none of them is an outage, not an empty board.
	if unreadable > 0 && read == 0 {
		return nil, fmt.Errorf("authenticjobs: listed %d postings, read none", len(cards))
	}
	return jobs, nil
}

// list reads the postings (id and URL) of the whole open catalogue from the plugin's listing endpoint.
func (s authenticjobs) list(ctx context.Context) ([]authenticjobsCardRef, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	var resp authenticjobsListing
	form := url.Values{"page": {"1"}, "per_page": {strconv.Itoa(authenticjobsPerPage)}}
	if err := s.http.PostFormJSON(ctx, authenticjobsBase+"/jm-ajax/get_listings/", form, &resp); err != nil {
		return nil, fmt.Errorf("authenticjobs: listing: %w", err)
	}
	if resp.MaxNumPages > 1 {
		return nil, fmt.Errorf("authenticjobs: listing spans %d pages, one is read", resp.MaxNumPages)
	}
	seen := map[string]bool{}
	var cards []authenticjobsCardRef
	for _, m := range authenticjobsCard.FindAllStringSubmatch(resp.HTML, -1) {
		if !seen[m[1]] {
			seen[m[1]] = true
			cards = append(cards, authenticjobsCardRef{ID: m[1], URL: html.UnescapeString(m[2])})
		}
	}
	if resp.FoundJobs && len(cards) == 0 {
		return nil, fmt.Errorf("authenticjobs: listing reports jobs but no card was read")
	}
	return cards, nil
}

// detail reads one posting from its REST item. ok=false with a nil error is a posting the site
// marks filled.
func (s authenticjobs) detail(ctx context.Context, id string, categories map[int]string) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	var p authenticjobsPosting
	if err := s.http.GetJSON(ctx, authenticjobsBase+"/wp-json/wp/v2/job-listings/"+id, &p); err != nil {
		return Job{}, false, err
	}
	if p.Meta.Filled {
		return Job{}, false, nil
	}
	body := sanitizeHTML(html.UnescapeString(p.Content.Rendered))
	title := strings.TrimSpace(html.UnescapeString(p.Title.Rendered))
	if body == "" || title == "" {
		return Job{}, false, fmt.Errorf("authenticjobs: posting %s has no title or body", id)
	}
	location := strings.TrimSpace(p.Meta.Location)
	j := Job{
		ExternalID:     id,
		URL:            p.Link,
		Title:          title,
		Company:        strings.TrimSpace(html.UnescapeString(p.Meta.Company)),
		Location:       location,
		Description:    body,
		Remote:         bool(p.Meta.Remote) || isRemote(location),
		EmploymentType: authenticjobsEmploymentType(p.Classes),
		PostedAt:       parseLayout("2006-01-02T15:04:05", p.DateGMT),
	}
	if p.Meta.Remote {
		j.WorkMode = "remote"
	}
	for _, cat := range p.Categories {
		name, err := s.categoryName(ctx, cat, categories)
		if err == nil && authenticjobsTechCategories[name] {
			j.IsTechHint = true
		}
	}
	return j, true, nil
}

func authenticjobsEmploymentType(classes []string) string {
	for _, c := range classes {
		if slug, ok := strings.CutPrefix(c, "job-type-"); ok {
			if v, ok := authenticjobsEmploymentTypes[slug]; ok {
				return v
			}
		}
	}
	return ""
}

// categoryName resolves a job-category term id once per run. A failed lookup only withholds the
// technical hint, so it is not fatal to the posting.
func (s authenticjobs) categoryName(ctx context.Context, id int, cache map[int]string) (string, error) {
	if name, ok := cache[id]; ok {
		return name, nil
	}
	if err := s.limiter.Wait(ctx); err != nil {
		return "", err
	}
	var t struct {
		Name string `json:"name"`
	}
	if err := s.http.GetJSON(ctx, authenticjobsBase+"/wp-json/wp/v2/job-categories/"+strconv.Itoa(id), &t); err != nil {
		return "", err
	}
	name := html.UnescapeString(t.Name)
	cache[id] = name
	return name, nil
}
