package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// skipthedrive adapts skipthedrive.com, a WordPress remote-jobs aggregator. Boardless and
// multi-company, so it stays in the source facet and takes each posting's company from the
// listing. Every posting is remote. See the "SkipTheDrive traps" in providers/skipthedrive.md.
//
// There is no feed or REST route a plain client can reach, so the listing is the category
// pages themselves (paginated /job-category/<slug>/page/N/), whose slugs come from the sitemap.
// A listing row carries title, company and link but no body; each posting page carries the body,
// the location and the link out to the employer's own ATS, so the adapter hydrates only the
// postings the catalogue lacks, newest first plus a moving backlog window (hydrationLanes).
type skipthedrive struct {
	http skipthedriveHTTP
	now  func() time.Time
}

type skipthedriveHTTP interface {
	TextGetter
	HTMLGetter
}

const (
	skipthedriveBase       = "https://www.skipthedrive.com"
	skipthedriveSitemapURL = skipthedriveBase + "/sitemap.xml"
	// The measured longest category is 33 pages of 20; the cap only stops a runaway walk.
	skipthedriveMaxPages = 60
	// One run's detail budget: at 2 workers this fits the default run, and the initial ~6,700
	// backlog drains over several runs.
	skipthedriveFreshPerRun   = 300
	skipthedriveBacklogPerRun = 300
	skipthedriveDetailWorkers = 2
)

var (
	skipthedriveCategoryRE = regexp.MustCompile(`/job-category/([a-z0-9-]+)/`)
	skipthedriveIDRE       = regexp.MustCompile(`-(\d+)/?$`)
)

// skipthedriveRequestInterval paces every request of a run, listing and posting pages alike: an
// unpaced walk of the ~23 categories (up to 33 pages each) drew 429s partway through. One a
// second keeps a full run (~700 listing pages plus 600 posting pages) inside the unit's timeout.
const skipthedriveRequestInterval = time.Second

// pacedSkipTheDrive routes the sitemap, listing and posting requests through one limiter.
func pacedSkipTheDrive(c skipthedriveHTTP) skipthedriveHTTP {
	limiter := rate.NewLimiter(rate.Every(skipthedriveRequestInterval), 1)
	return struct {
		TextGetter
		HTMLGetter
	}{
		rateLimitedTextGetter{inner: c, limiter: limiter},
		rateLimitedHTMLGetter{inner: c, limiter: limiter},
	}
}

// NewSkipTheDrive builds the SkipTheDrive adapter over the given HTTP client; production paces
// it (pacedSkipTheDrive).
func NewSkipTheDrive(c skipthedriveHTTP) Source { return skipthedrive{http: c, now: time.Now} }

func (skipthedrive) Provider() string { return "skipthedrive" }

func (skipthedrive) Boardless() {}

func (skipthedrive) Aggregator() {}

type skipthedrivePosting struct {
	id, url, title, company string
}

func (p skipthedrivePosting) toJob() Job {
	return Job{
		ExternalID: p.id,
		URL:        p.url,
		Title:      p.title,
		Company:    p.company,
		Remote:     true,
		WorkMode:   "remote",
	}
}

// Fetch is the list-only fallback HydratingSource specifies: every posting, no body.
func (s skipthedrive) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	postings, err := s.listing(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(postings))
	for _, p := range postings {
		jobs = append(jobs, p.toJob())
	}
	return jobs, nil
}

// FetchNew refreshes a stored posting by identity (no request, no content rewrite) and buys the
// page of an unseen one.
func (s skipthedrive) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.listing(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []skipthedrivePosting
	for _, p := range postings {
		if seen(p.id) {
			base := p.toJob()
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, skipthedriveFreshPerRun, skipthedriveBacklogPerRun, s.now())
	var attempted, read atomic.Int64
	hydrated := fetchDetails(picked, skipthedriveDetailWorkers, func(p skipthedrivePosting) (Job, bool) {
		job := p.toJob()
		attempted.Add(1)
		root, err := s.http.GetHTML(ctx, p.url)
		if err != nil {
			// The listing is authoritative for the posting existing, so a failed body keeps it;
			// pipeline.HydrationRetryWindow re-offers the row for hydration later.
			log.Printf("skipthedrive: body %s failed; ingesting list-only: %v", p.id, err)
			return job, true
		}
		if !skipthedriveApplyDetail(root, &job) {
			log.Printf("skipthedrive: body %s unreadable; ingesting list-only", p.id)
			return job, true
		}
		read.Add(1)
		return job, true
	})
	// Pages were due and none could be read: the markup changed or a wall answers 200, which a
	// list-only success would report as a healthy crawl (the echojobs#2588 lesson).
	if attempted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("skipthedrive: fetched %d posting pages and read none of them", attempted.Load())
	}
	if n := len(unseen) - len(picked); n > 0 {
		log.Printf("skipthedrive: %d new postings past this run's bound, deferred to later runs", n)
	}
	return append(jobs, hydrated...), nil
}

// skipthedriveApplyDetail fills the posting's body, location, posting date and the employer's
// own apply link from its page, reporting whether a body was found.
func skipthedriveApplyDetail(root *html.Node, job *Job) bool {
	section := firstByID(root, "skipthedrive_section")
	if section == nil {
		return false
	}
	body := sanitizeHTML(innerHTML(section))
	if strings.TrimSpace(body) == "" {
		return false
	}
	job.Description = body
	job.PostedAt = parseRFC3339(metaProperty(root, "article:published_time"))
	if marker := firstByClass(root, "fa-map-marker"); marker != nil && marker.Parent != nil {
		job.Location = strings.TrimSpace(textContent(marker.Parent))
	}
	if box := firstByClass(root, "apply_to_position_div"); box != nil {
		walk(box, func(n *html.Node) bool {
			if n.Type == html.ElementNode && n.Data == "a" && job.ExternalApplyURL == "" {
				if href := Attr(n, "href"); strings.HasPrefix(href, "http") {
					job.ApplyMethod, job.ExternalApplyURL = ApplyExternal, href
				}
			}
			return true
		})
	}
	return true
}

// listing walks every category's pages and returns each posting once, in the order first met.
// A category whose first page fails fails the crawl: an absent category would otherwise read as
// its postings having left.
func (s skipthedrive) listing(ctx context.Context) ([]skipthedrivePosting, error) {
	sitemap, err := s.http.GetText(ctx, skipthedriveSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("skipthedrive: sitemap: %w", err)
	}
	var slugs []string
	seenSlug := map[string]bool{}
	for _, m := range skipthedriveCategoryRE.FindAllStringSubmatch(sitemap, -1) {
		if !seenSlug[m[1]] {
			seenSlug[m[1]] = true
			slugs = append(slugs, m[1])
		}
	}
	if len(slugs) == 0 {
		return nil, fmt.Errorf("skipthedrive: sitemap lists no job category")
	}
	var out []skipthedrivePosting
	seenID := map[string]bool{}
	for _, slug := range slugs {
		_, err := crawlPagedLinks(ctx, s.http, skipthedriveMaxPages,
			func(page int) string {
				if page == 1 {
					return skipthedriveBase + "/job-category/" + slug + "/"
				}
				return fmt.Sprintf("%s/job-category/%s/page/%d/", skipthedriveBase, slug, page)
			},
			func(root *html.Node) []string {
				var links []string
				for _, p := range skipthedrivePageRows(root) {
					links = append(links, p.url)
					if !seenID[p.id] {
						seenID[p.id] = true
						out = append(out, p)
					}
				}
				return links
			})
		if err != nil {
			return nil, fmt.Errorf("skipthedrive: category %s: %w", slug, err)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("skipthedrive: no posting in any category")
	}
	return out, nil
}

// skipthedrivePageRows reads the usable rows of one listing page: a /job/<slug>-<id>/ link, its
// title and its company (a row with no company would break the public slug).
func skipthedrivePageRows(root *html.Node) []skipthedrivePosting {
	var rows []skipthedrivePosting
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !HasClass(n, "post-content") {
			return true
		}
		title := firstByClass(n, "post-title")
		company := firstByClass(n, "custom_fields_company_name_display_search_results")
		if title == nil || company == nil {
			return false
		}
		var href string
		walk(title, func(a *html.Node) bool {
			if a.Type == html.ElementNode && a.Data == "a" && href == "" {
				href = Attr(a, "href")
			}
			return true
		})
		u, err := url.Parse(href)
		m := skipthedriveIDRE.FindStringSubmatch(href)
		name := strings.Join(strings.Fields(textContent(company)), " ") // the span leads with an &nbsp;
		if err == nil && m != nil && strings.HasPrefix(u.Path, "/job/") && name != "" {
			rows = append(rows, skipthedrivePosting{id: m[1], url: href, title: strings.TrimSpace(textContent(title)), company: name})
		}
		return false
	})
	return rows
}
