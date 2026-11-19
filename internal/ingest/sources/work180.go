package sources

import (
	"bytes"
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// work180 adapts WORK180 (work180.com), a job board that lists only the postings of employers it
// has endorsed as supporting women at work. It is an aggregator: the postings are re-listings of
// the employers' own ATS vacancies (Workday, SuccessFactors, PageUp, Ashby, …), each naming the
// employer's application URL.
//
// A board is one of WORK180's three regional sites — au (en-au, which also carries New Zealand),
// uk (en-gb) and us (en-us). The board is what selects the slice to crawl, so the provider is
// board-based and not boardless; the three board names are distinct, so a board-scoped close
// never has to tell one region's postings from another's (the region column stays empty).
//
// The listing is the region's jobs sitemap: it names every posting URL, and with it the posting's
// numeric id (the external id, unique across the regions) and the employer's slug. The site's own
// search cannot serve as a listing — it answers at most 100 results (10 pages of 10) whatever
// the catalogue size. Each posting page server-renders a schema.org JobPosting (title, body,
// employer, places, work arrangement, employment type) and, in its React Server Components
// payload, the employer's application link; so this is a HydratingSource that buys a page only
// for a posting the catalogue lacks.
//
// The adapter does NOT claim fullBoardListing or fullCatalog: the sitemap is a snapshot, not a
// proven bar. See the WORK180 traps in AGENTS.md.
type work180 struct {
	sitemap XMLGetter  // the region's jobs sitemap (one request per board per run)
	pages   HTMLGetter // posting pages, paced to the site's Crawl-delay
	// maxNewPerBoard bounds how many posting pages one board's crawl hydrates. Anything past it
	// is deferred — not yielded, so not stored — and is offered again by a later run.
	maxNewPerBoard int
}

const work180Base = "https://work180.com"

// work180Locales maps a board to the locale its site lives under. au/uk/us are the API's own
// Region enum values (the listing payload states "region":"AU"/"UK"/"US"); the UK site's locale
// is en-gb.
var work180Locales = map[string]string{
	"au": "en-au",
	"uk": "en-gb",
	"us": "en-us",
}

// robots.txt declares "Crawl-delay: 5" for every user agent, and the catalogue is small enough
// to honour it literally: the steady state is the day's new postings, and the first run's
// backlog accretes over several runs under work180MaxNewPerBoard. Every posting page goes through
// this one limiter, across boards. The three sitemap requests a run makes are not paced.
const (
	work180RequestInterval = 5 * time.Second
	work180RequestBurst    = 1
)

// work180MaxNewPerBoard caps one board's hydration in one run (120 pages a board, 360 a run, is
// 30 minutes at the Crawl-delay pace). The newest postings go first, so a run that stops at the
// cap still takes the day's new postings before the backlog.
const work180MaxNewPerBoard = 120

// NewWORK180 builds the WORK180 adapter: sitemap reads the jobs sitemaps, pages the posting pages
// (which must be paced — see work180RequestInterval).
func NewWORK180(sitemap XMLGetter, pages HTMLGetter) Source {
	return work180{sitemap: sitemap, pages: pages, maxNewPerBoard: work180MaxNewPerBoard}
}

func (work180) Provider() string { return "work180" }
func (work180) Aggregator()      {}

// work180Listing is what a sitemap entry states about one posting.
type work180Listing struct {
	ID           int64
	URL          string // the posting page on work180.com
	EmployerSlug string
}

func (l work180Listing) externalID() string { return strconv.FormatInt(l.ID, 10) }

func (s work180) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s work180) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, list, seen, nil)
}

// FetchNewGated is FetchNew told which employers the pipeline is going to discard as already
// crawled first-party. Such a posting is never stored, so never seen, so without the gate every
// run would buy its page again, out of a per-run cap the genuinely new backlog needs. A covered
// posting is still yielded, body-less, so the pipeline makes and counts the decision.
//
// The sitemap states no employer NAME, only the employer's slug on WORK180, so the slug is what
// the coverage probe is asked about and what a covered posting carries as its company — the same
// string, so the pipeline's own gate reaches the same answer. A slug that spells the employer
// differently from freehire's company key (ernst-and-young for EY) only reads as uncovered,
// which costs the page and never a posting the catalogue needed.
func (s work180) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	list, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	var slugs []string
	for _, l := range list {
		if !seen(l.externalID()) && !slices.Contains(slugs, l.EmployerSlug) {
			slugs = append(slugs, l.EmployerSlug)
		}
	}
	var covered map[string]bool
	if len(slugs) > 0 && gate.Covered != nil {
		covered = gate.Covered(slugs)
	}
	return s.hydrate(ctx, list, seen, covered)
}

// work180JobPath matches a posting page: /<locale>/for-women/employer/<employer>/job/<id>/<slug>.
var work180JobPath = regexp.MustCompile(`^/(en-[a-z]{2})/for-women/employer/([^/]+)/job/([0-9]+)(?:/[^/]*)?/?$`)

// list reads the board's jobs sitemap and keeps its posting URLs, newest first. The sitemap also
// names the region's search category pages, which are not postings and are skipped. A sitemap
// with no posting at all is a failure, never an empty region: each region lists thousands.
func (s work180) list(ctx context.Context, board string) ([]work180Listing, error) {
	locale, ok := work180Locales[board]
	if !ok {
		return nil, fmt.Errorf("work180: unknown board %q (want au, uk or us)", board)
	}
	doc, err := getSitemap(ctx, s.sitemap, work180Base+"/"+locale+"/jobs-sitemap.xml")
	if err != nil {
		return nil, fmt.Errorf("work180: %s jobs sitemap: %w", board, err)
	}
	ids := map[int64]bool{}
	var out []work180Listing
	for _, u := range doc.URLs {
		l, ok := parseWORK180JobURL(u.Loc, locale)
		if !ok || ids[l.ID] {
			continue
		}
		ids[l.ID] = true
		out = append(out, l)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("work180: %s jobs sitemap names no posting", board)
	}
	slices.SortFunc(out, func(a, b work180Listing) int { return cmp.Compare(b.ID, a.ID) })
	return out, nil
}

// parseWORK180JobURL accepts only a posting URL on work180.com under the board's own locale.
func parseWORK180JobURL(raw, locale string) (work180Listing, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || !strings.EqualFold(u.Hostname(), "work180.com") {
		return work180Listing{}, false
	}
	m := work180JobPath.FindStringSubmatch(u.Path)
	if m == nil || m[1] != locale {
		return work180Listing{}, false
	}
	id, err := strconv.ParseInt(m[3], 10, 64)
	if err != nil || id <= 0 {
		return work180Listing{}, false
	}
	return work180Listing{ID: id, URL: work180Base + u.Path, EmployerSlug: m[2]}, true
}

// hydrate yields a seen posting as a liveness refresh, a covered one body-less, and buys the page
// of every other one up to the board's cap. A page that could not be read defers its posting
// rather than storing it body-less (a stored row is seen and would never be hydrated again).
// Once the edge refuses one request (403/429) the rest of the run's hydration is deferred too:
// the refusal is about the IP, and every further request would only prolong it.
func (s work180) hydrate(ctx context.Context, list []work180Listing, seen func(string) bool, covered map[string]bool) ([]Job, error) {
	var jobs []Job
	var fresh []work180Listing // in the listing's newest-first order, so the cap keeps the newest
	for _, l := range list {
		id := l.externalID()
		switch {
		case seen(id):
			jobs = append(jobs, Job{ExternalID: id, URL: l.URL, SeenRefresh: true})
		case covered[l.EmployerSlug]:
			jobs = append(jobs, Job{ExternalID: id, URL: l.URL, Company: l.EmployerSlug})
		default:
			fresh = append(fresh, l)
		}
	}
	if len(fresh) > s.maxNewPerBoard {
		log.Printf("work180: %d new postings, hydrating the newest %d this run and deferring the rest",
			len(fresh), s.maxNewPerBoard)
		fresh = fresh[:s.maxNewPerBoard]
	}

	var refused atomic.Bool
	var wanted, read atomic.Int64
	hydrated := fetchDetails(fresh, defaultDetailWorkers, func(l work180Listing) (Job, bool) {
		wanted.Add(1)
		if refused.Load() {
			return Job{}, false
		}
		root, err := s.pages.GetHTML(ctx, l.URL)
		if err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("work180: posting pages refused (%v); deferring the rest of this run's hydration", err)
			}
			if detailUnreadable(err) {
				log.Printf("work180: detail %s failed; deferring: %v", l.URL, err)
			}
			return Job{}, false
		}
		job, ok := parseWORK180Posting(root, l)
		if !ok {
			log.Printf("work180: detail %s carries no usable live JobPosting; deferring", l.URL)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("work180: listed %d new postings and read none of their pages", wanted.Load())
	}
	return append(jobs, hydrated...), nil
}

// work180Posting is the subset of the posting page's schema.org JobPosting the adapter maps.
type work180Posting struct {
	Identifier struct {
		Value string `json:"value"`
	} `json:"identifier"`
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation     work180Places   `json:"jobLocation"`
	JobLocationType string          `json:"jobLocationType"`
	EmploymentType  json.RawMessage `json:"employmentType"`
}

// work180Place is one jobLocation entry. The address carries a display "name" ("Sydney, NSW,
// Australia") beside the parts, and the parts omit the country, so the name is the location.
type work180Place struct {
	Address struct {
		Name            string `json:"name"`
		AddressLocality string `json:"addressLocality"`
		AddressRegion   string `json:"addressRegion"`
	} `json:"address"`
}

// work180Places decodes jobLocation as either one Place or an array of them (schema.org permits
// both; the site emits an array, empty for a remote posting).
type work180Places []work180Place

func (p *work180Places) UnmarshalJSON(b []byte) error {
	b = bytes.TrimSpace(b)
	if len(b) > 0 && b[0] == '[' {
		var arr []work180Place
		if err := json.Unmarshal(b, &arr); err != nil {
			return err
		}
		*p = arr
		return nil
	}
	var one work180Place
	if err := json.Unmarshal(b, &one); err != nil {
		return err
	}
	*p = work180Places{one}
	return nil
}

func (p work180Places) location() string {
	var parts []string
	for _, pl := range p {
		name := firstNonEmpty(strings.TrimSpace(pl.Address.Name), joinNonEmpty(pl.Address.AddressLocality, pl.Address.AddressRegion))
		if name != "" && !slices.Contains(parts, name) {
			parts = append(parts, name)
		}
	}
	return strings.Join(parts, "; ")
}

// work180Job is the posting's own record from the page's RSC payload — what the JobPosting does
// not carry: whether it has expired, and where the employer takes applications.
type work180Job struct {
	ID              string  `json:"id"`
	Expired         *string `json:"expired"`
	ApplicationType string  `json:"applicationType"`
	ApplicationLink string  `json:"applicationLink"`
	Employer        struct {
		Name string `json:"name"`
	} `json:"employer"`
}

func parseWORK180Posting(root *html.Node, l work180Listing) (Job, bool) {
	var p work180Posting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	id := l.externalID()
	// The page's JobPosting must be the posting the sitemap named; a route that started rendering
	// another posting must not be stored under this id.
	if strings.TrimSpace(p.Identifier.Value) != id {
		return Job{}, false
	}
	rec, hasRec := work180RSCJob(root, id)
	if hasRec && rec.Expired != nil {
		return Job{}, false // the page states the posting has closed
	}
	title := strings.TrimSpace(p.Title)
	company := strings.TrimSpace(firstNonEmpty(p.HiringOrganization.Name, rec.Employer.Name))
	description := sanitizeHTML(p.Description)
	if title == "" || company == "" || strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}

	applyURL := l.URL
	// An EXTERNAL application names the employer's own ATS posting; an EMAIL one names a
	// mail-relay link, which is not a posting URL, so the WORK180 page stays the URL.
	if strings.EqualFold(rec.ApplicationType, "EXTERNAL") && validHTTPURL(rec.ApplicationLink) {
		applyURL = strings.TrimSpace(rec.ApplicationLink)
	}

	job := Job{
		ExternalID:     id,
		URL:            applyURL,
		Title:          title,
		Company:        company,
		Location:       p.JobLocation.location(),
		Description:    description,
		PostedAt:       parseRFC3339(p.DatePosted),
		EmploymentType: work180EmploymentType(p.EmploymentType),
	}
	if strings.EqualFold(p.JobLocationType, "TELECOMMUTE") {
		job.Remote = true
		job.WorkMode = "remote"
	}
	return job, true
}

// work180EmploymentType maps the JobPosting's employmentType (a list, or a bare string) onto the
// vocabulary: the first value that maps, "" when none does.
func work180EmploymentType(raw json.RawMessage) string {
	var many []string
	if err := json.Unmarshal(raw, &many); err != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		many = []string{one}
	}
	for _, t := range many {
		if v := schemaEmploymentType(t); v != "" {
			return v
		}
	}
	return ""
}

// work180RSCJob finds the posting's own record in the page's React Server Components payload. The
// payload is the concatenation of every self.__next_f.push([1, "<chunk>"]) string, and a record
// may straddle two chunks, so they are joined before searching. The record is the object under
// "job" whose id is this posting's; the page also renders other postings, which have other ids.
func work180RSCJob(root *html.Node, id string) (work180Job, bool) {
	const prefix = "self.__next_f.push("
	var flight strings.Builder
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "script" {
			return true
		}
		text := textContent(n)
		if !strings.HasPrefix(text, prefix) || !strings.HasSuffix(text, ")") {
			return false
		}
		var push []json.RawMessage
		if json.Unmarshal([]byte(text[len(prefix):len(text)-1]), &push) != nil || len(push) != 2 {
			return false
		}
		var chunk string
		if json.Unmarshal(push[1], &chunk) == nil {
			flight.WriteString(chunk)
		}
		return false
	})
	payload := flight.String()
	marker := `"job":{"id":` + strconv.Quote(id)
	i := strings.Index(payload, marker)
	if i < 0 {
		return work180Job{}, false
	}
	var rec work180Job
	if err := json.NewDecoder(strings.NewReader(payload[i+len(`"job":`):])).Decode(&rec); err != nil || rec.ID != id {
		return work180Job{}, false
	}
	return rec, true
}
