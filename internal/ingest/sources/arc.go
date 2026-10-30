package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	neturl "net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"github.com/yuin/goldmark"
	gmhtml "github.com/yuin/goldmark/renderer/html"
	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// arc adapts Arc (arc.dev), a remote-jobs board. Besides a few dozen postings of its own it
// mirrors ~7.5k "external" remote postings — LinkedIn listings it re-publishes for about a
// month, each under its real employer — and those are what this crawls. Boardless and
// multi-company, like workatastartup.
//
// The complete list is session-gated. The public /remote-jobs pages render only the newest
// ~30 postings of a category with no pagination (658 category pages reach ~47% of the total),
// while the signed-in site's own search (SEARCH_EXTERNAL_JOBS in its bundle) pages through all
// of them: POST api.arc.dev/p/job-posts/search, once per job type, answering 401 to a request
// without the session cookie. So the crawl needs ARC_SESSION_COOKIE — the whole Cookie header
// of a signed-in browser session — and is registered only where it is set (see All).
//
// The listing carries title, employer, required countries, levels and date but no body; the
// body lives only on the public job page's __NEXT_DATA__. So this is a HydratingSource that
// buys a page only for postings the catalogue lacks, and CoverageGated because a large share
// re-lists employers freehire already crawls first-party (~40% measured 2026-10-30). The apply
// link on every sampled posting was a linkedin.com URL, so the stored URL is Arc's own page.
type arc struct {
	http    arcHTTP
	session string
}

// arcHTTP is what the adapter needs: the signed-in listing POST and the public job page,
// read with its final URL because Arc answers a posting it no longer carries with a redirect.
type arcHTTP interface {
	HeaderJSONPoster
	HTMLResolvedGetter
}

const (
	arcSessionEnv = "ARC_SESSION_COOKIE"
	arcSearchURL  = "https://api.arc.dev/p/job-posts/search"
	arcJobURL     = "https://arc.dev/remote-jobs/j/%s-%s"
	// arcJobPathPrefix is where every job page lives; a detail request that lands anywhere
	// else was redirected away from a posting Arc no longer carries.
	arcJobPathPrefix = "/remote-jobs/j/"

	// arcPageSize is served as asked; 100 keeps the full walk to ~76 requests.
	arcPageSize = 100
	// arcMaxPages is a ceiling ~13x the measured catalogue (72 full-time pages), never the
	// expected end. Reaching it fails the crawl: the provider is swept by source, so a
	// truncated walk must not pass.
	arcMaxPages = 1000
)

// arcJobTypes are the two listings the site's search splits the catalogue into; their stated
// totals summed to the catalogue's (7,148 + 358 = 7,506 on 2026-10-30).
var arcJobTypes = []string{"fulltime", "freelance"}

// One request a second through one limiter for the listing and the pages alike: the session is
// a person's account, and a crawl that hammers it is a crawl that gets it banned. A cold
// catalogue is ~7.5k pages, so one run hydrates at most arcDetailBudget of them and the rest
// accrete over the next runs instead of one run outliving its unit timeout and saving nothing.
// At ~250 new postings a day a warm daily run spends a small fraction of it.
const (
	arcRequestInterval = time.Second
	arcRequestBurst    = 1
	arcDetailWorkers   = 2
	arcDetailBudget    = 2000
)

// NewArc builds the Arc adapter over the given HTTP client and session cookie.
func NewArc(c arcHTTP, session string) Source { return arc{http: c, session: session} }

// rateLimitedArc gates both of the adapter's request kinds through one limiter.
type rateLimitedArc struct {
	inner   arcHTTP
	limiter waiter
}

func (p rateLimitedArc) PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error {
	if err := p.limiter.Wait(ctx); err != nil {
		return err
	}
	return p.inner.PostJSONWithHeaders(ctx, url, headers, body, v)
}

func (p rateLimitedArc) GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error) {
	if err := p.limiter.Wait(ctx); err != nil {
		return nil, "", err
	}
	return p.inner.GetHTMLResolved(ctx, url)
}

// pacedArc wraps a client with a fresh limiter shared across one registry build.
func pacedArc(c arcHTTP) arcHTTP {
	return rateLimitedArc{inner: c, limiter: rate.NewLimiter(rate.Every(arcRequestInterval), arcRequestBurst)}
}

func (arc) Provider() string { return "arc" }
func (arc) boardless()       {}
func (arc) aggregator()      {}

// arc lists its whole catalogue every run and fails any walk whose distinct postings fall
// short of the listing's own stated total, so a posting the run did not see is gone.
func (arc) fullCatalog() {}

// arcListing is one row of the signed-in search.
type arcListing struct {
	RandomKey         string   `json:"random_key"`
	Title             string   `json:"title"`
	JobType           string   `json:"job_type"`
	RequiredCountries []string `json:"required_countries"`
	ExperienceLevels  []string `json:"experience_levels"`
	URLString         string   `json:"url_string"`
	PostedAt          int64    `json:"posted_at"`
	Company           *struct {
		Name string `json:"name"`
	} `json:"company"`
}

type arcListPage struct {
	Total *int         `json:"total_job_count"`
	Jobs  []arcListing `json:"jobs"`
}

// arcNextData is the slice of a job page's __NEXT_DATA__ the adapter reads.
type arcNextData struct {
	Props struct {
		PageProps struct {
			Job *struct {
				RandomKey   string `json:"randomKey"`
				Description string `json:"description"`
				Closed      bool   `json:"closed"`
			} `json:"job"`
		} `json:"pageProps"`
	} `json:"props"`
}

func (s arc) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s arc) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, seen, nil, nil)
}

// FetchNewGated is FetchNew told which postings the pipeline will discard anyway, so a
// covered employer's pages are not bought only to be thrown away on every run.
func (s arc) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		names := make([]string, 0, len(rows))
		for _, r := range rows {
			names = append(names, r.company())
		}
		covered = gate.Covered(names)
	}
	return s.hydrate(ctx, rows, seen, covered, gate.Rejected)
}

// list walks every job type's listing to its end. Any page failing fails the crawl, and so does
// a walk whose distinct postings do not equal the listing's stated total: the provider is swept
// by source, and a short walk would close live postings.
func (s arc) list(ctx context.Context) ([]arcListing, error) {
	headers := map[string]string{
		"Cookie":           s.session,
		"Origin":           "https://arc.dev",
		"Referer":          "https://arc.dev/",
		"X-Requested-From": "arc-next",
	}
	var out []arcListing
	seen := map[string]bool{}
	for _, jobType := range arcJobTypes {
		total, distinct := -1, 0
		for page := 1; ; page++ {
			if page > arcMaxPages {
				return nil, fmt.Errorf("arc: %s listing did not end within %d pages", jobType, arcMaxPages)
			}
			body := map[string]any{"job_type": jobType, "page": page, "per_page": arcPageSize, "sort_by": "latest"}
			var p arcListPage
			if err := s.http.PostJSONWithHeaders(ctx, arcSearchURL, headers, body, &p); err != nil {
				var se *StatusError
				if errors.As(err, &se) && (se.Code == http.StatusUnauthorized || se.Code == http.StatusForbidden) {
					return nil, fmt.Errorf("arc: the session was refused; renew %s: %w", arcSessionEnv, err)
				}
				return nil, fmt.Errorf("arc: %s listing page %d: %w", jobType, page, err)
			}
			if p.Total == nil {
				return nil, errListingAbsent("arc", jobType, "total_job_count")
			}
			if total < 0 {
				total = *p.Total
			} else if *p.Total != total {
				return nil, fmt.Errorf("arc: %s listing total moved from %d to %d mid-walk", jobType, total, *p.Total)
			}
			for _, r := range p.Jobs {
				if r.RandomKey == "" || seen[r.RandomKey] {
					continue
				}
				seen[r.RandomKey] = true
				distinct++
				out = append(out, r)
			}
			if len(p.Jobs) == 0 || distinct >= total {
				break
			}
		}
		// Arc always lists full-time postings, so an empty one is a broken answer, not a quiet
		// month; the freelance listing is small enough that empty is merely unlikely.
		if total <= 0 && jobType == "fulltime" {
			return nil, fmt.Errorf("arc: %s listing states %d postings", jobType, total)
		}
		if distinct != total {
			return nil, fmt.Errorf("arc: %s listing read %d distinct postings of %d", jobType, distinct, total)
		}
	}
	return out, nil
}

// hydrate maps the listed rows to Jobs. A seen posting is a liveness refresh with no request; a
// posting the pipeline will discard — its employer covered, or its title rejected — is yielded
// body-less for the pipeline to count; every other posting costs one page, up to the run's
// budget. A page that could not be read defers its posting to a later run rather than storing it
// body-less, which would never be hydrated again.
func (s arc) hydrate(ctx context.Context, rows []arcListing, seen func(string) bool,
	covered map[string]bool, rejected func(Job) bool) ([]Job, error) {
	var budget, answered atomic.Int64
	jobs := fetchDetails(rows, arcDetailWorkers, func(r arcListing) (Job, bool) {
		base, ok := r.toJob()
		if !ok {
			return Job{}, false
		}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			return base, true
		}
		if covered[base.Company] || (rejected != nil && rejected(base)) {
			return base, true
		}
		if budget.Add(1) > arcDetailBudget {
			return Job{}, false
		}
		job, ok, err := s.withDetail(ctx, base)
		if err != nil {
			log.Printf("arc: detail %s failed; deferring: %v", base.ExternalID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that asked for pages and got no answer to any of them is broken, not quiet.
	// "Answered" counts a posting Arc reported gone, so a run whose only new rows are those passes.
	if wanted := min(budget.Load(), arcDetailBudget); wanted > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("arc: requested %d job pages and none answered", wanted)
	}
	if budget.Load() > arcDetailBudget {
		log.Printf("arc: detail budget %d spent; %d new postings deferred to later runs",
			arcDetailBudget, budget.Load()-arcDetailBudget)
	}
	return jobs, nil
}

// arcMarkdown renders a job page's body: Markdown (LinkedIn's HTML run through a converter)
// that kept its inline <strong> tags raw. The default renderer would replace those with a
// "raw HTML omitted" comment; letting them through is safe because sanitizeHTML runs after.
var arcMarkdown = goldmark.New(goldmark.WithRendererOptions(gmhtml.WithUnsafe()))

// arcStrongRun is a bold run with the whitespace at its edges. LinkedIn's bodies close the run
// after a paragraph break ("<strong>About X  \n  \n</strong>Text"), which Markdown renders as a
// paragraph closing inside the bold run; moving that whitespace outside the tags keeps the
// run within its paragraph.
var arcStrongRun = regexp.MustCompile(`(?s)<strong>(\s*)(.*?)(\s*)</strong>`)

// withDetail adds the job page's body. ok=false with a nil error is Arc's own answer that the
// posting is gone: a redirect away from the job page (what an unknown key gets, measured), a
// 404/410, or a page marking the posting closed. An error is the absence of an answer.
func (s arc) withDetail(ctx context.Context, base Job) (Job, bool, error) {
	root, final, err := s.http.GetHTMLResolved(ctx, base.URL)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if u, perr := neturl.Parse(final); perr == nil && !strings.HasPrefix(u.Path, arcJobPathPrefix) {
		return Job{}, false, nil
	}
	var nd arcNextData
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" || json.Unmarshal([]byte(raw), &nd) != nil {
		return Job{}, false, errors.New("job page carries no readable __NEXT_DATA__")
	}
	j := nd.Props.PageProps.Job
	if j == nil || j.RandomKey != base.ExternalID {
		return Job{}, false, errors.New("job page carries no posting for its key")
	}
	if j.Closed {
		return Job{}, false, nil
	}
	var b strings.Builder
	md := arcStrongRun.ReplaceAllString(j.Description, "$1<strong>$2</strong>$3")
	if err := arcMarkdown.Convert([]byte(md), &b); err != nil {
		return Job{}, false, fmt.Errorf("render body: %w", err)
	}
	body := sanitizeHTML(b.String())
	if strings.TrimSpace(body) == "" {
		return Job{}, false, errors.New("posting carries no body")
	}
	base.Description = body
	return base, true, nil
}

func (r arcListing) company() string {
	if r.Company == nil {
		return ""
	}
	return strings.TrimSpace(r.Company.Name)
}

// toJob maps a listing row to a body-less Job. A row with no employer is dropped: the catalogue
// cannot file it, and the coverage gate cannot judge it. Every posting is remote — that is the
// board's own scope, the listing being Arc's remote-jobs search.
func (r arcListing) toJob() (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := r.company()
	if r.RandomKey == "" || r.URLString == "" || title == "" || company == "" {
		return Job{}, false
	}
	countries, fallback := arcCountries(r.RequiredCountries)
	j := Job{
		ExternalID:        r.RandomKey,
		URL:               fmt.Sprintf(arcJobURL, r.URLString, r.RandomKey),
		Title:             title,
		Company:           company,
		WorkMode:          "remote",
		Remote:            true,
		Countries:         countries,
		FallbackCountries: fallback,
		Seniority:         arcSeniority(r.ExperienceLevels),
		EmploymentType:    arcEmploymentType(r.JobType),
	}
	if r.PostedAt > 0 {
		t := time.Unix(r.PostedAt, 0)
		j.PostedAt = NotFuture(&t)
	}
	return j, true
}

// arcStateCollisions are the country codes a US location can turn into: every state
// abbreviation that is also an ISO 3166 code (IL Israel, VA Vatican, GA Gabon, CA Canada…), plus
// GE and JE, which the sample shows standing for Georgia and New Jersey beside "US" (measured
// 2026-10-30: 866 of 4,300 postings listed "US" beside one of these).
var arcStateCollisions = func() map[string]bool {
	m := map[string]bool{"GE": true, "JE": true}
	for _, s := range strings.Fields("AL AR AZ CA CO DE GA ID IL IN KY LA MA MD ME MN MO MS MT NC NE PA SC TN VA") {
		m[s] = true
	}
	return m
}()

// arcCountries reads required_countries. Arc derives the list from LinkedIn's "City, ST,
// United States", so beside "US" a state abbreviation arrives as a country — ["US","IL"] is
// Illinois. Such a list cannot be told from a genuine "US and Canada", so it is demoted to the
// FallbackCountries signal and stripped of every code a state could have produced; a list
// without "US" is taken as stated.
func arcCountries(codes []string) (countries, fallback []string) {
	upper := make([]string, 0, len(codes))
	hasUS := false
	for _, c := range codes {
		c = strings.ToUpper(strings.TrimSpace(c))
		upper = append(upper, c)
		hasUS = hasUS || c == "US"
	}
	if !hasUS {
		return countriesFromCodes(upper), nil
	}
	var kept []string
	ambiguous := false
	for _, c := range upper {
		if arcStateCollisions[c] {
			ambiguous = true
			continue
		}
		kept = append(kept, c)
	}
	if !ambiguous {
		return countriesFromCodes(upper), nil
	}
	return nil, countriesFromCodes(kept)
}

// arcSeniority maps experience_levels to the seniority vocabulary. Role names that carry no
// level of their own (manager, director, architect, vp) state none, and a row ticking two
// different levels states none either.
func arcSeniority(levels []string) string {
	out := ""
	for _, l := range levels {
		var s string
		switch strings.ToLower(strings.TrimSpace(l)) {
		case "intern":
			s = "intern"
		case "junior":
			s = "junior"
		case "mid", "middle", "mid_level":
			s = "middle"
		case "senior":
			s = "senior"
		case "lead":
			s = "lead"
		case "staff":
			s = "staff"
		case "principal":
			s = "principal"
		case "cto":
			s = "c_level"
		default:
			return ""
		}
		if out != "" && out != s {
			return ""
		}
		out = s
	}
	return out
}

// arcEmploymentType maps the listing's job_type: the full-time search answers "permanent", the
// freelance one "contract".
func arcEmploymentType(jobType string) string {
	switch jobType {
	case "permanent":
		return "full_time"
	case "contract":
		return "contract"
	}
	return ""
}
