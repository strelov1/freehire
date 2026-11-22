package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"math"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// cvbankas adapts CVbankas.lt, Lithuania's largest general job board, every trade, whose
// postings each name their own employer. Boardless aggregator: one crawl covers the whole
// catalogue (~7,200 open postings on 2026-11-22).
//
// The listing is the site's own paged front page, /?page=N, 50 postings a page, newest first;
// each row carries the posting's id, URL, title, employer, place and salary, but no body. The
// sitemaps answer Cloudflare's 403 from the crawl host while the listing and the posting pages
// are served to a plain client, so this is a HydratingSource that reads a posting's page only
// when the catalogue lacks it, up to cvbankasDetailBudget a run. The page carries a schema.org
// microdata JobPosting (no ld+json): the body is its description section, the date its
// datePosted. robots.txt disallows only account and candidate-action routes. See
// providers/cvbankas.md.
type cvbankas struct {
	http         HTMLGetter
	maxNewPerRun int
}

const (
	cvbankasBase     = "https://www.cvbankas.lt"
	cvbankasPageSize = 50
	// cvbankasMaxPages is a ceiling, not an end: the listing's deepest page today is 144, and a walk
	// that reaches the ceiling without a page of nothing new has not seen the catalogue's end.
	cvbankasMaxPages = 400
	// cvbankasRequestInterval paces the whole run at one request a second, a courtesy ceiling:
	// a posting page is ~400 KB.
	cvbankasRequestInterval = time.Second
	cvbankasDetailWorkers   = 2
	// cvbankasDetailBudget bounds one run's page reads: 1,000 at a second each is ~17 minutes
	// beside the ~145 listing requests. A cold catalogue drains in about seven runs; a day's
	// intake is a few hundred postings.
	cvbankasDetailBudget = 1000
)

// NewCVbankas builds the CVbankas adapter over the given HTML client.
func NewCVbankas(c HTMLGetter) Source {
	return cvbankas{http: c, maxNewPerRun: cvbankasDetailBudget}
}

// pacedCVbankas holds the listing and the page reads of a run to one request a second.
func pacedCVbankas(c HTMLGetter) Source {
	return NewCVbankas(pacedHTMLGetter(c, cvbankasRequestInterval, 1))
}

func (cvbankas) Provider() string { return "cvbankas" }

// boardless: one crawl covers the whole national catalogue.
func (cvbankas) Boardless() {}

// Every posting names its own employer, so CVbankas stays in the source facet and in the
// cross-source duplicate suppression set.
func (cvbankas) Aggregator() {}

// cvbankasRow is one listing row.
type cvbankasRow struct {
	id      string
	url     string
	title   string
	company string
	cities  []string
	salary  cvbankasSalary
}

// cvbankasSalary is the row's salary. Only a gross figure is kept: the site also states pay after
// tax, which the Job has no way to mark.
type cvbankasSalary struct {
	min, max int
	period   string
}

func (s cvbankas) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew walks the listing, refreshes the stored postings by identity and reads up to
// maxNewPerRun of the rest, newest first. An unreadable page retains its listing identity.
func (s cvbankas) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []cvbankasRow
	deferred := 0
	for _, r := range rows {
		if seen(r.id) {
			jobs = append(jobs, Job{ExternalID: r.id, URL: r.url, SeenRefresh: true})
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, r)
	}
	var answered atomic.Int64
	read := fetchDetails(unseen, cvbankasDetailWorkers, func(r cvbankasRow) (Job, bool) {
		job, ok, err := s.detail(ctx, r)
		if err != nil {
			log.Printf("cvbankas: posting %s failed; deferring: %v", r.id, err)
			return unreadableDetail(r.id, r.url, r.company), true
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet.
	if len(unseen) > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("cvbankas: requested %d posting pages and none answered", len(unseen))
	}
	if deferred > 0 {
		log.Printf("cvbankas: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list walks /?page=N until a page names nothing new. The site answers a page past the end with a
// redirect to an earlier page, which names nothing new either. The first page failing fails the
// crawl; a later page failing ends the walk with the rows gathered.
func (s cvbankas) list(ctx context.Context) ([]cvbankasRow, error) {
	var out []cvbankasRow
	seen := map[string]bool{}
	for page := 1; page <= cvbankasMaxPages; page++ {
		root, err := s.http.GetHTML(ctx, fmt.Sprintf("%s/?page=%d", cvbankasBase, page))
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("cvbankas: listing: %w", err)
			}
			log.Printf("cvbankas: listing page %d failed; ending the walk: %v", page, err)
			return out, nil
		}
		added := 0
		for _, r := range cvbankasRows(root) {
			if seen[r.id] {
				continue
			}
			seen[r.id] = true
			out = append(out, r)
			added++
		}
		if added == 0 {
			if page == 1 {
				return nil, errors.New("cvbankas: the listing names no posting")
			}
			return out, nil
		}
	}
	return nil, fmt.Errorf("cvbankas: listing did not end within %d pages", cvbankasMaxPages)
}

func textByClass(n *html.Node, class string) string {
	if c := firstByClass(n, class); c != nil {
		return collapseSpace(textContent(c))
	}
	return ""
}

var cvbankasPostingPath = regexp.MustCompile(`/1-(\d+)$`)

// cvbankasRows reads every posting row of a listing page.
func cvbankasRows(root *html.Node) []cvbankasRow {
	var rows []cvbankasRow
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "article" || !HasClass(n, "list_article") {
			return true
		}
		a := firstByClass(n, "list_a")
		if a == nil {
			return false
		}
		href := Attr(a, "href")
		if !strings.HasPrefix(href, cvbankasBase+"/") {
			return false
		}
		m := cvbankasPostingPath.FindStringSubmatch(href)
		if m == nil {
			return false
		}
		r := cvbankasRow{
			id:    m[1],
			url:   href,
			title: textByClass(n, "list_h3"),
		}
		if hs := firstByClass(n, "heading_secondary"); hs != nil {
			r.company = textByClass(hs, "dib")
		}
		walk(n, func(c *html.Node) bool {
			if HasClass(c, "list_city") {
				if t := collapseSpace(textContent(c)); t != "" {
					r.cities = append(r.cities, t)
				}
			}
			return true
		})
		r.salary = cvbankasParseSalary(textByClass(n, "salary_amount"), textByClass(n, "salary_period"),
			textByClass(n, "salary_calculation"))
		rows = append(rows, r)
		return false
	})
	return rows
}

// cvbankasParseSalary reads the row's amount ("2150-2480", "Nuo 1800", "6,50-8"), period ("€/mėn.",
// "€/val.") and tax note. A figure after tax ("Į rankas") or in another currency is not kept.
func cvbankasParseSalary(amount, period, tax string) cvbankasSalary {
	var s cvbankasSalary
	if !strings.HasPrefix(period, "€/") || !strings.HasPrefix(tax, "Neatskaičius") {
		return s
	}
	switch strings.TrimPrefix(period, "€/") {
	case "mėn.":
		s.period = "month"
	case "val.":
		s.period = "hour"
	default:
		return cvbankasSalary{}
	}
	amount = strings.TrimSpace(strings.TrimPrefix(amount, "Nuo"))
	lo, hi, _ := strings.Cut(amount, "-")
	min, ok := cvbankasNumber(lo)
	if !ok {
		return cvbankasSalary{}
	}
	s.min, s.max = min, min
	if hi != "" {
		if max, ok := cvbankasNumber(hi); ok && max >= min {
			s.max = max
		}
	}
	return s
}

// cvbankasNumber reads a figure with a decimal comma, rounded to a whole unit.
func cvbankasNumber(s string) (int, bool) {
	f, err := strconv.ParseFloat(strings.ReplaceAll(strings.TrimSpace(s), ",", "."), 64)
	if err != nil || f < 1 {
		return 0, false
	}
	return int(math.Round(f)), true
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410). An unreadable or incomplete posting yields an error.
func (s cvbankas) detail(ctx context.Context, r cvbankasRow) (Job, bool, error) {
	root, err := s.http.GetHTML(ctx, r.url)
	if err != nil {
		if !detailUnreadable(err) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	job, ok := cvbankasJob(root, r)
	if !ok {
		return Job{}, false, errors.New("page carries no usable posting")
	}
	return job, true, nil
}

// cvbankasJob maps a posting page and its listing row to a Job. ok is false for one with no
// title, employer or body.
func cvbankasJob(root *html.Node, r cvbankasRow) (Job, bool) {
	title := r.title
	if h := findItemprops(root, "title"); len(h) > 0 {
		title = collapseSpace(textContent(h[0]))
	}
	company := r.company
	if org := findItemprops(root, "hiringOrganization"); len(org) > 0 {
		for _, n := range findItemprops(org[0], "name") {
			if v := collapseSpace(Attr(n, "content")); v != "" {
				company = v
				break
			}
		}
	}
	var body string
	if d := findItemprops(root, "description"); len(d) > 0 {
		body = sanitizeHTML(innerHTML(d[0]))
	}
	if title == "" || company == "" || strings.TrimSpace(textFromHTML(body)) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        r.id,
		URL:               r.url,
		Title:             title,
		Company:           company,
		Description:       body,
		Location:          cvbankasPlaces(root, r),
		FallbackCountries: []string{"lt"},
	}
	for _, n := range findItemprops(root, "datePosted") {
		if t := parseRFC3339OrDate(strings.TrimSpace(Attr(n, "content"))); t != nil {
			j.PostedAt = t
			break
		}
	}
	if r.salary.period != "" {
		lo, hi := r.salary.min, r.salary.max
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency, j.SalaryPeriod = "EUR", r.salary.period
	}
	return j, true
}

// cvbankasPlaces names the posting's places from its jobLocation (nominative: "Vilnius"), falling
// back to the listing's own spelling (locative: "Vilniuje").
func cvbankasPlaces(root *html.Node, r cvbankasRow) string {
	var out []string
	if loc := findItemprops(root, "jobLocation"); len(loc) > 0 {
		for _, n := range findItemprops(loc[0], "addressLocality") {
			if v := collapseSpace(textContent(n)); v != "" && !slices.Contains(out, v) {
				out = append(out, v)
			}
		}
	}
	if len(out) == 0 {
		out = r.cities
	}
	return strings.Join(out, "; ")
}
