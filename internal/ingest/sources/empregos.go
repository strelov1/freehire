package sources

import (
	"cmp"
	"context"
	"fmt"
	"html"
	"log"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// empregos adapts Empregos.com.br, a general Brazilian job board: ~90k open postings from
// thousands of employers and staffing agencies, nearly all of them outside IT. It is boardless
// and an aggregator — the employer is the company each posting names. See the "Empregos.com.br
// traps" section of AGENTS.md for the measurements behind each rule.
//
// The crawl reads the search API the site's own pages call, which answers a whole posting —
// body included — per result, so no posting page is ever requested. Paging that API loses
// postings (a posting crosses a page boundary and is answered twice while another is never
// answered), so the catalogue is cut into slices small enough to arrive in ONE answer each.
type empregos struct {
	http         JSONPoster
	limiter      waiter
	leafMax      int
	maxNewPerRun int
}

const (
	empregosSearchURL  = "https://gatewayb2c.empregos.com.br/searchjob/v2/elastic/search"
	empregosPostingURL = "https://www.empregos.com.br/vaga/"
)

// empregosRequestInterval paces the run's requests through one limiter. The answers are slow
// (2-12 s each) and requested one at a time, so latency sets the real pace; the interval only
// keeps a burst of small answers from arriving back to back. No refusal was seen at it.
const empregosRequestInterval = time.Second

// empregosLeafMax is how many postings one answer may carry. A slice that reports more is cut
// further; a slice that cannot be cut further fails the crawl. The largest slice the cuts leave
// measured 10,356 postings (2026-10-24), and an answer of 15,000 is ~35 MB uncompressed —
// under the client's 64 MiB response cap and, at ~12 s, inside the long-timeout transport.
const empregosLeafMax = 15000

// empregosMaxNewPerRun bounds how many unseen postings one run writes. The crawl itself costs
// ~100 requests whatever the catalogue holds; what grows with it is the database work, measured
// from the crawl host on 2026-10-24: a new posting's write ran at ~7 a second (14,119 in the
// ~36 minutes before the first run's deadline), a stored posting's refresh at ~190 a second.
// Against the run's 40-minute crawl budget (50 minutes less the sweep reserve), a run at steady
// state spends ~4 minutes listing and ~8 refreshing ~90k stored postings, and 5,000 new writes
// add ~12 — about 60% of the budget. The newest are written first; the rest wait for later runs.
const empregosMaxNewPerRun = 5000

// The search's facet ids, as its own pages send them.
const (
	empregosFacetState     = 1
	empregosFacetLevel     = 10
	empregosFacetWorkplace = 17
)

// empregosStates are Brazil's 27 federative units, in the lowercase form the State filter
// takes ("SP" matches nothing). The State facet lists only its top 25, so the closed list is
// enumerated rather than read from it.
var empregosStates = []string{
	"ac", "al", "ap", "am", "ba", "ce", "df", "es", "go", "ma", "mt", "ms", "mg", "pa",
	"pb", "pr", "pe", "pi", "rj", "rn", "rs", "ro", "rr", "sc", "sp", "se", "to",
}

// empregosZone is the clock the posting timestamps are written in: Brasília time, which has
// kept UTC-3 all year since Brazil dropped daylight saving in 2019. The API states no offset.
var empregosZone = time.FixedZone("BRT", -3*3600)

// NewEmpregos builds the adapter over a JSON client with a timeout long enough for the
// largest answer (see empregosLeafMax).
func NewEmpregos(c JSONPoster) Source {
	return empregos{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(empregosRequestInterval), 1),
		leafMax:      empregosLeafMax,
		maxNewPerRun: empregosMaxNewPerRun,
	}
}

func (empregos) Provider() string { return "empregos" }

// One national board, no per-tenant id.
func (empregos) boardless() {}

// Thousands of employers post here, each named per posting.
func (empregos) aggregator() {}

type empregosFilter struct {
	FacetItem   int    `json:"facetItem"`
	Description string `json:"description"`
}

type empregosRequest struct {
	Keyword  string           `json:"keyword"`
	Filters  []empregosFilter `json:"filters"`
	Page     int              `json:"page"`
	Size     int              `json:"size"`
	Order    int              `json:"order"`
	Distance int              `json:"distance"`
}

type empregosResponse struct {
	Total    int               `json:"total"`
	Postings []empregosPosting `json:"jobPostingCollection"`
	Facets   []empregosFacet   `json:"facetCollection"`
}

type empregosFacet struct {
	FacetItem int                 `json:"facetItem"`
	Items     []empregosFacetItem `json:"itens"`
}

type empregosFacetItem struct {
	Value string `json:"value"`
	Count int    `json:"count"`
}

type empregosPosting struct {
	ReferenceID           int64  `json:"referenceId"`
	Title                 string `json:"title"`
	Description           string `json:"description"`
	RequiredQualification string `json:"requiredQualification"`
	AcademicFormation     string `json:"academicFormation"`
	WorkTime              string `json:"workTime"`
	WorkplaceTypes        string `json:"workplaceTypes"`
	ContractForm          string `json:"contractForm"`
	CreatedAt             string `json:"createdAt"`
	Location              []struct {
		City              string `json:"city"`
		StateAbbreviation string `json:"stateAbbreviation"`
		Country           string `json:"country"`
	} `json:"location"`
	Company *empregosCompany `json:"company"`
}

type empregosCompany struct {
	Name string `json:"name"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s empregos) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the catalogue, refreshes the postings already stored and writes up to
// maxNewPerRun of the rest, newest first. Nothing here costs a request per posting — the
// listing carries the body — so the bound is on the run's database work, not on its requests.
func (s empregos) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var fresh []empregosPosting
	var refreshed []Job
	for _, p := range postings {
		company := p.company()
		if company == "" {
			continue
		}
		id := strconv.FormatInt(p.ReferenceID, 10)
		if seen(id) {
			refreshed = append(refreshed, Job{ExternalID: id, URL: empregosPostingURL + id, Title: strings.TrimSpace(p.Title), Company: company, SeenRefresh: true})
			continue
		}
		fresh = append(fresh, p)
	}
	slices.SortStableFunc(fresh, func(a, b empregosPosting) int { return cmp.Compare(b.CreatedAt, a.CreatedAt) })
	if len(fresh) > s.maxNewPerRun {
		log.Printf("empregos: %d new postings, writing the newest %d; the rest wait for a later run", len(fresh), s.maxNewPerRun)
		fresh = fresh[:s.maxNewPerRun]
	}
	// The refreshes go first: they are cheap and they are what keeps the stored postings open,
	// so a run cut short by its deadline loses new writes rather than liveness.
	for _, p := range fresh {
		refreshed = append(refreshed, p.job())
	}
	return refreshed, nil
}

// empregosSlice is one cut of a slice: the filter that selects it and, when the parent's
// facet said so, how many postings it holds (-1 when nothing said).
type empregosSlice struct {
	filter empregosFilter
	count  int
}

// empregosCut splits a slice too large for one answer, from that slice's own answer.
type empregosCut func(parent empregosResponse) ([]empregosSlice, error)

// list reads every posting the cuts reach: by level, then by state, then by workplace type.
// A posting with no location is in no state, so a level too large for one answer loses its
// location-less postings (~0.1% of the catalogue, measured 2026-10-24) — which is why this
// adapter does not claim fullCatalog. The shortfall is logged rather than failed on.
func (s empregos) list(ctx context.Context) ([]empregosPosting, error) {
	byID := make(map[int64]empregosPosting)
	cuts := []empregosCut{
		empregosFacetCut(empregosFacetLevel),
		empregosStateCut,
		empregosFacetCut(empregosFacetWorkplace),
	}
	total, err := s.collect(ctx, nil, math.MaxInt, cuts, byID)
	if err != nil {
		return nil, err
	}
	if len(byID) < total {
		log.Printf("empregos: read %d distinct postings of the %d the search reports; the rest name no state", len(byID), total)
	}
	out := make([]empregosPosting, 0, len(byID))
	for _, p := range byID {
		out = append(out, p)
	}
	slices.SortFunc(out, func(a, b empregosPosting) int { return cmp.Compare(a.ReferenceID, b.ReferenceID) })
	return out, nil
}

// collect reads the slice the filters select and returns the total the search reported for
// it. A slice known to exceed one answer is asked only for its counts; any other is asked for
// all of it, and is cut further only when the answer reports more than it carries.
func (s empregos) collect(ctx context.Context, filters []empregosFilter, known int, cuts []empregosCut, out map[int64]empregosPosting) (int, error) {
	size := s.leafMax
	if known > s.leafMax {
		size = 1
	}
	resp, err := s.search(ctx, filters, size)
	if err != nil {
		return 0, err
	}
	if len(resp.Postings) >= resp.Total {
		for _, p := range resp.Postings {
			out[p.ReferenceID] = p
		}
		return resp.Total, nil
	}
	if resp.Total <= s.leafMax {
		if size == 1 {
			// The slice shrank below one answer since its count was read.
			return s.collect(ctx, filters, resp.Total, cuts, out)
		}
		return 0, fmt.Errorf("empregos: %s answered %d of the %d postings it reports", empregosDescribe(filters), len(resp.Postings), resp.Total)
	}
	if len(cuts) == 0 {
		return 0, fmt.Errorf("empregos: %s holds %d postings, more than one answer carries (%d), and nothing is left to cut it by", empregosDescribe(filters), resp.Total, s.leafMax)
	}
	subs, err := cuts[0](resp)
	if err != nil {
		return 0, fmt.Errorf("empregos: %s: %w", empregosDescribe(filters), err)
	}
	for _, sub := range subs {
		if _, err := s.collect(ctx, append(slices.Clip(filters), sub.filter), sub.count, cuts[1:], out); err != nil {
			return 0, err
		}
	}
	return resp.Total, nil
}

// empregosFacetCut cuts a slice by one facet's values. The facet lists at most 25 values, so
// its counts must add up to the slice's total — anything less means values were cut off, and
// the postings under them would be silently skipped.
func empregosFacetCut(facet int) empregosCut {
	return func(parent empregosResponse) ([]empregosSlice, error) {
		for _, f := range parent.Facets {
			if f.FacetItem != facet {
				continue
			}
			var subs []empregosSlice
			sum := 0
			for _, it := range f.Items {
				if it.Count > 0 {
					subs = append(subs, empregosSlice{empregosFilter{facet, it.Value}, it.Count})
					sum += it.Count
				}
			}
			if sum != parent.Total {
				return nil, fmt.Errorf("facet %d lists %d of the slice's %d postings", facet, sum, parent.Total)
			}
			return subs, nil
		}
		return nil, fmt.Errorf("the answer carries no facet %d", facet)
	}
}

// empregosStateCut cuts a slice by every federative unit. A posting listed in several states
// is answered once per state; the caller keys by id.
func empregosStateCut(empregosResponse) ([]empregosSlice, error) {
	subs := make([]empregosSlice, len(empregosStates))
	for i, uf := range empregosStates {
		subs[i] = empregosSlice{empregosFilter{empregosFacetState, uf}, -1}
	}
	return subs, nil
}

func empregosDescribe(filters []empregosFilter) string {
	if len(filters) == 0 {
		return "the catalogue"
	}
	parts := make([]string, len(filters))
	for i, f := range filters {
		parts[i] = fmt.Sprintf("%d=%q", f.FacetItem, f.Description)
	}
	return "slice " + strings.Join(parts, " ")
}

// search asks for the first page of a slice through the run's limiter.
func (s empregos) search(ctx context.Context, filters []empregosFilter, size int) (empregosResponse, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return empregosResponse{}, err
	}
	if filters == nil {
		filters = []empregosFilter{} // the API wants [], not null
	}
	req := empregosRequest{Filters: filters, Size: size, Distance: 100}
	var resp empregosResponse
	if err := s.http.PostJSON(ctx, empregosSearchURL, req, &resp); err != nil {
		return empregosResponse{}, fmt.Errorf("empregos: search %s: %w", empregosDescribe(filters), err)
	}
	return resp, nil
}

// company is the employer the posting names, or "" when it names none. "Confidencial" and
// "Empresa confidencial" (~7% of postings) are the board's label for a hidden employer, not an
// employer, so such a posting is skipped like an unnamed one: there is nobody to file it under.
func (p empregosPosting) company() string {
	if p.Company == nil {
		return ""
	}
	name := strings.TrimSpace(p.Company.Name)
	if strings.EqualFold(name, "Confidencial") || strings.EqualFold(name, "Empresa confidencial") {
		return ""
	}
	return name
}

func (p empregosPosting) job() Job {
	id := strconv.FormatInt(p.ReferenceID, 10)
	j := Job{
		ExternalID:     id,
		URL:            empregosPostingURL + id,
		Title:          strings.TrimSpace(p.Title),
		Company:        p.company(),
		Description:    p.body(),
		WorkMode:       empregosWorkMode(p.WorkplaceTypes),
		EmploymentType: empregosEmploymentType(p.ContractForm),
	}
	j.Remote = j.WorkMode == "remote"
	var places []string
	for _, l := range p.Location {
		if place := joinNonEmpty(strings.TrimSpace(l.City), strings.TrimSpace(l.StateAbbreviation)); place != "" && !slices.Contains(places, place) {
			places = append(places, place)
		}
		if c := location.NormalizeCountry(l.Country); c != "" && !slices.Contains(j.Countries, c) {
			j.Countries = append(j.Countries, c)
		}
	}
	j.Location = strings.Join(places, "; ")
	if t, err := time.ParseInLocation("2006-01-02T15:04:05", p.CreatedAt, empregosZone); err == nil {
		j.PostedAt = &t
	}
	return j
}

// body is the description followed by the structured fields an employer filled in beside it.
// The description is HTML on most postings and newline-separated plain text (with HTML
// entities) on the rest.
func (p empregosPosting) body() string {
	var b strings.Builder
	b.WriteString(empregosHTML(p.Description))
	for _, section := range []struct{ label, text string }{
		{"Requisitos", p.RequiredQualification},
		{"Formação", p.AcademicFormation},
		{"Horário", p.WorkTime},
	} {
		if strings.TrimSpace(section.text) != "" {
			b.WriteString("<h3>" + section.label + "</h3>" + empregosHTML(section.text))
		}
	}
	return sanitizeHTML(b.String())
}

func empregosHTML(s string) string {
	if strings.Contains(s, "<") {
		return s
	}
	return plainTextToHTML(html.UnescapeString(s))
}

func empregosWorkMode(t string) string {
	switch t {
	case "Presencial":
		return "onsite"
	case "Remoto":
		return "remote"
	case "Híbrido":
		return "hybrid"
	}
	return ""
}

// empregosEmploymentType maps the contract form where it names one. "A Combinar" (to be
// agreed) is on 99.96% of postings and says nothing; "Efetivo (CLT)" is the employment regime,
// not the schedule.
func empregosEmploymentType(form string) string {
	switch form {
	case "Estágio":
		return "internship"
	case "Prestador de serviços (PJ)", "Autônomo":
		return "contract"
	}
	return ""
}
