package sources

import (
	"bytes"
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/text/encoding/charmap"
	"golang.org/x/time/rate"
)

// netempregos adapts Net-Empregos (www.net-empregos.com), Portugal's largest general job board:
// ~62k live postings across every trade, ~330 new ids an hour at the afternoon peak, about half of
// them posted by staffing agencies, ~2% filed under the Informática categories. It is boardless and an aggregator — the
// employer is read per posting. See the "Net-Empregos traps" section of AGENTS.md for the
// measurements behind each rule.
//
// No listing reaches the whole catalogue cheaply (the search pages hold 18 cards each, 3,446
// pages), so the crawl reads the two feeds the site publishes for crawlers: Sitemap.asp, the
// newest 5,000 posting URLs (under a day), and rss.asp, the newest 1,000 (~3 hours) with each
// posting's employer, category and zone. Every new posting's body is bought from its page's
// schema.org JobPosting, newest id first, within a per-run budget; the rest are refreshed by
// identity while they stay in the window.
type netempregos struct {
	http         TextGetter
	limiter      waiter
	maxNewPerRun int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNew returns, so one that runs into its deadline
	// stores nothing at all. Pages still unread when the margin is reached wait for a later run.
	deadlineMargin time.Duration
}

const (
	netempregosBaseURL = "https://www.net-empregos.com"
	netempregosSitemap = netempregosBaseURL + "/Sitemap.asp"
	netempregosRSS     = netempregosBaseURL + "/rss.asp"

	// netempregosRequestInterval paces every request of a run, feed and posting page alike, at
	// one a second — a courtesy ceiling toward a national site, not a measured limit.
	netempregosRequestInterval = time.Second
	// netempregosDetailWorkers keeps enough pages in flight that the limiter, not latency (~0.4 s
	// a page from the crawl host), sets the pace.
	netempregosDetailWorkers = 2
	// netempregosMaxNewPerRun bounds how many unseen postings one run reads: 1,500 pages at one a
	// second is 25 minutes, and the writes after the fetch (~7 new rows a second from the crawl
	// host) take ~4 more, inside the scheduler's default 2,400 s crawl budget. ~330 new ids an
	// hour reach the feeds at the afternoon peak, so a run every few hours keeps up; a posting
	// past the bound is read by a later run while it stays in the 5,000-posting window.
	netempregosMaxNewPerRun   = 1500
	netempregosDeadlineMargin = 5 * time.Minute
	// netempregosSweepGrace absorbs the window: a posting leaves both feeds within a day of being
	// published while it stays live for 30 days past its last refresh (the listing's oldest card
	// is dated 30 days back), and nothing else lists it. Thirty days past last sight keeps a live
	// posting open; a withdrawn one lingers that long at most.
	netempregosSweepGrace = 30 * 24 * time.Hour
)

// netempregosHidden is the employer name the site shows when the poster chose to stay anonymous
// (~0.6% of the RSS sample). It is a placeholder, not a company, so such a posting is dropped
// rather than filed under one bogus employer (catho's rule) — and, when the feed already says so,
// dropped before its page is bought.
const netempregosHidden = "anónimo"

// netempregosTechIndustry prefixes the site's technical categories ("Informática ( Programação
// )", "( Analise de Sistemas )", "( Internet )", "( Técnico de Hardware )"). It is the poster's
// own structured choice, so it sets IsTechHint.
const netempregosTechIndustry = "informática"

// NewNetEmpregos builds the adapter over the given client.
func NewNetEmpregos(c TextGetter) Source {
	return netempregos{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(netempregosRequestInterval), 1),
		maxNewPerRun:   netempregosMaxNewPerRun,
		deadlineMargin: netempregosDeadlineMargin,
	}
}

func (netempregos) Provider() string { return "netempregos" }

// One national board, no per-tenant id.
func (netempregos) boardless() {}

// Thousands of employers and agencies post here, each named per posting.
func (netempregos) aggregator() {}

func (netempregos) sweepGrace() time.Duration { return netempregosSweepGrace }

// netempregosPosting is one posting the feeds name. Only the RSS names the title, employer,
// category, zone and publication time; a posting the sitemap alone names carries its id and URL.
type netempregosPosting struct {
	id       int64
	url      string
	title    string
	company  string
	category string
	zone     string
	posted   *time.Time
}

func (p netempregosPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// netempregosPostingURL matches a posting URL, /<id>/<slug>/. The slug is cosmetic; the id is the
// identity.
var netempregosPostingURL = regexp.MustCompile(`https?://www\.net-empregos\.com/(\d+)/[^\s/"<>]+/`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s netempregos) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s netempregos) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every posting the feeds name, refreshes the ones already stored, and reads
// up to maxNewPerRun of the rest, newest id first. A posting the pipeline will discard anyway —
// its employer already covered by a non-aggregator source, or its title rejected — is judged on
// what the RSS says and yielded body-less without a request; one the RSS shows with a hidden
// employer is not yielded at all. A posting whose page cannot be read is not yielded; a later run
// tries again.
func (s netempregos) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		var companies []string
		for _, p := range postings {
			if p.company != "" {
				companies = append(companies, p.company)
			}
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []netempregosPosting
	deferred := 0
	for _, p := range postings {
		base := netempregosBaseJob(p)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if strings.EqualFold(p.company, netempregosHidden) {
			continue
		}
		if covered[p.company] || (p.title != "" && gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, p)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, netempregosDetailWorkers, func(p netempregosPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("netempregos: posting %d failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("netempregos: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("netempregos: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("netempregos: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s netempregos) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// netempregosBaseJob is the body-less Job a posting's feed entries spell.
func netempregosBaseJob(p netempregosPosting) Job {
	return Job{
		ExternalID: p.externalID(),
		URL:        p.url,
		Title:      p.title,
		Company:    p.company,
		IsTechHint: netempregosIsTech(p.category),
	}
}

// list reads both feeds and returns their postings merged by id, newest first. The sitemap is the
// wider window, so its failing fails the crawl; the RSS only adds what the gate reads, so a failed
// RSS costs the gate, not the crawl.
func (s netempregos) list(ctx context.Context) ([]netempregosPosting, error) {
	sitemap, err := s.get(ctx, netempregosSitemap)
	if err != nil {
		return nil, fmt.Errorf("netempregos: sitemap: %w", err)
	}
	byID := make(map[int64]*netempregosPosting)
	for _, m := range netempregosPostingURL.FindAllStringSubmatch(sitemap, -1) {
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || id <= 0 || byID[id] != nil {
			continue
		}
		byID[id] = &netempregosPosting{id: id, url: m[0]}
	}
	if len(byID) == 0 {
		return nil, fmt.Errorf("netempregos: sitemap names no posting")
	}
	rss, err := s.get(ctx, netempregosRSS)
	if err != nil {
		log.Printf("netempregos: rss failed; listing the sitemap alone: %v", err)
	} else if items, err := netempregosParseRSS(rss); err != nil {
		log.Printf("netempregos: rss unreadable; listing the sitemap alone: %v", err)
	} else {
		for _, it := range items {
			if p := byID[it.id]; p != nil {
				it.url = p.url
			}
			byID[it.id] = &it
		}
	}
	out := make([]netempregosPosting, 0, len(byID))
	for _, p := range byID {
		out = append(out, *p)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// get fetches one document and decodes it from Windows-1252: the site serves ISO-8859-1 and
// declares no charset in its Content-Type, so the shared getters' UTF-8 reading would turn every
// accented letter into U+FFFD. Windows-1252 is the superset that also maps the curly quotes and
// dashes such sites emit in the 0x80-0x9F range.
func (s netempregos) get(ctx context.Context, u string) (string, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return "", err
	}
	raw, err := s.http.GetText(ctx, u)
	if err != nil {
		return "", err
	}
	return charmap.Windows1252.NewDecoder().String(raw)
}

// netempregosRSSDoc is the part of rss.asp the adapter reads. Each item's description is an
// escaped HTML block opening with "Empresa:", "Categoria:", "Zona:" and "Data:" lines.
type netempregosRSSDoc struct {
	Items []struct {
		Title       string `xml:"title"`
		Creator     string `xml:"creator"`
		Link        string `xml:"link"`
		Description string `xml:"description"`
		PubDate     string `xml:"pubDate"`
	} `xml:"channel>item"`
}

var (
	netempregosCategory = regexp.MustCompile(`Categoria:\s*</b>([^<]*)`)
	netempregosZone     = regexp.MustCompile(`Zona:\s*</b>([^<]*)`)
)

// netempregosParseRSS reads the feed, already decoded to UTF-8 (so its iso-8859-1 declaration is
// not acted on again).
func netempregosParseRSS(doc string) ([]netempregosPosting, error) {
	dec := xml.NewDecoder(strings.NewReader(doc))
	dec.CharsetReader = func(_ string, r io.Reader) (io.Reader, error) { return r, nil }
	var feed netempregosRSSDoc
	if err := dec.Decode(&feed); err != nil {
		return nil, err
	}
	clean := func(s string) string { return strings.Join(strings.Fields(html.UnescapeString(s)), " ") }
	var out []netempregosPosting
	for _, it := range feed.Items {
		m := netempregosPostingURL.FindStringSubmatch(strings.TrimSpace(it.Link))
		if m == nil {
			continue
		}
		id, err := strconv.ParseInt(m[1], 10, 64)
		if err != nil || id <= 0 {
			continue
		}
		body := html.UnescapeString(it.Description)
		p := netempregosPosting{id: id, url: m[0], title: clean(it.Title), company: clean(it.Creator), posted: parsePubDate(strings.TrimSpace(it.PubDate))}
		if c := netempregosCategory.FindStringSubmatch(body); c != nil {
			p.category = clean(c[1])
		}
		if z := netempregosZone.FindStringSubmatch(body); z != nil {
			p.zone = clean(z[1])
		}
		out = append(out, p)
	}
	return out, nil
}

// netempregosLD is the part of the posting page's schema.org JobPosting the adapter reads. Its
// datePosted and validThrough are the time the page was RENDERED and thirty days on, so neither
// is read.
type netempregosLD struct {
	Title           string `json:"title"`
	Description     string `json:"description"`
	EmploymentType  string `json:"employmentType"`
	JobLocationType string `json:"jobLocationType"`
	Industry        string `json:"industry"`
	JobLocation     struct {
		Address struct {
			Locality string `json:"addressLocality"`
		} `json:"address"`
	} `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone — a 404/410, or the netempregosInactive notice; an error is the absence of an answer,
// including a page that is neither a posting nor that notice.
func (s netempregos) detail(ctx context.Context, p netempregosPosting) (Job, bool, error) {
	page, err := s.get(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	root, err := html.Parse(bytes.NewBufferString(page))
	if err != nil {
		return Job{}, false, err
	}
	var ld netempregosLD
	if !ldJobPosting(root, &ld) {
		// Only the site's own notice says the posting is gone. Any other page without a
		// JobPosting is not an answer: a datacenter IP is served the candidate LOGIN page
		// (HTTP 200) for every posting, and reading that as "gone" made a whole run read 1,500
		// pages and store nothing without a single error.
		if strings.Contains(page, netempregosInactive) {
			return Job{}, false, nil
		}
		return Job{}, false, fmt.Errorf("page carries no JobPosting (%q)", titleText(root))
	}
	job, ok := netempregosJob(ld, p)
	return job, ok, nil
}

// netempregosInactive is the notice the site serves, HTTP 200 and no JobPosting, for a withdrawn
// or expired posting.
const netempregosInactive = "Esta oferta já não está activa"

// netempregosSelfLink is the posting's own URL the site appends to every description.
var netempregosSelfLink = regexp.MustCompile(`(?i)(\s*<br\s*/?>)*\s*https?://www\.net-empregos\.com/\d+/\S*\s*$`)

// netempregosJob maps a posting's JobPosting and its feed entry to a Job. ok is false for one
// with no title, body or named employer.
func netempregosJob(ld netempregosLD, p netempregosPosting) (Job, bool) {
	j := netempregosBaseJob(p)
	if t := strings.Join(strings.Fields(ld.Title), " "); t != "" {
		j.Title = t
	}
	if c := strings.TrimSpace(ld.HiringOrganization.Name); c != "" {
		j.Company = c
	}
	j.Description = sanitizeHTML(netempregosSelfLink.ReplaceAllString(ld.Description, ""))
	if j.Title == "" || j.Company == "" || strings.EqualFold(j.Company, netempregosHidden) || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	zone := strings.TrimSpace(ld.JobLocation.Address.Locality)
	if zone == "" {
		zone = p.zone
	}
	j.Location, j.FallbackCountries = netempregosPlace(zone)
	j.PostedAt = NotFuture(p.posted)
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode = "remote"
		j.Remote = true
	}
	switch strings.ToUpper(strings.TrimSpace(ld.EmploymentType)) {
	case "FULL_TIME":
		j.EmploymentType = "full_time"
	case "PART_TIME":
		j.EmploymentType = "part_time"
	case "INTERN", "INTERNSHIP":
		j.EmploymentType = "internship"
	case "CONTRACTOR", "TEMPORARY":
		j.EmploymentType = "contract"
	}
	if ld.Industry != "" {
		j.IsTechHint = netempregosIsTech(ld.Industry)
	}
	return j, true
}

// netempregosIsTech reports whether a category is one of the Informática ones.
func netempregosIsTech(category string) bool {
	return strings.HasPrefix(strings.ToLower(strings.TrimSpace(category)), netempregosTechIndustry)
}

// netempregosPlace reads the site's zone, which is a Portuguese district, "( Todas as Zonas )"
// for the whole country, or one of two foreign shapes: "Estrangeiro - <country>" and "Outros
// Locais - Estrangeiro". The JobPosting's addressCountry is "PT" on all of them, so it is not
// read; a foreign posting gets no Portuguese fallback, and the location dictionary reads the
// country it names, if any.
func netempregosPlace(zone string) (string, []string) {
	pt := []string{"pt"}
	z := strings.TrimSpace(zone)
	lower := strings.ToLower(z)
	switch {
	case lower == "":
		return "", pt
	case strings.Contains(lower, "todas as zonas"):
		return "Portugal", pt
	case strings.HasPrefix(lower, "estrangeiro -"):
		return strings.TrimSpace(z[len("estrangeiro -"):]), nil
	case strings.Contains(lower, "estrangeiro"):
		return "", nil
	}
	return z, pt
}
