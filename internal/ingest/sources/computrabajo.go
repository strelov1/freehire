package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// computrabajo adapts Computrabajo (<cc>.computrabajo.com), the general job board Redarbor runs
// in each Spanish-speaking Latin American country — not its Pandapé ATS (pandapelatam), whose
// tenants publish here among thousands of other employers. The board is the country's subdomain
// ("ar"); every trade, mostly staffing agencies, a few percent technical. It is an aggregator —
// the employer is read per posting. See the "Computrabajo traps" section of AGENTS.md for the
// measurements behind each rule.
//
// A country's listing stops at 500 pages (10k postings) while Argentina alone holds ~15k, so the
// walk reads it province by province — the site's own location filter, each slice far under the
// cap — taking the province list and sizes from the filter itself. The cards spell each
// posting's title, employer, place, salary and arrangement — everything but the body, which is
// bought from the posting page for postings the catalogue lacks, within a per-run budget.
type computrabajo struct {
	http           HTMLGetter
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

const (
	// computrabajoPageSize is the cards a listing page carries.
	computrabajoPageSize = 20
	// computrabajoMaxPages is the site's own pager ceiling: page 500 of a listing answers 404.
	computrabajoMaxPages = 499
	// computrabajoRequestInterval paces every request of a run, listing and posting page alike,
	// at one a second — a courtesy ceiling, not a measured limit.
	computrabajoRequestInterval = time.Second
	computrabajoDetailWorkers   = 2
	// computrabajoMaxNewPerRun bounds the posting pages one run reads: Argentina's ~770 listing
	// pages already take ~15 minutes at one a second, so 600 posting pages and the writes after
	// them fit the scheduler's 2,400 s crawl budget. A cold catalogue is taken in slices.
	computrabajoMaxNewPerRun   = 600
	computrabajoDeadlineMargin = 5 * time.Minute
)

// NewComputrabajo builds the adapter over the given client. The edge refuses the project's bare
// User-Agent, so the registry hands it the browser-compatible form (compatibleUASource).
func NewComputrabajo(c HTMLGetter) Source {
	return computrabajo{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(computrabajoRequestInterval), 1),
		maxNewPerRun:   computrabajoMaxNewPerRun,
		deadlineMargin: computrabajoDeadlineMargin,
	}
}

func (computrabajo) Provider() string { return "computrabajo" }

// Thousands of employers and agencies post here, each named per posting.
func (computrabajo) aggregator() {}

// computrabajoCountry is a board: a two-letter country subdomain.
var computrabajoCountry = regexp.MustCompile(`^[a-z]{2}$`)

func computrabajoBase(country string) string { return "https://" + country + ".computrabajo.com" }

// computrabajoCard is one listing card: what the gate and the job need before the page is bought.
type computrabajoCard struct {
	id       string
	url      string
	title    string
	company  string
	place    string
	salary   string
	workMode string
	// rank is the card's position in its province's listing, which runs newest first; the
	// budget is spent across provinces by rank, so no province starves the others.
	rank int
}

// computrabajoProvince is one slice of the location filter: its listing path and its size.
type computrabajoProvince struct {
	path  string
	count int
}

// computrabajoIDPath is the 32-hex id ending every posting path.
var computrabajoIDPath = regexp.MustCompile(`^/ofertas-de-trabajo/oferta-de-trabajo-de-[^/?#]*-([0-9A-F]{32})$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s computrabajo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s computrabajo) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks every province, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest. A posting the pipeline will discard anyway is judged on its card and
// yielded body-less without a request; one with a hidden employer is dropped before any. A
// posting whose page cannot be read is not yielded; a later run tries again.
func (s computrabajo) FetchNewGated(ctx context.Context, e CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	country := strings.ToLower(strings.TrimSpace(e.Board))
	if !computrabajoCountry.MatchString(country) {
		return nil, fmt.Errorf("computrabajo: board %q is not a country subdomain", e.Board)
	}
	cards, err := s.list(ctx, country)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, c.company)
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []computrabajoCard
	deferred := 0
	for _, c := range cards {
		base := computrabajoBaseJob(c, country)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[c.company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		if len(unseen) >= s.maxNewPerRun {
			deferred++
			continue
		}
		unseen = append(unseen, c)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, computrabajoDetailWorkers, func(c computrabajoCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c, country)
		if err != nil {
			log.Printf("computrabajo: posting %s failed; deferring: %v", c.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("computrabajo: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("computrabajo: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("computrabajo: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s computrabajo) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list reads the province filter and walks each province, returning the named-employer postings
// ordered by their rank in their province. Any page failing fails the crawl: a partial listing
// would let the company-scoped sweep close the postings of the employers it did see on the pages
// it did not.
func (s computrabajo) list(ctx context.Context, country string) ([]computrabajoCard, error) {
	base := computrabajoBase(country)
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	// Page 1 of the national listing redirects home; page 2 carries the same filter.
	root, err := s.http.GetHTML(ctx, base+"/ofertas-de-trabajo/?p=2")
	if err != nil {
		return nil, fmt.Errorf("computrabajo: listing: %w", err)
	}
	provinces := computrabajoProvinces(root)
	if len(provinces) == 0 {
		return nil, fmt.Errorf("computrabajo: listing carries no location filter")
	}
	seen := make(map[string]bool)
	var out []computrabajoCard
	for _, p := range provinces {
		pages := min((p.count+computrabajoPageSize-1)/computrabajoPageSize+1, computrabajoMaxPages)
		rank := 0
		for page := 1; page <= pages; page++ {
			if err := s.limiter.Wait(ctx); err != nil {
				return nil, err
			}
			u := base + p.path
			if page > 1 {
				u += "?p=" + strconv.Itoa(page)
			}
			root, err := s.http.GetHTML(ctx, u)
			if err != nil {
				// Past a listing's end the site answers an empty page, or a 404 at its pager cap.
				var se *StatusError
				if errors.As(err, &se) && se.Code == http.StatusNotFound {
					break
				}
				return nil, fmt.Errorf("computrabajo: %s page %d: %w", p.path, page, err)
			}
			cards := computrabajoCards(root, base)
			if len(cards) == 0 {
				break
			}
			for _, c := range cards {
				rank++
				if seen[c.id] || c.company == "" {
					continue
				}
				seen[c.id] = true
				c.rank = rank
				out = append(out, c)
			}
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].rank < out[j].rank })
	return out, nil
}

// computrabajoProvinces reads the listing's location filter: every province's path and size.
func computrabajoProvinces(root *html.Node) []computrabajoProvince {
	var out []computrabajoProvince
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "ul" || Attr(n, "data-id") != "idlocation" {
			return true
		}
		for li := n.FirstChild; li != nil; li = li.NextSibling {
			if li.Type != html.ElementNode || li.Data != "li" {
				continue
			}
			a := firstByTag(li, "a")
			if a == nil {
				continue
			}
			path := strings.TrimSpace(Attr(a, "href"))
			if !strings.HasPrefix(path, "/empleos-en-") {
				continue
			}
			p := computrabajoProvince{path: path}
			if tag := firstByClass(li, "tag"); tag != nil {
				p.count, _ = strconv.Atoi(strings.ReplaceAll(strings.TrimSpace(textContent(tag)), ".", ""))
			}
			out = append(out, p)
		}
		return false
	})
	return out
}

// computrabajoCards reads the posting cards of one listing page. A card whose employer is hidden
// ("Importante empresa del sector") names it without the link to its company page, and is
// returned with no company.
func computrabajoCards(root *html.Node, base string) []computrabajoCard {
	var out []computrabajoCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "article" || !HasClass(n, "box_offer") {
			return true
		}
		if c, ok := computrabajoCardFrom(n, base); ok {
			out = append(out, c)
		}
		return false
	})
	return out
}

func computrabajoCardFrom(n *html.Node, base string) (computrabajoCard, bool) {
	a := firstByClass(n, "js-o-link")
	if a == nil {
		return computrabajoCard{}, false
	}
	path, _, _ := strings.Cut(strings.TrimSpace(Attr(a, "href")), "#")
	m := computrabajoIDPath.FindStringSubmatch(path)
	if m == nil {
		return computrabajoCard{}, false
	}
	c := computrabajoCard{id: m[1], url: base + path, title: computrabajoClean(textContent(a))}
	walk(n, func(x *html.Node) bool {
		if x.Type != html.ElementNode {
			return true
		}
		if x.Data == "a" && hasAttr(x, "offer-grid-article-company-url") {
			c.company = computrabajoClean(textContent(x))
			return false
		}
		if x.Data == "span" && HasClass(x, "dIB") {
			switch {
			case firstByClass(x, "i_salary") != nil:
				c.salary = computrabajoClean(textContent(x))
			case firstByClass(x, "i_home") != nil:
				c.workMode = computrabajoClean(textContent(x))
			}
			return false
		}
		return true
	})
	if p := firstByClass(n, "fs16"); p != nil {
		// The first fs16 paragraph is the employer line; the place is the next one's span.
		for q := p.NextSibling; q != nil; q = q.NextSibling {
			if q.Type == html.ElementNode && q.Data == "p" && HasClass(q, "fs16") {
				if span := firstByTag(q, "span"); span != nil {
					c.place = computrabajoClean(textContent(span))
				}
				break
			}
		}
	}
	if c.title == "" {
		return computrabajoCard{}, false
	}
	return c, true
}

func hasAttr(n *html.Node, name string) bool {
	for _, a := range n.Attr {
		if a.Key == name {
			return true
		}
	}
	return false
}

func computrabajoClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// computrabajoBaseJob is the body-less Job a card alone supports: the identity, and the facts the
// gate judges on.
func computrabajoBaseJob(c computrabajoCard, country string) Job {
	j := Job{
		ExternalID:        c.id,
		URL:               c.url,
		Title:             c.title,
		Company:           c.company,
		Location:          c.place,
		WorkMode:          computrabajoWorkMode(c.workMode),
		FallbackCountries: []string{country},
	}
	j.Remote = j.WorkMode == "remote"
	// The card states no currency; it is the country's own, so only Argentina's is named.
	if lo, ok := computrabajoSalary(c.salary); ok && country == "ar" {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &lo, "ARS", "month"
	}
	return j
}

// computrabajoLD is the part of the posting page's schema.org JobPosting the adapter reads. Not
// every page carries one (one in seven sampled did not), so it only adds to what the page shows.
type computrabajoLD struct {
	DatePosted         string `json:"datePosted"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation struct {
		Address struct {
			Country string `json:"addressCountry"`
		} `json:"address"`
	} `json:"jobLocation"`
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or a page carrying no posting body; an error is the absence of an answer.
func (s computrabajo) detail(ctx context.Context, c computrabajoCard, country string) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.http.GetHTML(ctx, c.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	j := computrabajoBaseJob(c, country)
	offer := computrabajoOffer(root)
	if offer == nil {
		// A withdrawn posting redirects to a listing, which has no offer block.
		return Job{}, false, nil
	}
	body, tags, years := computrabajoBody(offer)
	j.Description = sanitizeHTML(body)
	if strings.TrimSpace(j.Description) == "" {
		return Job{}, false, nil
	}
	for _, t := range tags {
		if m := computrabajoWorkMode(t); m != "" {
			j.WorkMode, j.Remote = m, m == "remote"
		}
		if et := computrabajoEmploymentType(t); et != "" {
			j.EmploymentType = et
		}
	}
	if years > 0 {
		j.ExperienceYearsMin = &years
	}
	var ld computrabajoLD
	if ldJobPosting(root, &ld) {
		if name := computrabajoClean(ld.HiringOrganization.Name); name != "" {
			j.Company = name
		}
		if code := location.NormalizeCountry(strings.TrimSpace(ld.JobLocation.Address.Country)); code != "" {
			j.Countries = []string{code}
		}
		if t, err := time.Parse("2006-01-02", strings.TrimSpace(ld.DatePosted)); err == nil {
			j.PostedAt = NotFuture(&t)
		}
	}
	return j, true, nil
}

// computrabajoOffer finds the posting page's offer block, <div div-link="oferta">.
func computrabajoOffer(root *html.Node) *html.Node {
	var found *html.Node
	walk(root, func(n *html.Node) bool {
		if found != nil {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "div" && Attr(n, "div-link") == "oferta" {
			found = n
			return false
		}
		return true
	})
	return found
}

var computrabajoYears = regexp.MustCompile(`^(\d+) años? de experiencia$`)

// computrabajoBody renders the offer block's body: the employer's text and its "Requerimientos"
// list, without the heading, the fact tags above it, or the keyword line and skill chips after it. It also returns
// the fact tags (salary, contract, schedule, arrangement) and the years of experience the
// requirements state.
func computrabajoBody(offer *html.Node) (body string, tags []string, years int) {
	var b strings.Builder
	for n := offer.FirstChild; n != nil; n = n.NextSibling {
		if n.Type != html.ElementNode {
			continue
		}
		switch {
		case n.Data == "div" && HasClass(n, "mbB"):
			walk(n, func(x *html.Node) bool {
				if x.Type == html.ElementNode && x.Data == "span" && HasClass(x, "tag") {
					tags = append(tags, computrabajoClean(textContent(x)))
					return false
				}
				return true
			})
		case n.Data == "p" && HasClass(n, "mbB"):
			// The closing "Palabras clave: ..." line is the site's search keywords, not the posting.
			if !strings.HasPrefix(computrabajoClean(textContent(n)), "Palabras clave:") {
				b.WriteString("<p>" + innerHTML(n) + "</p>")
			}
		case n.Data == "p" && computrabajoClean(textContent(n)) == "Requerimientos":
			b.WriteString("<h3>Requerimientos</h3>")
		case n.Data == "ul" && HasClass(n, "disc"):
			b.WriteString("<ul>" + innerHTML(n) + "</ul>")
			walk(n, func(x *html.Node) bool {
				if x.Type == html.ElementNode && x.Data == "li" {
					if m := computrabajoYears.FindStringSubmatch(computrabajoClean(textContent(x))); m != nil {
						years, _ = strconv.Atoi(m[1])
					}
					return false
				}
				return true
			})
		}
	}
	return b.String(), tags, years
}

// computrabajoWorkMode reads the arrangement tag. "Presencial" says nothing a reader would not
// assume and is not mapped.
func computrabajoWorkMode(tag string) string {
	switch strings.ToLower(tag) {
	case "remoto":
		return "remote"
	case "presencial y remoto":
		return "hybrid"
	}
	return ""
}

func computrabajoEmploymentType(tag string) string {
	switch strings.ToLower(tag) {
	case "jornada completa":
		return "full_time"
	case "jornada part time", "jornada parcial", "jornada reducida", "por horas":
		return "part_time"
	}
	return ""
}

// computrabajoSalaryRe reads the salary fact, "$ 2.500.000,00 (Mensual)", possibly followed by a
// commission note ("+ Comisión").
var computrabajoSalaryRe = regexp.MustCompile(`^\$ ([\d.]+),\d{2} \(Mensual\)`)

// computrabajoMinSalary is the floor under which a stated monthly salary is a placeholder: posters
// type "$ 2.000,00" or "$ 11.111,00" to get past the form, and Argentina's minimum wage is several
// hundred thousand pesos a month.
const computrabajoMinSalary = 100000

// computrabajoSalary reads the card's structured monthly salary, in pesos. A figure under the
// floor or made of one repeated digit (999.999) is a placeholder.
func computrabajoSalary(s string) (int, bool) {
	m := computrabajoSalaryRe.FindStringSubmatch(strings.TrimSpace(s))
	if m == nil {
		return 0, false
	}
	digits := strings.ReplaceAll(m[1], ".", "")
	n, err := strconv.Atoi(digits)
	if err != nil || n < computrabajoMinSalary || strings.Count(digits, digits[:1]) == len(digits) {
		return 0, false
	}
	return n, true
}
