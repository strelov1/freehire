package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"
	"unicode"

	"golang.org/x/time/rate"
)

// ess adapts the job search of the Employment Service of Slovenia (Zavod Republike Slovenije za
// zaposlovanje, www.ess.gov.si): every vacancy a Slovenian employer registers with the public
// service, ~3.6k open at a time, every trade, mostly local employers and the public sector. It
// is boardless and an aggregator — the employer is read per posting. See the "ESS traps" section
// of AGENTS.md for the measurements behind each rule.
//
// The site's search page is an Angular app over a public JSON API behind a 3scale gateway. The
// listing endpoint pages the whole Slovenian catalogue with each vacancy's title, employer,
// place and dates; the body is only in the per-vacancy detail endpoint, bought for vacancies the
// catalogue lacks, newest first, within a per-run budget. Two more listing slices mark the
// vacancies the site files under IT, and the part-time ones.
type ess struct {
	http           essHTTP
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

// essHTTP is the transport the adapter needs: the listing is a JSON POST, the detail a JSON GET.
type essHTTP interface {
	JSONPoster
	JSONGetter
}

const (
	essAPIBase    = "https://apigateway-prod-www-prod.apps.ess.gov.si/iskalnik-po-pdm/v1/delovno-mesto"
	essListingURL = essAPIBase + "/prosta-delovna-mesta-filtri"
	essDetailURL  = essAPIBase + "/podrobnosti-prosto-delovno-mesto"
	// essUserKey is the 3scale application key the search page itself sends with every request:
	// public, embedded in the page's own script (ue_dataFor3ScaleApi_userKey), not a credential
	// issued to us. A rotated key answers every request with an error, which fails the crawl.
	essUserKey = "9b7dcbe8ec1855d14f0b2ec4f6335a91" // gitleaks:allow — public key served in the site's own JS
	// essPostingURL opens the vacancy's detail panel on the public search page; the server
	// renders the vacancy's title and duties into the page's og: tags for it.
	essPostingURL = "https://www.ess.gov.si/iskalci-zaposlitve/iskanje-zaposlitve/iskanje-dela/?idp="

	// essPageSize is the listing page size. The API caps a page at 2,000; a page of 500 answered
	// in ~8 s (2026-10-29), so the long-timeout client is used and a full walk is 8 pages.
	essPageSize = 500
	// essMaxPages is a ceiling far past the catalogue's size (~3.6k = 8 pages); reaching it
	// before the listing's end fails the crawl rather than accepting a truncated walk.
	essMaxPages = 40
	// essSortNewest is the listing's "Po datumu objave" order (newest first); "0" is relevance.
	essSortNewest = 7
	// essITField is the occupational field "Računalništvo, programiranje, IT".
	essITField = "0020"
	// essPartTime is the working-time filter "Krajši delovni čas"; "1" is full time. The two
	// partition the catalogue exactly (3,419 + 202 = 3,621 on 2026-10-29).
	essPartTime = "2"

	// essRequestInterval paces every request of a run, listing and detail alike, at one a
	// second — a courtesy ceiling toward a public service, not a measured limit.
	essRequestInterval = time.Second
	essDetailWorkers   = 2
	// essMaxNewPerRun bounds the detail requests one run buys: 1,200 at one a second is 20
	// minutes, plus ~10 listing requests and the writes that follow, inside the scheduler's
	// default 2,400 s crawl budget. ~170 vacancies arrive each weekday, so a cold catalogue
	// (~3.6k) is taken in three runs, newest first, and every later run keeps up.
	essMaxNewPerRun   = 1200
	essDeadlineMargin = 5 * time.Minute

	// essSalaryMin and essSalaryMax bound a monthly gross pay that reads as a real figure.
	essSalaryMin = 500
	essSalaryMax = 20000
)

// NewESS builds the adapter over the given client. A listing page takes several seconds to
// answer, so the registry hands it the long-timeout client.
func NewESS(c essHTTP) Source {
	return ess{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(essRequestInterval), 1),
		maxNewPerRun:   essMaxNewPerRun,
		deadlineMargin: essDeadlineMargin,
	}
}

func (ess) Provider() string { return "ess" }

// One national exchange, no per-tenant id.
func (ess) boardless() {}

// Thousands of employers post here, each named per vacancy.
func (ess) aggregator() {}

// essSearchRequest is the listing query the search page posts. Every filter must be present: an
// absent array is not read as "no filter", and the sort must be a number.
type essSearchRequest struct {
	Title          string   `json:"nazivDelovnegaMesta"`
	Place          string   `json:"lokacija"`
	Countries      []string `json:"drzave"`
	Published      string   `json:"datumObjave"`
	Fields         []string `json:"poklicnaPodrocja"`
	Regions        []string `json:"regije"`
	References     []string `json:"referenceDelodajalca"`
	Education      []string `json:"ravniIzobrazbe"`
	Schedule       []string `json:"urnikDela"`
	EducationAreas []string `json:"podrocjaIzobrazbe"`
	Languages      []string `json:"jeziki"`
	WorkingTime    string   `json:"delovniCas"`
	Page           int      `json:"stran"`
	PageSize       int      `json:"stZadetkov"`
	ReturnFilters  bool     `json:"vrniFiltre"`
	Sort           int      `json:"urejevalniPojem"`
}

// essSlice narrows the listing to one occupational field or one working time; the zero value is
// the whole Slovenian catalogue.
type essSlice struct {
	field       string
	workingTime string
}

func essSearchBody(slice essSlice, page int) essSearchRequest {
	fields := []string{}
	if slice.field != "" {
		fields = []string{slice.field}
	}
	return essSearchRequest{
		Countries:      []string{"SI"},
		Fields:         fields,
		Regions:        []string{},
		References:     []string{},
		Education:      []string{},
		Schedule:       []string{},
		EducationAreas: []string{},
		Languages:      []string{},
		WorkingTime:    slice.workingTime,
		Page:           page,
		PageSize:       essPageSize,
		Sort:           essSortNewest,
	}
}

// essSearchResponse is one listing page. Total is the size of the whole result, restated on
// every page.
type essSearchResponse struct {
	Total    *int      `json:"steviloDelovnihMest"`
	Postings []essCard `json:"seznamDelovnihMest"`
}

// essCard is one vacancy as the listing states it.
type essCard struct {
	ID       string `json:"idDelovnoMesto"`
	Title    string `json:"nazivDelovnegaMesta"`
	Employer string `json:"delodajalec"`
	Place    string `json:"krajDM"`
	Posted   string `json:"datumObjave"`
}

// essPosting is a listed vacancy with what the slices said about it.
type essPosting struct {
	card     essCard
	tech     bool
	partTime *bool
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s ess) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s ess) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists every Slovenian vacancy, refreshes the ones already stored, and reads up to
// maxNewPerRun of the rest, newest first. The listing names the employer, so a vacancy the
// pipeline will discard anyway (a covered employer, a rejected title) is yielded body-less
// without a request. A vacancy whose detail cannot be read is not yielded; a later run tries
// again.
func (s ess) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(postings))
		for _, p := range postings {
			companies = append(companies, essClean(p.card.Employer))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []essPosting
	deferred := 0
	for _, p := range postings {
		base := essBaseJob(p)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
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
	read := fetchDetails(unseen, essDetailWorkers, func(p essPosting) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("ess: vacancy %s failed; deferring: %v", p.card.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no detail at all is broken, not quiet. "Answered" counts a vacancy the
	// API reported gone; vacancies left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("ess: requested %d vacancy details and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("ess: %d new vacancies left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("ess: %d new vacancies past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s ess) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the whole catalogue, newest first, then the IT and part-time slices. Any page of the
// catalogue failing fails the crawl: a partial listing would let the company-scoped sweep close
// the vacancies of employers it did see on the pages it did not. A slice only adds a mark, so
// its failing costs the mark, not the crawl — and a failed part-time slice leaves the working
// time unstated rather than calling every vacancy full time.
func (s ess) list(ctx context.Context) ([]essPosting, error) {
	cards, err := s.walk(ctx, essSlice{})
	if err != nil {
		return nil, err
	}
	if len(cards) == 0 {
		return nil, fmt.Errorf("ess: the listing names no vacancy")
	}
	tech := map[string]bool{}
	if it, err := s.walk(ctx, essSlice{field: essITField}); err != nil {
		log.Printf("ess: IT field slice failed; no vacancy is marked technical this run: %v", err)
	} else {
		for _, c := range it {
			tech[c.ID] = true
		}
	}
	var partTime map[string]bool
	if pt, err := s.walk(ctx, essSlice{workingTime: essPartTime}); err != nil {
		log.Printf("ess: part-time slice failed; no working time is stated this run: %v", err)
	} else {
		partTime = make(map[string]bool, len(pt))
		for _, c := range pt {
			partTime[c.ID] = true
		}
	}
	out := make([]essPosting, 0, len(cards))
	for _, c := range cards {
		p := essPosting{card: c, tech: tech[c.ID]}
		if partTime != nil {
			v := partTime[c.ID]
			p.partTime = &v
		}
		out = append(out, p)
	}
	return out, nil
}

// walk reads one slice of the listing to its end: until a page comes back empty or the distinct
// vacancies read reach the total the listing states. Any page failing fails the walk, and so
// does reaching essMaxPages first. Vacancies are kept once each, in listing order.
func (s ess) walk(ctx context.Context, slice essSlice) ([]essCard, error) {
	seenIDs := map[string]bool{}
	var out []essCard
	for page := 1; page <= essMaxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var resp essSearchResponse
		if err := s.http.PostJSON(ctx, essListingURL+"?user_key="+essUserKey, essSearchBody(slice, page), &resp); err != nil {
			return nil, fmt.Errorf("ess: listing page %d: %w", page, err)
		}
		if resp.Total == nil {
			return nil, errListingAbsent("ess", "", "steviloDelovnihMest")
		}
		for _, c := range resp.Postings {
			c.ID = strings.TrimSpace(c.ID)
			if c.ID == "" || seenIDs[c.ID] {
				continue
			}
			seenIDs[c.ID] = true
			out = append(out, c)
		}
		if len(resp.Postings) == 0 || len(out) >= *resp.Total {
			return out, nil
		}
	}
	return nil, fmt.Errorf("ess: listing still not at its end after %d pages", essMaxPages)
}

// essBaseJob is the body-less Job a vacancy's listing card supports.
func essBaseJob(p essPosting) Job {
	j := Job{
		ExternalID: p.card.ID,
		URL:        essPostingURL + p.card.ID,
		Title:      essTitle(p.card.Title),
		Company:    essClean(p.card.Employer),
		Location:   essPlace(p.card.Place),
		PostedAt:   NotFuture(parseRFC3339(strings.TrimSpace(p.card.Posted))),
		IsTechHint: p.tech,
		// The listing is queried for Slovenia alone (drzave=SI), so the country is the query's,
		// not a reading of the place name.
		Countries: []string{"si"},
	}
	if p.partTime != nil {
		j.EmploymentType = "full_time"
		if *p.partTime {
			j.EmploymentType = "part_time"
		}
	}
	return j
}

// essDetail is the part of a vacancy's detail record the adapter reads. An id the API no longer
// holds (and one that never existed) answers HTTP 200 with the id and a view count alone.
type essDetail struct {
	Title            string `json:"nazivDelovnegaMesta"`
	Duties           string `json:"opisDelInNalog"`
	Expectations     string `json:"ostalaPricakovanja"`
	OtherConditions  string `json:"drugiPogoji"`
	Competences      string `json:"kompetenceInZnanja"`
	Education        string `json:"izobrazba"`
	AltEducation     string `json:"alternativnaIzobrazba"`
	Experience       string `json:"delovneIzkusnje"`
	Languages        string `json:"znanjeJezikov"`
	ComputerSkills   string `json:"racunalniskaZnanja"`
	DrivingLicence   string `json:"vozniskoDovoljenje"`
	Duration         string `json:"trajanjeZaposlitve"`
	Probation        string `json:"poskusnoDelo"`
	WorkingTime      string `json:"delovniCas"`
	Schedule         string `json:"urnikDela"`
	Openings         string `json:"steviloDelovnihMest"`
	Salary           string `json:"okvirnaPlaca"`
	HowToApply       string `json:"nacinPrijaveKandidata"`
	Other            string `json:"ostalo"`
	ApplicationsTill string `json:"prijavaDo"`
}

// detail reads one vacancy's detail record. ok=false with a nil error is the API's answer that
// the vacancy is gone; an error is the absence of an answer.
func (s ess) detail(ctx context.Context, p essPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	var d essDetail
	u := essDetailURL + "?user_key=" + essUserKey + "&idDelovnoMesto=" + p.card.ID
	if err := s.http.GetJSON(ctx, u, &d); err != nil {
		return Job{}, false, err
	}
	job, ok := essJob(d, p)
	return job, ok, nil
}

// essJob maps a detail record and its listing card to a Job. ok is false for a record with no
// title or body, which is how the API answers for a vacancy it no longer holds.
func essJob(d essDetail, p essPosting) (Job, bool) {
	j := essBaseJob(p)
	if strings.TrimSpace(d.Title) == "" {
		return Job{}, false
	}
	j.Description = essBody(d)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	if pay, ok := essSalary(d.Salary); ok {
		j.SalaryMin, j.SalaryMax = &pay, &pay
		j.SalaryCurrency = "EUR"
		j.SalaryPeriod = "month"
	}
	return j, true
}

// essBodyFields are the record's facts rendered under the duties, labelled as the site labels
// them. The contact block (kontaktZaKandidata: a named person, a phone, an e-mail) is a person's
// card, not part of the posting, and is never copied.
func essBodyFields(d essDetail) []struct{ label, value string } {
	return []struct{ label, value string }{
		{"Izobrazba", d.Education},
		{"Alternativna izobrazba", d.AltEducation},
		{"Delovne izkušnje", d.Experience},
		{"Znanje jezikov", d.Languages},
		{"Računalniška znanja", d.ComputerSkills},
		{"Vozniško dovoljenje", d.DrivingLicence},
		{"Trajanje zaposlitve", d.Duration},
		{"Poskusno delo", d.Probation},
		{"Delovni čas", d.WorkingTime},
		{"Urnik dela", d.Schedule},
		{"Število delovnih mest", d.Openings},
		{"Okvirna plača", d.Salary},
		{"Prijave do", d.ApplicationsTill},
	}
}

// essBody builds the body: the duties, the expectations and other conditions, the competences,
// a list of the record's facts, then how to apply. Every field is plain text that may carry
// HTML entities and the odd anchor, so each is read as text and escaped again.
func essBody(d essDetail) string {
	var b strings.Builder
	section := func(heading, text string) {
		if text = essText(text); text == "" {
			return
		}
		if heading != "" {
			b.WriteString("<h3>" + html.EscapeString(heading) + "</h3>")
		}
		b.WriteString(essParagraphs(text))
	}
	section("", d.Duties)
	section("Pričakovanja", d.Expectations)
	section("Drugi pogoji", d.OtherConditions)
	section("Kompetence in znanja", d.Competences)
	var facts []string
	for _, f := range essBodyFields(d) {
		if v := essText(f.value); v != "" {
			facts = append(facts, "<li>"+html.EscapeString(f.label)+": "+html.EscapeString(v)+"</li>")
		}
	}
	if len(facts) > 0 {
		b.WriteString("<ul>" + strings.Join(facts, "") + "</ul>")
	}
	section("Način prijave", d.HowToApply)
	section("Ostalo", d.Other)
	return sanitizeHTML(b.String())
}

// essTags matches markup inside a field; essSpaces a run of whitespace.
var (
	essTags   = regexp.MustCompile(`<[^>]*>`)
	essSpaces = regexp.MustCompile(`\s+`)
)

// essText reads one field as plain text: entities decoded (a field may be escaped twice), markup
// dropped, whitespace collapsed.
func essText(s string) string {
	s = html.UnescapeString(html.UnescapeString(s))
	s = essTags.ReplaceAllString(s, " ")
	return strings.TrimSpace(essSpaces.ReplaceAllString(s, " "))
}

// essDash matches a dash that opens an item of an inline list: at the start, or after a space,
// and followed by a word ("-izvajanje zdravstvene nege -priprava terapije"). A hyphenated word
// ("medicinsko-tehničnih") has no space before its hyphen and is left alone.
var essDash = regexp.MustCompile(`(?:^|\s)-\s*`)

// essParagraphs renders a field. Employers write a list on one line, each item opened by a dash;
// a field that starts with one is rendered as a list, anything else as a paragraph.
func essParagraphs(text string) string {
	if !strings.HasPrefix(text, "-") {
		return "<p>" + html.EscapeString(text) + "</p>"
	}
	var items []string
	for _, it := range essDash.Split(text, -1) {
		if it = strings.TrimSpace(it); it != "" {
			items = append(items, "<li>"+html.EscapeString(it)+"</li>")
		}
	}
	if len(items) == 0 {
		return ""
	}
	return "<ul>" + strings.Join(items, "") + "</ul>"
}

// essGenderMarker is the "- M/Ž" (male/female) every title carries, as the Slovenian rules on
// equal treatment require of a vacancy notice. It says nothing about the role.
var essGenderMarker = regexp.MustCompile(`(?i)\s*[-–(]?\s*m\s*/\s*ž\s*\)?\s*$`)

// essTitle cleans a listing title: the trailing gender marker removed. The capitals most titles
// are written in are kept — a roman-numeral pay grade ("PROGRAMER APLIKACIJ VII/2") does not
// survive case folding.
func essTitle(s string) string {
	return strings.TrimSpace(essGenderMarker.ReplaceAllString(essClean(s), ""))
}

func essClean(s string) string { return strings.Join(strings.Fields(html.UnescapeString(s)), " ") }

// essPlaceLower are the connecting words of a Slovenian place name, written lower case inside
// it ("Šmarje pri Jelšah", "Radlje ob Dravi").
var essPlaceLower = map[string]bool{"pri": true, "ob": true, "na": true, "v": true, "nad": true, "pod": true}

// essPlace turns the listing's place, an administrative unit in capitals ("NOVA GORICA",
// "ŠMARJE PRI JELŠAH"), into a readable name; a value already in mixed case is kept.
func essPlace(s string) string {
	s = essClean(s)
	if s != strings.ToUpper(s) {
		return s
	}
	words := strings.Fields(strings.ToLower(s))
	for i, w := range words {
		if i > 0 && essPlaceLower[w] {
			continue
		}
		words[i] = essCapitalize(w)
	}
	return strings.Join(words, " ")
}

// essCapitalize upper-cases the first letter of a word and of each part after a hyphen.
func essCapitalize(w string) string {
	rs := []rune(w)
	start := true
	for i, r := range rs {
		if start {
			rs[i] = unicode.ToUpper(r)
		}
		start = r == '-'
	}
	return string(rs)
}

// essPay reads "1.500,00 EUR bruto mesečno": a monthly gross pay in euros, Slovenian digit
// grouping.
var essPay = regexp.MustCompile(`^([0-9]{1,3}(?:\.[0-9]{3})*|[0-9]+)(?:,[0-9]{1,2})?\s*EUR\s+bruto\s+mesečno$`)

// essSalary reads the record's indicative pay, kept only within the bounds of a real monthly
// figure.
func essSalary(s string) (int, bool) {
	m := essPay.FindStringSubmatch(strings.TrimSpace(essText(s)))
	if m == nil {
		return 0, false
	}
	pay, err := strconv.Atoi(strings.ReplaceAll(m[1], ".", ""))
	if err != nil || pay < essSalaryMin || pay > essSalaryMax {
		return 0, false
	}
	return pay, true
}
