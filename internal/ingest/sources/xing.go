package sources

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// xing adapts XING Jobs (www.xing.com/jobs), the DACH region's professional-network job board:
// ~520k public postings from tens of thousands of employers in Germany, Austria and Switzerland,
// ~40% of them applied to on XING itself rather than on an employer's ATS. It is boardless and
// an aggregator — the employer is read per posting. See the "XING traps" section of AGENTS.md
// for the measurements behind each rule.
//
// robots.txt disallows the search and the GraphQL API, so the posting set comes from the job
// sitemaps, which carry nothing but each posting's URL. Every fact is bought with the posting
// page, whose server-rendered Apollo cache holds the posting as the web app reads it: the
// adapter hydrates only postings the catalogue lacks and refreshes the rest by identity.
type xing struct {
	http    xingHTTP
	limiter waiter
	// freshPerRun and backlogPerRun bound one run's detail requests; see xingFreshPerRun.
	freshPerRun   int
	backlogPerRun int
	now           func() time.Time
}

// xingHTTP is the transport the adapter needs: the gzip sitemaps as a raw stream (they are
// served as application/gzip files, not with a Content-Encoding the transport would undo) and
// each posting page as HTML.
type xingHTTP interface {
	StreamGetter
	HTMLGetter
}

const (
	xingBaseURL = "https://www.xing.com"
	// xingSitemapIndex lists the posting sitemaps ("<n>-sitemap.xml.gz", 50,000 URLs each). It
	// is the job sitemap index the site-wide sitemap.xml.gz names; the other job sitemaps there
	// list search landing pages (roles, skills, cities), not postings.
	xingSitemapIndex = xingBaseURL + "/jobs/sitemap.xml.gz"
	// xingMaxSitemapBytes caps one decompressed sitemap. A 50,000-URL file inflates to ~7 MB;
	// the cap only stops a runaway body from exhausting the worker.
	xingMaxSitemapBytes = 64 << 20
)

// xingRequestInterval paces every request of a run, sitemap and posting page alike, through one
// limiter: one a second, courtesy toward the site rather than a limit we measured. A posting
// page is ~360 KB and answered in ~0.5 s from the crawl host; 400 pages at 1/s drew no refusal.
const xingRequestInterval = time.Second

// xingDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
const xingDetailWorkers = 2

// xingFreshPerRun and xingBacklogPerRun split one run's 2,000 detail requests (~35 minutes at
// one a second, inside the 50-minute run timeout) into two lanes.
//
// The fresh lane takes the newest unseen ids, so a posting published since the last run is read
// before any backlog: ~7,000 new ids reach the sitemaps a day, ~900 per three-hour run.
//
// The backlog lane takes a window of the remaining unseen ids that moves every hour. It exists
// because ~27% of XING's postings come from employers whose own ATS board we already crawl
// (arbeitsagentur, join, personio, ...): the pipeline's aggregator-coverage gate discards those
// unsaved, so they stay unseen and are bought again on every run. Newest-first alone would then
// stall the backlog for good once the discarded postings above it outnumbered the budget —
// after ~8,000 hydrations. The moving window always reaches older postings; what it re-reads in
// passing is the cost of not keeping a record of declined ids, which nothing in the pipeline
// does.
const (
	xingFreshPerRun   = 1200
	xingBacklogPerRun = 800
)

// NewXing builds the adapter over the given client.
func NewXing(c xingHTTP) Source {
	return xing{
		http:          c,
		limiter:       rate.NewLimiter(rate.Every(xingRequestInterval), 1),
		freshPerRun:   xingFreshPerRun,
		backlogPerRun: xingBacklogPerRun,
		now:           time.Now,
	}
}

func (xing) Provider() string { return "xing" }

// One national board, no per-tenant id.
func (xing) boardless() {}

// Tens of thousands of employers post here, each named per posting.
func (xing) aggregator() {}

// xingPosting is one sitemap entry: the posting's numeric id and its canonical URL.
type xingPosting struct {
	id  int64
	url string
}

func (p xingPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// xingPostingPath matches a posting URL, /jobs/<city>-<title>-<id>. The slug is cosmetic; the
// trailing id alone is the identity, and it is the id the page's own state states.
var xingPostingPath = regexp.MustCompile(`^https://www\.xing\.com/jobs/[a-z0-9-]+-(\d+)$`)

// xingPostingID returns the posting id a URL carries, or 0 when it is not a posting URL.
func xingPostingID(loc string) int64 {
	m := xingPostingPath.FindStringSubmatch(strings.TrimSpace(loc))
	if m == nil {
		return 0
	}
	id, _ := strconv.ParseInt(m[1], 10, 64)
	return id
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s xing) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemaps name, refreshes the ones already stored and hydrates
// the two lanes of the rest (see xingFreshPerRun). A posting whose page cannot be read, or that
// answers 410 Gone, is not yielded: nothing is stored for it, so a later run simply tries again.
func (s xing) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []xingPosting
	var refreshed []Job
	for _, p := range postings {
		if seen(p.externalID()) {
			refreshed = append(refreshed, Job{ExternalID: p.externalID(), URL: p.url, SeenRefresh: true})
			continue
		}
		unseen = append(unseen, p)
	}
	picked := xingLanes(unseen, s.freshPerRun, s.backlogPerRun, s.now())
	jobs := fetchDetails(picked, xingDetailWorkers, func(p xingPosting) (Job, bool) {
		return s.detail(ctx, p)
	})
	return append(jobs, refreshed...), nil
}

// xingLanes picks one run's postings out of the unseen ones, which arrive newest id first: the
// newest fresh, then backlog postings from the rest, starting at an offset that advances by
// backlog every hour and wraps around.
func xingLanes(unseen []xingPosting, fresh, backlog int, now time.Time) []xingPosting {
	if len(unseen) <= fresh+backlog {
		return unseen
	}
	picked := append([]xingPosting(nil), unseen[:fresh]...)
	rest := unseen[fresh:]
	start := int((now.Unix() / 3600 * int64(backlog)) % int64(len(rest)))
	for i := 0; i < backlog; i++ {
		picked = append(picked, rest[(start+i)%len(rest)])
	}
	return picked
}

// list reads every posting sitemap the index names and returns their postings, newest id first.
// Any sitemap failing fails the crawl: a short list would leave the postings it missed
// un-refreshed, and the sweep would close them.
func (s xing) list(ctx context.Context) ([]xingPosting, error) {
	index, err := s.sitemap(ctx, xingSitemapIndex)
	if err != nil {
		return nil, fmt.Errorf("xing: sitemap index: %w", err)
	}
	ids := make(map[int64]bool)
	var out []xingPosting
	files := 0
	for _, sm := range index.Sitemaps {
		if !strings.HasSuffix(sm.Loc, "-sitemap.xml.gz") {
			continue
		}
		files++
		doc, err := s.sitemap(ctx, sm.Loc)
		if err != nil {
			return nil, fmt.Errorf("xing: sitemap %s: %w", sm.Loc, err)
		}
		for _, u := range doc.URLs {
			id := xingPostingID(u.Loc)
			if id == 0 || ids[id] {
				continue
			}
			ids[id] = true
			out = append(out, xingPosting{id: id, url: strings.TrimSpace(u.Loc)})
		}
	}
	if files == 0 {
		return nil, errors.New("xing: sitemap index names no posting sitemap")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// sitemap fetches and decodes one gzip sitemap file.
func (s xing) sitemap(ctx context.Context, url string) (sitemapDoc, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return sitemapDoc{}, err
	}
	var doc sitemapDoc
	err := s.http.GetStream(ctx, url, "application/gzip", func(r io.Reader) error {
		zr, err := gzip.NewReader(r)
		if err != nil {
			return err
		}
		defer func() { _ = zr.Close() }()
		// Read to EOF before decoding: gzip verifies its checksum only there, and a decoder
		// that stops after the root element would accept a truncated file as a short list.
		body, err := io.ReadAll(io.LimitReader(zr, xingMaxSitemapBytes+1))
		if err != nil {
			return err
		}
		if len(body) > xingMaxSitemapBytes {
			return fmt.Errorf("sitemap exceeds %d bytes", xingMaxSitemapBytes)
		}
		return xml.Unmarshal(body, &doc)
	})
	return doc, err
}

// detail fetches one posting page and maps it. A closed posting answers 410 Gone and is dropped
// like any other unreadable page: the next run asks again only if the sitemap still lists it.
func (s xing) detail(ctx context.Context, p xingPosting) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	return xingDetail(root, p)
}

// xingState is the part of the page's Apollo cache the adapter reads: every normalized object,
// keyed by its cache id ("VisibleJob:158645036.74d31c", "Company:1957805.a1aed0", ...).
type xingState map[string]json.RawMessage

// xingJob is the posting as the cache holds it. A closed posting is cached as an
// UnauthorizedJob instead and carries none of this.
type xingJob struct {
	Title     string         `json:"title"`
	Activated string         `json:"activatedAt"`
	Locations []xingLocation `json:"locations"`
	Employ    xingRef        `json:"employmentType"`
	Company   struct {
		NameOverride string  `json:"companyNameOverride"`
		Company      xingRef `json:"company"`
	} `json:"companyInfo"`
	Salary        *xingSalary     `json:"salary"`
	Discipline    xingRef         `json:"discipline"`
	Description   xingDescription `json:"description"`
	RemoteOptions []string        `json:"remoteOptions"`
}

type xingRef struct {
	Ref string `json:"__ref"`
}

// refID returns the id inside a cache reference: "EmploymentType:FULL_TIME.ef2fe9" reads
// "FULL_TIME", "Discipline:1011.6cf3f7" reads "1011". The suffix after the dot is a hash of the
// query, not part of the value.
func (r xingRef) refID() string {
	_, id, ok := strings.Cut(r.Ref, ":")
	if !ok {
		return ""
	}
	id, _, _ = strings.Cut(id, ".")
	return id
}

type xingLocation struct {
	City    string `json:"city"`
	Region  string `json:"region"`
	Country struct {
		Code string `json:"countryCode"`
	} `json:"country"`
}

// label is the place as written on the posting, "Zürich, Kanton Zürich".
func (l xingLocation) label() string { return joinNonEmpty(l.City, l.Region) }

// xingSalary is a salary the cache states. Its __typename says whose figure it is: a
// SalaryRange or Salary is the employer's own ("Unternehmensangabe" on the page), while a
// SalaryEstimate is XING's guess for the role and is never stored.
type xingSalary struct {
	Type     string `json:"__typename"`
	Currency string `json:"currency"`
	Minimum  int    `json:"minimum"`
	Maximum  int    `json:"maximum"`
	Amount   int    `json:"amount"`
}

// xingDescription is the body in one of two shapes: an HtmlDescription's single content, or a
// TemplateData's sections, each under the heading the employer gave it.
type xingDescription struct {
	Type            string `json:"__typename"`
	Content         string `json:"content"`
	Generic         string `json:"genericDescription"`
	ResponsibilityT string `json:"responsibilityTitle"`
	ResponsibilityC string `json:"responsibilityContent"`
	SkillsT         string `json:"skillsTitle"`
	SkillsC         string `json:"skillsContent"`
	WeOfferT        string `json:"weOfferTitle"`
	WeOfferC        string `json:"weOfferContent"`
}

// xingITDiscipline is XING's own "IT und Softwareentwicklung" discipline (~8% of postings).
const xingITDiscipline = "1011"

// xingDetail maps a posting page to a Job. ok is false for a page carrying no live posting, and
// for one without a title, employer or body: storing it body-less would make it seen, and a seen
// posting is never read again.
func xingDetail(root *html.Node, p xingPosting) (Job, bool) {
	state, ok := xingApolloState(root)
	if !ok {
		return Job{}, false
	}
	var j xingJob
	if !state.visibleJob(p.id, &j) {
		return Job{}, false
	}
	title := strings.TrimSpace(j.Title)
	company := strings.TrimSpace(j.Company.NameOverride)
	if company == "" {
		company = state.companyName(j.Company.Company.Ref)
	}
	body := j.Description.html()
	if title == "" || company == "" || body == "" {
		return Job{}, false
	}
	workMode := xingWorkMode(j.RemoteOptions)
	codes := make([]string, 0, len(j.Locations))
	for _, l := range j.Locations {
		codes = append(codes, l.Country.Code)
	}
	job := Job{
		ExternalID:     p.externalID(),
		URL:            p.url,
		Title:          title,
		Company:        company,
		Location:       distinctJoin(j.Locations, "; ", xingLocation.label),
		Description:    body,
		WorkMode:       workMode,
		Remote:         workMode == "remote",
		PostedAt:       NotFuture(parseRFC3339(j.Activated)),
		EmploymentType: xingEmploymentType(j.Employ.refID()),
		Countries:      countriesFromCodes(codes),
		IsTechHint:     j.Discipline.refID() == xingITDiscipline,
	}
	if lo, hi, ok := j.Salary.yearly(); ok {
		job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, j.Salary.Currency, "year"
	}
	return job, true
}

// xingCratePrefix opens the script that carries the page's state: a JavaScript assignment of an
// object literal, not JSON.
const xingCratePrefix = "window.crate="

// xingUndefined is the one JavaScript-only token the literal uses. Rewriting it to null, only in
// a value position, is what makes the literal JSON.
var xingUndefined = regexp.MustCompile(`:undefined([,}\]])`)

// xingApolloState reads the Apollo cache out of the page's runtime-config script.
func xingApolloState(root *html.Node) (xingState, bool) {
	script := firstByID(root, "runtime-config")
	if script == nil {
		return nil, false
	}
	src := strings.TrimSpace(textContent(script))
	src, ok := strings.CutPrefix(src, xingCratePrefix)
	if !ok {
		return nil, false
	}
	src = xingUndefined.ReplaceAllString(strings.TrimSuffix(src, ";"), ":null$1")
	var crate struct {
		ServerData struct {
			State xingState `json:"APOLLO_STATE"`
		} `json:"serverData"`
	}
	if err := json.Unmarshal([]byte(src), &crate); err != nil {
		return nil, false
	}
	return crate.ServerData.State, crate.ServerData.State != nil
}

// visibleJob decodes the cache's live posting with the given id.
func (s xingState) visibleJob(id int64, v *xingJob) bool {
	prefix := "VisibleJob:" + strconv.FormatInt(id, 10) + "."
	for key, raw := range s {
		if strings.HasPrefix(key, prefix) {
			return json.Unmarshal(raw, v) == nil
		}
	}
	return false
}

// companyName reads a Company object's name, the fallback when the posting states no override.
func (s xingState) companyName(ref string) string {
	raw, ok := s[ref]
	if !ok {
		return ""
	}
	var c struct {
		Name string `json:"companyName"`
	}
	if json.Unmarshal(raw, &c) != nil {
		return ""
	}
	return strings.TrimSpace(c.Name)
}

// html assembles the body. A TemplateData's company description is left out: it is the
// employer's standing blurb, repeated across its postings, not part of this one.
func (d xingDescription) html() string {
	if d.Type == "HtmlDescription" {
		return sanitizeHTML(d.Content)
	}
	var b strings.Builder
	b.WriteString(d.Generic)
	for _, sec := range [][2]string{
		{d.ResponsibilityT, d.ResponsibilityC},
		{d.SkillsT, d.SkillsC},
		{d.WeOfferT, d.WeOfferC},
	} {
		if strings.TrimSpace(sec[1]) == "" {
			continue
		}
		if t := strings.TrimSpace(sec[0]); t != "" {
			b.WriteString("<h3>" + html.EscapeString(t) + "</h3>")
		}
		b.WriteString(sec[1])
	}
	return sanitizeHTML(b.String())
}

// xingWorkMode maps the posting's remoteOptions. FULL_REMOTE is remote and PARTLY_REMOTE hybrid.
// NON_REMOTE is not read as onsite: it is the form's default, set on 5% of sampled postings
// whose own text offers home office — one titled "100% Remote deutschlandweit" — so it says
// nothing, and the pipeline's location and description reading decides.
func xingWorkMode(opts []string) string {
	mode := ""
	for _, o := range opts {
		switch o {
		case "FULL_REMOTE":
			return "remote"
		case "PARTLY_REMOTE":
			mode = "hybrid"
		}
	}
	return mode
}

// xingEmploymentType maps the employment type the cache names.
func xingEmploymentType(t string) string {
	switch t {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "INTERN":
		return "internship"
	case "CONTRACTOR", "TEMPORARY":
		return "contract"
	}
	return ""
}

// xingMinYearlySalary is the smallest figure read as a yearly salary. XING states no period: its
// ld+json calls every employer salary YEAR, including "14 € – 18 €" (an hourly wage) and
// "2.800 € – 3.400 €" (a monthly one). A full-time year below this does not occur in DACH, and an
// hour, a day or a month above it does not either, so only figures past it are stored, as yearly.
const xingMinYearlySalary = 20000

// yearly returns the employer's own salary as a yearly range, or ok=false for an estimate, for a
// figure too small to be a year, and for no salary at all.
func (s *xingSalary) yearly() (lo, hi *int, ok bool) {
	if s == nil || s.Currency == "" {
		return nil, nil, false
	}
	minimum, maximum := s.Minimum, s.Maximum
	switch s.Type {
	case "SalaryRange":
	case "Salary":
		minimum, maximum = s.Amount, s.Amount
	default:
		return nil, nil, false
	}
	if minimum < xingMinYearlySalary || maximum < minimum {
		return nil, nil, false
	}
	return &minimum, &maximum, true
}
