package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"
)

// taleez adapts Taleez (taleez.com), a French ATS whose customers — SMEs, staffing groups,
// healthcare networks, garages of the AD network — publish each opening at
// https://taleez.com/apply/<token>. There is no per-customer listing any more (the old
// taleez.com/careers/<slug> pages answer 404, and <slug>.taleez.com is wildcard DNS), so the
// adapter is boardless: the posting set is the job sitemap robots.txt advertises, and the
// employer is read per posting. robots.txt disallows /api/, /feeds/ and /widgets/, none of which
// is read.
//
// The sitemap carries only each posting's URL, oldest first, so every fact is bought with the
// posting page's schema.org JobPosting. The adapter hydrates only postings the catalogue lacks,
// newest first plus a moving backlog window (hydrationLanes), and refreshes the rest by identity.
//
// It is marked an aggregator because it serves many employers under one provider, which is what
// keeps it in the source facet; its URLs are the employers' own application pages all the same.
// It does NOT claim fullCatalog: nothing states how many postings the sitemap should hold, so a
// truncated sitemap could not be told from a shrunken catalogue.
type taleez struct {
	sitemap        XMLGetter
	pages          HTMLGetter
	limiter        waiter
	fresh, backlog int
	deadlineMargin time.Duration
	now            func() time.Time
}

const (
	taleezHost       = "taleez.com"
	taleezSitemapURL = "https://" + taleezHost + "/sitemap-job.xml"

	// taleezRequestInterval paces every posting page at one a second: a quick burst from one IP
	// drew a refused connection (2026-10-28), so this is a courtesy ceiling.
	taleezRequestInterval = time.Second
	taleezDetailWorkers   = 2
	// taleezFreshPerRun and taleezBacklogPerRun bound one run's page reads: 1,200 pages at one a
	// second is twenty minutes. The sitemap held 12,136 postings on 2026-10-28, so the first runs
	// take it in slices.
	taleezFreshPerRun    = 900
	taleezBacklogPerRun  = 300
	taleezDeadlineMargin = 5 * time.Minute
)

// NewTaleez builds the adapter: the sitemap is read over sitemap, the posting pages over pages.
func NewTaleez(sitemap XMLGetter, pages HTMLGetter) Source {
	return taleez{
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(taleezRequestInterval), 1),
		fresh:          taleezFreshPerRun,
		backlog:        taleezBacklogPerRun,
		deadlineMargin: taleezDeadlineMargin,
		now:            time.Now,
	}
}

func (taleez) Provider() string { return "taleez" }

// No per-customer listing exists; the one sitemap is the whole crawl.
func (taleez) boardless() {}

// Many employers publish through it, each named per posting.
func (taleez) aggregator() {}

// taleezToken is a posting's identity: the last path segment of /apply/<token>, either a short
// id ("fmudc") or a title slug.
var taleezToken = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9-]*$`)

// taleezPosting is one sitemap entry, pinned to https://taleez.com.
type taleezPosting struct {
	token string
	url   string
}

func taleezParsePosting(raw string) (taleezPosting, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.Host != taleezHost || u.RawQuery != "" {
		return taleezPosting{}, false
	}
	token, ok := strings.CutPrefix(u.Path, "/apply/")
	if !ok || !taleezToken.MatchString(token) {
		return taleezPosting{}, false
	}
	return taleezPosting{token: token, url: "https://" + taleezHost + "/apply/" + token}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s taleez) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the sitemap, refreshes the postings already stored, and reads a bounded slice
// of the rest. A posting whose page cannot be read is not yielded; a later run tries again.
func (s taleez) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []taleezPosting
	for _, p := range postings {
		if seen(p.token) {
			jobs = append(jobs, Job{ExternalID: p.token, URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := hydrationLanes(unseen, s.fresh, s.backlog, s.now())
	var answered, outOfTime atomic.Int64
	read := fetchDetails(picked, taleezDetailWorkers, func(p taleezPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("taleez: posting %s failed; deferring: %v", p.token, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(picked)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("taleez: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("taleez: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if n := len(unseen) - len(picked); n > 0 {
		log.Printf("taleez: %d new postings past this run's bound of %d, deferred to later runs", n, s.fresh+s.backlog)
	}
	return append(jobs, read...), nil
}

// list reads the job sitemap, newest first (the sitemap lists postings oldest first). It failing
// fails the crawl; so does a sitemap that lists no posting at all.
func (s taleez) list(ctx context.Context) ([]taleezPosting, error) {
	doc, err := getSitemap(ctx, s.sitemap, taleezSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("taleez: sitemap: %w", err)
	}
	tokens := map[string]bool{}
	var out []taleezPosting
	for _, u := range doc.URLs {
		p, ok := taleezParsePosting(u.Loc)
		if !ok || tokens[p.token] {
			continue
		}
		tokens[p.token] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("taleez: sitemap lists no posting")
	}
	slices.Reverse(out)
	return out, nil
}

// taleezLD is the part of the posting page's schema.org JobPosting the adapter reads.
type taleezLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	Qualifications     string          `json:"qualifications"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	JobLocation        schemaPlaces    `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// errTaleezNoPosting marks a page that answered 200 without a JobPosting. The site answers a
// removed posting with a 404, so this is not a "gone" answer — an interstitial looks the same —
// and counts as no answer.
var errTaleezNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410); an error is the absence of an answer.
func (s taleez) detail(ctx context.Context, p taleezPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.pages.GetHTML(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld taleezLD
	if !ldJobPosting(root, &ld) {
		return Job{}, false, errTaleezNoPosting
	}
	job, ok := taleezJob(ld, p)
	return job, ok, nil
}

// taleezJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer or
// body.
func taleezJob(ld taleezLD, p taleezPosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description + headedSection("Qualifications", ld.Qualifications))
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     p.token,
		URL:            p.url,
		Title:          title,
		Company:        company,
		Description:    body,
		PostedAt:       parseRFC3339OrDate(strings.TrimSpace(ld.DatePosted)),
		EmploymentType: taleezEmploymentType(ld.EmploymentType),
	}
	var places []string
	for _, pl := range ld.JobLocation {
		if loc := joinNonEmpty(pl.Address.AddressLocality, pl.Address.AddressRegion); loc != "" && !slices.Contains(places, loc) {
			places = append(places, loc)
		}
		for _, c := range countryFromCode(pl.Address.AddressCountry) {
			if !slices.Contains(j.Countries, c) {
				j.Countries = append(j.Countries, c)
			}
		}
	}
	j.Location = strings.Join(places, "; ")
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	return j, true
}

// taleezEmploymentType reads employmentType, a list. TEMPORARY is a French fixed-term contract
// (CDD) — an employment, not contract work — so it states nothing on its own; the first value
// that does decide wins.
func taleezEmploymentType(raw json.RawMessage) string {
	var types []string
	if err := json.Unmarshal(raw, &types); err != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		types = []string{one}
	}
	for _, t := range types {
		if strings.EqualFold(strings.TrimSpace(t), "TEMPORARY") {
			continue
		}
		if et := schemaEmploymentType(t); et != "" {
			return et
		}
	}
	return ""
}
