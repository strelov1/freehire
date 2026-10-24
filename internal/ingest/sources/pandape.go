package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// pandape adapts Pandapé, the ATS InfoJobs sells in Brazil and Computrabajo sells across the
// rest of Latin America: one <tenant>.<domain> career site per employer, and a board is that
// tenant label. The two deployments run the same software on separate domains with separate
// tenant namespaces (a Brazilian tenant 404s on the Latin-American domain and vice versa), so
// each is its own provider (pandapeDomain). See the "Pandapé traps" section of AGENTS.md for
// the measurements behind each rule.
//
// The career site renders its first 20 vacancies; its own "load more" button POSTs
// /ListVacancies, which answers JSON carrying the page's vacancy cards as an HTML fragment, the
// tenant's total ("subtotal") and an isLast flag. The cards carry no body, so this is a
// HydratingSource over each vacancy's /Detail/<id> page, which server-renders the description
// and (usually) a schema.org JobPosting.
type pandape struct {
	domain         pandapeDomain
	http           pandapeHTTP
	limiter        waiter
	maxNewPerBoard int
}

type pandapeHTTP interface {
	FormJSONPoster
	HTMLGetter
}

// pandapeDomain is one Pandapé deployment.
type pandapeDomain struct {
	provider string
	// host is the career-site domain every tenant is a subdomain of.
	host string
	// templatedJobPosting marks a deployment whose JobPosting is filled from a Brazilian
	// template rather than from the vacancy: on the Latin-American domain every JobPosting
	// sampled said addressCountry BR, currency BRL and employmentType partial-time (a Lima call
	// centre's full-time job, paid in soles), and cut the last letter off a locality with no
	// region ("Ayacuch", "Santiago Centr"), while some vacancy pages carry no JobPosting at all.
	// There only datePosted and the salary bounds are read from it; the place comes from the
	// card, the country from the page's own analytics props, the currency from that country.
	templatedJobPosting bool
}

var (
	// pandapeBrazil is InfoJobs Brasil's deployment. The same tenant answers on
	// pandape.catho.com.br too (identical listing), so one host is canonical.
	pandapeBrazil = pandapeDomain{provider: "pandape", host: "pandape.infojobs.com.br"}
	// pandapeLatam is Computrabajo's deployment across Spanish-speaking Latin America (Chile,
	// Peru, Colombia, Mexico, ...); each tenant states its own country.
	pandapeLatam = pandapeDomain{provider: "pandapelatam", host: "pandape.computrabajo.com", templatedJobPosting: true}
)

// pandapePageSize is the listing page size. The server ignores anything larger (PageSize=200
// still answers 20 cards), so asking for more only hides the real page size.
const pandapePageSize = 20

// pandapeMaxPages is the pager's own window: it serves 500 pages (10,000 vacancies) and no
// more — measured 2026-10-24 on activoschile, which states 55,483: page 500 answers 20 cards,
// page 501 answers {"isLast": true} with no cards and no subtotal. The largest Brazilian tenant
// (rhftalentos, 8,319 vacancies, 416 pages) fits; a tenant stating more than the window can
// never be listed whole, so its crawl fails on the first page rather than walk 500 pages to the
// same answer, and fullBoardListing never sweeps it on a partial list.
const pandapeMaxPages = 500

// PandapeListingWindow is how many vacancies the pager can list for one tenant. Exported for
// cmd/harvest-boards, which does not propose a tenant stating more.
const PandapeListingWindow = pandapeMaxPages * pandapePageSize

// PandapeRequestInterval paces every request of a run — every tenant's listing and detail
// pages alike, since all ~3.3k tenants are served by one origin, which answers a burst with a
// 403 (not a 429, so the client's per-host backoff never sees it). Measured 2026-10-23 from the
// prod address: ~2 requests a second sustained for ~40 minutes (~2.3k tenant pages) drew no
// refusal; ~6-7 a second for one minute drew a run of 403s on detail and listing pages alike,
// which cleared within seconds of the burst ending. Two a second is the rate that was observed
// to pass, not a limit we know. Exported because cmd/harvest-boards probes the same origin and
// must hold the same pace: unpaced, its 16 workers reached ~35 req/s and drew the 403.
const PandapeRequestInterval = 500 * time.Millisecond

// pandapeMaxNewPerBoard bounds how many unseen vacancies one board's crawl hydrates. The first
// crawl of the largest tenants faces thousands of bodies (randstad-brasil lists 3,827); at two
// requests a second shared by the whole fleet, one such board would hold the run for half an
// hour while every other board waits; 300 is two and a half minutes. The vacancies past the bound are not yielded, so
// nothing is stored for them, and the next run hydrates the next slice.
const pandapeMaxNewPerBoard = 300

// NewPandape builds the adapter for InfoJobs Brasil's Pandapé over the shared client.
func NewPandape(c pandapeHTTP) Source { return newPandape(pandapeBrazil, c) }

// NewPandapeLatam builds the adapter for Computrabajo's Latin-American Pandapé.
func NewPandapeLatam(c pandapeHTTP) Source { return newPandape(pandapeLatam, c) }

func newPandape(d pandapeDomain, c pandapeHTTP) pandape {
	return pandape{
		domain:         d,
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(PandapeRequestInterval), 1),
		maxNewPerBoard: pandapeMaxNewPerBoard,
	}
}

func (s pandape) Provider() string { return s.domain.provider }

// fullBoardListing: every page states the tenant's total and whether it is the last; the walk
// ends only on isLast and fails unless the distinct vacancies read reach the stated total. Any
// page failing, a page that adds nothing before the end, and the page cap all fail the crawl.
func (pandape) fullBoardListing() {}

// pandapePage is the pager's answer.
type pandapePage struct {
	View     string `json:"view"`
	IsLast   *bool  `json:"isLast"`
	Subtotal *int   `json:"subtotal"`
}

// pandapeCard is one vacancy card of the listing: the title and the icon-tagged facts.
// Schedule and Salary are read only on the Latin-American domain, where the JobPosting's are
// a template's.
type pandapeCard struct {
	ID, Title, Location, WorkMode, Contract, Schedule, Salary string
}

var pandapeBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

func (s pandape) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists the tenant's whole career site, refreshes the vacancies already stored and
// hydrates up to maxNewPerBoard of the rest.
func (s pandape) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !pandapeBoard.MatchString(board) {
		return nil, fmt.Errorf("pandape: board %q is not a tenant label", e.Board)
	}
	cards, err := s.list(ctx, board)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, board, e, cards, seen)
}

func (d pandapeDomain) site(board string) string { return "https://" + board + "." + d.host }

func (d pandapeDomain) vacancyURL(board, id string) string { return d.site(board) + "/Detail/" + id }

func (s pandape) list(ctx context.Context, board string) ([]pandapeCard, error) {
	var cards []pandapeCard
	ids := map[string]bool{}
	total := -1
	for page := 1; ; page++ {
		if page > pandapeMaxPages {
			return nil, fmt.Errorf("pandape: no end of the listing after %d pages", pandapeMaxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var resp pandapePage
		form := url.Values{"PageNumber": {strconv.Itoa(page)}, "PageSize": {strconv.Itoa(pandapePageSize)}}
		if err := s.http.PostFormJSON(ctx, s.domain.site(board)+"/ListVacancies", form, &resp); err != nil {
			// An unknown tenant answers 404 with the platform's "A Página de Carreiras desta
			// empresa não está mais disponível no Pandapé" page; a live one never does.
			var se *StatusError
			if page == 1 && errors.As(err, &se) && se.Code == http.StatusNotFound {
				return nil, fmt.Errorf("pandape: board %q: %w", board, ErrBoardGone)
			}
			return nil, fmt.Errorf("pandape: page %d: %w", page, err)
		}
		if resp.IsLast == nil || resp.Subtotal == nil {
			return nil, fmt.Errorf("pandape: page %d carries no isLast/subtotal", page)
		}
		if total < 0 {
			total = *resp.Subtotal
			if total > PandapeListingWindow {
				return nil, fmt.Errorf("pandape: the career site states %d vacancies, past the pager's %d-page window", total, pandapeMaxPages)
			}
		}
		got, err := parsePandapeCards(resp.View)
		if err != nil {
			return nil, fmt.Errorf("pandape: page %d: %w", page, err)
		}
		for _, c := range got {
			if !ids[c.ID] {
				ids[c.ID] = true
				cards = append(cards, c)
			}
		}
		if *resp.IsLast {
			// More than the total is fine — a vacancy published mid-walk shifts the pages and
			// the total was read before it existed. Fewer is a walk that missed some.
			if len(cards) < total {
				return nil, fmt.Errorf("pandape: read %d vacancies, the career site states %d", len(cards), total)
			}
			return cards, nil
		}
		// The empty-page proof is the RAW card count: a page of already-seen cards (a re-served
		// page) is not proof that nothing follows it.
		if len(got) == 0 {
			return nil, fmt.Errorf("pandape: page %d is empty before the last page (%d of %d)", page, len(cards), total)
		}
	}
}

var pandapeDetailPath = regexp.MustCompile(`^/Detail/(\d+)$`)

// parsePandapeCards reads the vacancy cards of one listing fragment: an <a class="card-vacancy">
// linking /Detail/<id>, wrapping the title (h3[title]) and facts each led by an icon. The icon
// names differ between the card and the vacancy page, so a fact is identified by its icon here
// and only here.
func parsePandapeCards(fragment string) ([]pandapeCard, error) {
	root, err := xhtml.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, err
	}
	var out []pandapeCard
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "a" || !HasClass(n, "card-vacancy") {
			return true
		}
		m := pandapeDetailPath.FindStringSubmatch(Attr(n, "href"))
		if m == nil {
			return false
		}
		c := pandapeCard{ID: m[1]}
		if h := firstByTag(n, "h3"); h != nil {
			c.Title = strings.Join(strings.Fields(textContent(h)), " ")
		}
		walk(n, func(i *xhtml.Node) bool {
			if i.Type != xhtml.ElementNode || i.Data != "i" || !HasClass(i, "icon") {
				return true
			}
			// <div class="..."><div class="icon-container"><i class="icon icon-X"></i></div> fact</div>
			holder := i.Parent
			if holder != nil {
				holder = holder.Parent
			}
			if holder == nil {
				return true
			}
			text := strings.Join(strings.Fields(textContent(holder)), " ")
			switch {
			case HasClass(i, "icon-location-pin-1"):
				c.Location = text
			case HasClass(i, "icon-buildings"), HasClass(i, "icon-building-house"), HasClass(i, "icon-user-with-headphones"):
				c.WorkMode = text
			case HasClass(i, "icon-sheet"):
				c.Contract = text
			case HasClass(i, "icon-clock"):
				c.Schedule = text
			case HasClass(i, "icon-wallet"):
				c.Salary = text
			}
			return true
		})
		if c.Title != "" {
			out = append(out, c)
		}
		return false
	})
	return out, nil
}

// hydrate reads the vacancy page of each vacancy the catalogue lacks, up to maxNewPerBoard. A page
// that could not be read is deferred to a later run rather than stored body-less; a crawl that
// listed new vacancies and read none of them fails.
func (s pandape) hydrate(ctx context.Context, board string, e CompanyEntry, cards []pandapeCard, seen func(string) bool) ([]Job, error) {
	var refreshed []Job
	var unseen []pandapeCard
	for _, c := range cards {
		if seen(c.ID) {
			refreshed = append(refreshed, Job{ExternalID: c.ID, URL: s.domain.vacancyURL(board, c.ID), Title: c.Title, Company: e.Company, SeenRefresh: true})
			continue
		}
		if len(unseen) < s.maxNewPerBoard {
			unseen = append(unseen, c)
		}
	}
	var read atomic.Int64
	jobs := fetchDetails(unseen, defaultDetailWorkers, func(c pandapeCard) (Job, bool) {
		link := s.domain.vacancyURL(board, c.ID)
		if err := s.limiter.Wait(ctx); err != nil {
			return Job{}, false
		}
		root, err := s.http.GetHTML(ctx, link)
		if err != nil {
			if detailUnreadable(err) {
				log.Printf("pandape: %s: %v; deferring", link, err)
			}
			return Job{}, false
		}
		job, ok := s.domain.job(root, c, link, e)
		if !ok {
			log.Printf("pandape: %s is not a vacancy page; deferring", link)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	if len(unseen) > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("pandape: listed %d new vacancies and read none of their pages", len(unseen))
	}
	return append(jobs, refreshed...), nil
}

// pandapeLD is the part of the vacancy page's schema.org JobPosting the adapter reads.
type pandapeLD struct {
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	DatePosted     string       `json:"datePosted"`
	EmploymentType string       `json:"employmentType"`
	JobLocation    schemaPlaces `json:"jobLocation"`
	BaseSalary     *struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string   `json:"unitText"`
			MinValue *float64 `json:"minValue"`
			MaxValue *float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// job maps a vacancy page plus its card. ok is false when the page is not a vacancy
// page: on the Brazilian domain it carries no JobPosting; on the Latin-American one, where the
// JobPosting is optional (templatedJobPosting), it carries no body either.
func (d pandapeDomain) job(root *xhtml.Node, c pandapeCard, link string, e CompanyEntry) (Job, bool) {
	var ld pandapeLD
	if !ldJobPosting(root, &ld) && !d.templatedJobPosting {
		return Job{}, false
	}
	job := Job{
		ExternalID:  c.ID,
		URL:         link,
		Title:       c.Title,
		Company:     e.Company,
		Location:    c.Location,
		Description: pandapeBody(root, ld.Description),
		PostedAt:    pandapePostedAt(ld.DatePosted),
	}
	if job.Title == "" {
		job.Title = strings.TrimSpace(ld.Title)
	}
	switch c.WorkMode {
	case "Home Office", "Remoto":
		job.WorkMode, job.Remote = "remote", true
	case "Híbrido", "Presencial y remoto":
		job.WorkMode = "hybrid"
	case "Presencial":
		job.WorkMode = "onsite"
	}
	currency := ""
	if d.templatedJobPosting {
		job.Countries = countryFromCode(pandapeAnalyticsCountry(root))
		job.EmploymentType = pandapeLatamEmploymentType(c.Contract, c.Schedule)
		if len(job.Countries) == 1 {
			currency = pandapeLatamCurrency(job.Countries[0], c.Salary)
		}
	} else {
		job.EmploymentType = pandapeEmploymentType(c.Contract, ld.EmploymentType)
		if len(ld.JobLocation) > 0 {
			a := ld.JobLocation[0].Address
			// "Todo Brasil" (nationwide) postings state the country and no city; the card's own
			// wording is the better location text there.
			if a.AddressLocality != "" {
				job.Location = joinNonEmpty(a.AddressLocality, a.AddressRegion)
			}
			job.Countries = countryFromCode(a.AddressCountry)
		}
		if job.Countries == nil {
			job.FallbackCountries = []string{"br"}
		}
		if ld.BaseSalary != nil {
			currency = strings.ToUpper(ld.BaseSalary.Currency)
		}
	}
	if b := ld.BaseSalary; b != nil && b.Value.MinValue != nil {
		lo := int(math.Round(*b.Value.MinValue))
		hi := lo
		if b.Value.MaxValue != nil {
			hi = int(math.Round(*b.Value.MaxValue))
		}
		if period := schemaSalaryPeriod(b.Value.UnitText); lo > 0 && hi >= lo && period != "" && currency != "" {
			job.SalaryMin, job.SalaryMax = &lo, &hi
			job.SalaryCurrency, job.SalaryPeriod = currency, period
		}
	}
	return job, job.Title != "" && job.Description != ""
}

// pandapeAnalyticsCountry reads the country the career site tells its own analytics,
// "window.dataLayerProps.country = 'Peru';" — the only place a Latin-American vacancy page
// states its country correctly. It is the tenant's country, spelled in English, its non-ASCII
// letters HTML-escaped inside the script ("Espa&#xF1;a").
var pandapeAnalyticsCountryRe = regexp.MustCompile(`dataLayerProps\.country\s*=\s*'([^']*)'`)

func pandapeAnalyticsCountry(root *xhtml.Node) string {
	var country string
	walk(root, func(n *xhtml.Node) bool {
		if country != "" {
			return false
		}
		if n.Type == xhtml.ElementNode && n.Data == "script" {
			if m := pandapeAnalyticsCountryRe.FindStringSubmatch(textContent(n)); m != nil {
				country = html.UnescapeString(m[1])
			}
			return false
		}
		return true
	})
	return country
}

// pandapeBody renders the page's description and its structured requirement sections
// ("Requisitos", "Valorizado"). The page's HTML is preferred over the JobPosting's plain-text
// description, which the platform writes with every emoji replaced by "?".
func pandapeBody(root *xhtml.Node, ldDescription string) string {
	var b strings.Builder
	if d := firstByID(root, "description"); d != nil {
		if body := firstByClass(d, "order-1"); body != nil {
			b.WriteString(innerHTML(body))
		}
	}
	if strings.TrimSpace(b.String()) == "" && ldDescription != "" {
		b.WriteString("<p>" + strings.ReplaceAll(html.EscapeString(ldDescription), "\n", "<br>") + "</p>")
	}
	for _, id := range []string{"Requirements", "Valued"} {
		if n := firstByID(root, id); n != nil {
			_ = xhtml.Render(&b, n)
		}
	}
	return sanitizeHTML(b.String())
}

// pandapeZone is the zone datePosted is written in: the platform omits the offset, and its
// timestamps read as Brasília time (UTC-3, no daylight saving since 2019).
var pandapeZone = time.FixedZone("BRT", -3*60*60)

// pandapePostedAt parses datePosted ("2026-10-23T15:59:35.1108602", no offset).
func pandapePostedAt(s string) *time.Time {
	t, err := time.ParseInLocation("2006-01-02T15:04:05.9999999", s, pandapeZone)
	if err != nil {
		return nil
	}
	return NotFuture(&t)
}

// pandapeEmploymentType maps the card's contract picklist first — it says whether the
// vacancy is an internship or a contract — and otherwise the JobPosting's schedule, which the
// platform spells "full-time"/"partial-time". "Efetivo – CLT" (a permanent employee) and
// "Jovem Aprendiz" (an apprenticeship) name no schedule of their own.
func pandapeEmploymentType(contract, schedule string) string {
	switch contract {
	case "Estágio":
		return "internship"
	case "Prestador de Serviços (PJ)", "Autônomo", "Temporário":
		return "contract"
	}
	switch schedule {
	case "full-time":
		return "full_time"
	case "partial-time":
		return "part_time"
	}
	return ""
}

// pandapeLatamEmploymentType maps the Latin-American card's contract and schedule picklists
// (the JobPosting's employmentType is a constant there). The contract wins where it says the
// vacancy is not employment at all; the many employment contracts — indefinite, fixed-term,
// per-project ("a plazo fijo", "de obra o labor", Peru's "por Inicio o Incremento de
// Actividad") — say nothing about the schedule, which the schedule picklist states: a named
// full or part time, or a weekly hour count (Chile's statutory week is 40-45 hours).
func pandapeLatamEmploymentType(contract, schedule string) string {
	if contract == "Contrato civil por prestación de servicios" {
		return "contract"
	}
	switch strings.ToLower(schedule) {
	case "tiempo completo", "jornada completa":
		return "full_time"
	case "tiempo parcial", "jornada part time", "por horas", "jornada reducida":
		return "part_time"
	}
	if m := pandapeWeeklyHours.FindStringSubmatch(schedule); m != nil {
		if h, _ := strconv.Atoi(m[1]); h >= 40 {
			return "full_time"
		}
	}
	return ""
}

var pandapeWeeklyHours = regexp.MustCompile(`^Jornada de (\d+) horas semanales$`)

// pandapeLatamCurrencies is the currency a Latin-American tenant's country pays in and the
// symbol its cards write it with ("S/ 1,230", "1.000.000 $"). The JobPosting says BRL for
// every one of them, so the currency is taken from the country — and only when the card's
// salary carries that country's own symbol, so a posting paying in dollars is never recorded
// in pesos. The countries are the ones the tenants measured on 2026-10-24 state.
var pandapeLatamCurrencies = map[string]struct{ code, symbol string }{
	"ar": {"ARS", "$"},
	"cl": {"CLP", "$"},
	"co": {"COP", "$"},
	"mx": {"MXN", "$"},
	"pe": {"PEN", "S/"},
	"uy": {"UYU", "$"},
}

func pandapeLatamCurrency(country, cardSalary string) string {
	cur, ok := pandapeLatamCurrencies[country]
	if !ok || !strings.Contains(cardSalary, cur.symbol) {
		return ""
	}
	for _, foreign := range []string{"US$", "USD", "U$S"} {
		if strings.Contains(cardSalary, foreign) {
			return ""
		}
	}
	return cur.code
}
