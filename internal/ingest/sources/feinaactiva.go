package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// feinaactiva adapts Feina Activa (feinaactiva.gencat.cat), the job board of Catalonia's public
// employment service (SOC): ~5.7k open offers, about two thirds posted by employers and SOC
// offices and a third public-sector calls relayed from the Diputació de Barcelona's CIDO
// bulletin. It is boardless and an aggregator — the employer is read per offer. See the
// "Feina Activa traps" section of AGENTS.md for the measurements behind each rule.
//
// The listing is the site's own JSON API, which carries every offer in full — title, employer,
// place, duties, requirements and conditions — so no posting page is ever bought. It serves ten
// offers a page whatever the request asks for, so a whole walk is ~570 requests; the adapter
// therefore STREAMS each page's offers to the pipeline as it reads them, so a run cut short still
// keeps what it read, and reads ahead while the pipeline writes (see feinaactivaPrefetch).
type feinaactiva struct {
	http     JSONGetter
	limiter  waiter
	maxPages int
}

const (
	feinaactivaBaseURL = "https://feinaactiva.gencat.cat"
	// feinaactivaListURL is the offer search, newest first. The sort is what makes the walk
	// stable: without it the site's default order reshuffles between requests and a full walk
	// read only 4,296 distinct offers of 5,694. Pages are 1-based (page 0 repeats page 1).
	feinaactivaListURL = feinaactivaBaseURL + "/api/offers/list?sort=publishedAt&order=desc&page=%d"
	// feinaactivaOfferURL is an offer's public page, keyed by its reference.
	feinaactivaOfferURL = feinaactivaBaseURL + "/search/offers/detail/%s"

	// feinaactivaRequestInterval paces the walk at one page a second — a courtesy ceiling toward
	// a public service, not a measured limit.
	feinaactivaRequestInterval = time.Second
	// feinaactivaMaxPages is a safety ceiling: ~570 pages on 2026-10-27.
	feinaactivaMaxPages = 1500
)

// NewFeinaActiva builds the adapter over the given client.
func NewFeinaActiva(c JSONGetter) Source {
	return feinaactiva{
		http:     c,
		limiter:  rate.NewLimiter(rate.Every(feinaactivaRequestInterval), 1),
		maxPages: feinaactivaMaxPages,
	}
}

func (feinaactiva) Provider() string { return "feinaactiva" }

// One regional board, no per-tenant id.
func (feinaactiva) boardless() {}

// Thousands of employers, councils and public bodies post here, each named per offer.
func (feinaactiva) aggregator() {}

type feinaactivaPage struct {
	Total    int                `json:"total"`
	Included []feinaactivaOffer `json:"included"`
}

type feinaactivaOffer struct {
	Reference string `json:"reference"`
	Origin    string `json:"origin"`
	Detail    struct {
		Title    string `json:"title"`
		Business struct {
			Name    string `json:"name"`
			HasName bool   `json:"hasName"`
		} `json:"business"`
		PublishedAt  string   `json:"publishedAt"`
		Description  string   `json:"description"`
		Location     string   `json:"location"`
		Job          string   `json:"job"`
		Requirements []string `json:"requirements"`
		Conditions   []string `json:"conditions"`
	} `json:"offerDetail"`
}

func (s feinaactiva) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var (
		mu   sync.Mutex
		jobs []Job
	)
	err := s.FetchStream(ctx, e, func(j Job) {
		mu.Lock()
		jobs = append(jobs, j)
		mu.Unlock()
	})
	return jobs, err
}

// feinaactivaPrefetch is how many listing pages may wait, read, for the pipeline to save them.
// Reading and saving are decoupled because each is slow on its own: a page answers in ~1.5 s
// and its ten offers take ~1.5 s more to write from the crawl host, so a walk that waited on its
// own writes took ~45 minutes against a 50-minute run budget.
const feinaactivaPrefetch = 64

// feinaactivaBatch is one listing page as the walk hands it over: its offers, or the error that
// ended the walk on that page.
type feinaactivaBatch struct {
	page   int
	offers []feinaactivaOffer
	err    error
}

// FetchStream walks the listing newest first and emits each offer as its page arrives. The first
// page failing fails the crawl; a later one ends the walk with what was emitted, since the
// adapter is not swept by source.
func (s feinaactiva) FetchStream(ctx context.Context, _ CompanyEntry, emit func(Job)) error {
	batches := make(chan feinaactivaBatch, feinaactivaPrefetch)
	done := make(chan struct{})
	defer close(done)
	go s.walk(ctx, batches, done)

	seen := make(map[string]bool)
	read := 0
	for b := range batches {
		if b.err != nil {
			if b.page == 1 {
				return fmt.Errorf("feinaactiva: listing: %w", b.err)
			}
			log.Printf("feinaactiva: listing page %d failed; keeping the %d offers read: %v", b.page, read, b.err)
			return nil
		}
		for _, o := range b.offers {
			read++
			if seen[o.Reference] {
				continue
			}
			seen[o.Reference] = true
			if j, ok := feinaactivaJob(o); ok {
				emit(j)
			}
		}
	}
	return nil
}

// walk reads the listing pages in order into batches until a page carries no offer, the
// listing's own total is reached, or a page fails (handed over as that page's error). It stops
// early when the reader is gone.
func (s feinaactiva) walk(ctx context.Context, batches chan<- feinaactivaBatch, done <-chan struct{}) {
	defer close(batches)
	send := func(b feinaactivaBatch) bool {
		select {
		case batches <- b:
			return true
		case <-done:
			return false
		}
	}
	read := 0
	for page := 1; page <= s.maxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			send(feinaactivaBatch{page: page, err: err})
			return
		}
		var p feinaactivaPage
		if err := s.http.GetJSON(ctx, fmt.Sprintf(feinaactivaListURL, page), &p); err != nil {
			send(feinaactivaBatch{page: page, err: err})
			return
		}
		if len(p.Included) == 0 {
			if page == 1 {
				send(feinaactivaBatch{page: page, err: fmt.Errorf("listing carries no offer")})
			}
			return
		}
		if !send(feinaactivaBatch{page: page, offers: p.Included}) {
			return
		}
		read += len(p.Included)
		if p.Total > 0 && read >= p.Total {
			return
		}
	}
}

// feinaactivaJob maps an offer to a Job. ok is false for one with no reference, title or body, and
// for one whose employer is hidden (see feinaactivaHiddenEmployer): filing those under the label
// the site shows would invent one bogus employer collecting every anonymous offer of its kind.
func feinaactivaJob(o feinaactivaOffer) (Job, bool) {
	d := o.Detail
	ref := strings.TrimSpace(o.Reference)
	title := strings.TrimSuffix(feinaactivaClean(d.Title), " CIDO")
	company := feinaactivaClean(d.Business.Name)
	if ref == "" || title == "" || company == "" || !d.Business.HasName || feinaactivaHiddenEmployer(company) {
		return Job{}, false
	}
	body := feinaactivaBody(d.Description, d.Job, d.Requirements, d.Conditions)
	if body == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:        ref,
		URL:               fmt.Sprintf(feinaactivaOfferURL, ref),
		Title:             title,
		Company:           company,
		Description:       body,
		FallbackCountries: []string{"es"},
	}
	if place, ok := feinaactivaPlace(d.Location); ok {
		j.Location = place
		j.Countries = []string{"es"}
	}
	if t, err := time.Parse(time.RFC3339, strings.TrimSpace(d.PublishedAt)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	for _, c := range d.Conditions {
		lc := strings.ToLower(c)
		switch {
		case strings.HasPrefix(lc, "jornada completa"):
			j.EmploymentType = "full_time"
		case strings.HasPrefix(lc, "jornada parcial"):
			j.EmploymentType = "part_time"
		case strings.HasPrefix(lc, "contracte mercantil"):
			j.EmploymentType = "contract"
		}
		if lo, hi, period, ok := feinaactivaSalary(c); ok {
			j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "EUR", period
		}
	}
	return j, true
}

// feinaactivaLegalForm marks a registered company name ("..., S.L.", "SCCL").
var feinaactivaLegalForm = regexp.MustCompile(`(?i)\b(s\.?\s?l\.?u?|s\.?\s?a\.?u?|sccl|s\.?\s?coop)\b\.?$`)

// feinaactivaHiddenEmployer reports whether the name is the site's stand-in for a hidden
// employer. Three shapes stand in for one: hasName=false (checked by the caller); a description
// of the employer in place of its name, always opening with "Empresa" ("Empresa de serveis de
// neteja", "Empresa del sector de les TIC", "Empresa pública") — a registered name opening with
// the word carries its legal form; and "Oficina de Treball del SOC", the public office that posted
// on an employer's behalf.
func feinaactivaHiddenEmployer(name string) bool {
	if strings.HasPrefix(name, "Oficina de Treball") {
		return true
	}
	return strings.HasPrefix(name, "Empresa ") && !feinaactivaLegalForm.MatchString(name)
}

func feinaactivaClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// feinaactivaBody renders the offer's sections as HTML: the summary, the duties ("Veure
// convocatòria" — "see the call" — on a relayed public-sector call says nothing and is dropped),
// the requirements and the conditions. The API's text is plain with line breaks.
func feinaactivaBody(summary, duties string, requirements, conditions []string) string {
	para := func(s string) string {
		s = strings.TrimSpace(s)
		if s == "" {
			return ""
		}
		lines := strings.Split(html.EscapeString(s), "\n")
		for i := range lines {
			lines[i] = strings.TrimSpace(lines[i])
		}
		return "<p>" + strings.Join(lines, "<br>") + "</p>"
	}
	list := func(heading string, items []string) string {
		var lis []string
		for _, it := range items {
			if p := para(it); p != "" {
				lis = append(lis, "<li>"+strings.TrimSuffix(strings.TrimPrefix(p, "<p>"), "</p>")+"</li>")
			}
		}
		if len(lis) == 0 {
			return ""
		}
		return "<h3>" + heading + "</h3><ul>" + strings.Join(lis, "") + "</ul>"
	}
	var b strings.Builder
	b.WriteString(para(summary))
	if !strings.EqualFold(strings.TrimSpace(duties), "Veure convocatòria") && strings.TrimSpace(duties) != strings.TrimSpace(summary) {
		if p := para(duties); p != "" {
			b.WriteString("<h3>Funcions</h3>" + p)
		}
	}
	b.WriteString(list("Requisits", requirements))
	b.WriteString(list("Condicions", conditions))
	return sanitizeHTML(b.String())
}

// feinaactivaPlaceRe reads the API's place, "<PROVINCE> - <COMARCA> - <postcode> <MUNICIPALITY>"
// in capitals ("GIRONA - BAIX EMPORDÀ - 17100 BISBAL D'EMPORDÀ, LA"). The postcode loses its
// leading zero in Barcelona province ("8029").
var feinaactivaPlaceRe = regexp.MustCompile(`^(.+?) - .+? - \d{4,5} (.+)$`)

// feinaactivaPlace renders the place as "<Municipality>, <Province>", restoring the article the
// API moves to the end ("BISBAL D'EMPORDÀ, LA" is La Bisbal d'Empordà). ok is false for a place
// not in that shape.
func feinaactivaPlace(raw string) (string, bool) {
	m := feinaactivaPlaceRe.FindStringSubmatch(strings.TrimSpace(raw))
	if m == nil {
		return "", false
	}
	province, town := strings.TrimSpace(m[1]), strings.TrimSpace(m[2])
	if i := strings.LastIndex(town, ", "); i > 0 {
		town = town[i+2:] + " " + town[:i]
	}
	town, province = feinaactivaTitle(town), feinaactivaTitle(province)
	if strings.EqualFold(town, province) {
		return town, true
	}
	return town + ", " + province, true
}

// feinaactivaLower are the particles a Catalan or Spanish place name keeps in lower case.
var feinaactivaLower = map[string]bool{"de": true, "del": true, "dels": true, "i": true, "la": true, "les": true, "el": true, "els": true, "y": true}

// feinaactivaTitle title-cases a place written in capitals, keeping particles and the elided
// article ("d'", "l'") lower case: "BISBAL D'EMPORDÀ" is "Bisbal d'Empordà".
func feinaactivaTitle(s string) string {
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && feinaactivaLower[w] {
			continue
		}
		prefix := ""
		if len(w) > 2 && (strings.HasPrefix(w, "d'") || strings.HasPrefix(w, "l'")) {
			prefix, w = w[:2], w[2:]
			if i == 0 {
				prefix = strings.ToUpper(prefix[:1]) + prefix[1:]
			}
		}
		r := []rune(w)
		if len(r) > 0 {
			r[0] = []rune(strings.ToUpper(string(r[0])))[0]
		}
		words[i] = prefix + string(r)
	}
	return strings.Join(words, " ")
}

// feinaactivaSalaryRe reads a salary condition: "Salari mensual brut des de '1800' fins a '2500'"
// or "Salari mensual brut 1695.43".
var feinaactivaSalaryRe = regexp.MustCompile(`(?i)^salari (mensual|anual) brut (?:des de '([\d.,]+)' fins a '([\d.,]+)'|([\d.,]+))`)

func feinaactivaSalary(c string) (lo, hi int, period string, ok bool) {
	m := feinaactivaSalaryRe.FindStringSubmatch(strings.TrimSpace(c))
	if m == nil {
		return 0, 0, "", false
	}
	num := func(s string) int {
		s = strings.ReplaceAll(s, ",", ".")
		f, err := strconv.ParseFloat(s, 64)
		if err != nil {
			return 0
		}
		return int(f)
	}
	if m[4] != "" {
		lo = num(m[4])
		hi = lo
	} else {
		lo, hi = num(m[2]), num(m[3])
	}
	if lo <= 0 || hi < lo {
		return 0, 0, "", false
	}
	period = "month"
	if strings.EqualFold(m[1], "anual") {
		period = "year"
	}
	return lo, hi, period, true
}
