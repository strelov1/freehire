package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// mojposao adapts MojPosao (mojposao.hr), Croatia's largest commercial job board, run by Alma
// Career on its "borg" platform. Every trade, ~6% IT. It is boardless and an aggregator — the
// employer is read per posting. See the "MojPosao traps" section of AGENTS.md for the
// measurements behind each rule.
//
// The listing is the web app's own search API, 100 postings a request; it names the title,
// employer and place of every open posting but carries only a snippet of the body. The body,
// category, work arrangement and salary come from the posting API, bought only for postings the
// catalogue lacks, within a per-run budget; the rest are refreshed by identity.
type mojposao struct {
	get            JSONGetter
	limiter        waiter
	maxNewPerRun   int
	maxPages       int
	deadlineMargin time.Duration
}

const (
	mojposaoBaseURL = "https://mojposao.hr"
	// mojposaoListURL is the whole open catalogue, 100 postings a page, pages counted from 1.
	mojposaoListURL   = mojposaoBaseURL + "/api/proxy/jobs/search?page=%d"
	mojposaoDetailURL = mojposaoBaseURL + "/api/proxy/jobs/%s"

	// mojposaoRequestInterval paces every request of a run at one a second — robots.txt asks for
	// a one-second crawl delay.
	mojposaoRequestInterval = time.Second
	mojposaoDetailWorkers   = 2
	// mojposaoMaxPages is a safety ceiling: the catalogue was 1,449 postings (15 pages) on
	// 2026-10-28.
	mojposaoMaxPages = 100
	// mojposaoMaxNewPerRun bounds the posting requests of one run: ~15 listing requests and
	// 1,200 posting requests at one a second sit well inside the scheduler's default 2,400 s
	// crawl budget, and cover the whole catalogue's cold start in about two runs.
	mojposaoMaxNewPerRun   = 1200
	mojposaoDeadlineMargin = 5 * time.Minute

	// mojposaoITPosition is the root position "IT, Telecommunications" (the API's own id).
	mojposaoITPosition = 11
)

// NewMojPosao builds the adapter over the given client.
func NewMojPosao(c JSONGetter) Source {
	return mojposao{
		get:            c,
		limiter:        rate.NewLimiter(rate.Every(mojposaoRequestInterval), 1),
		maxNewPerRun:   mojposaoMaxNewPerRun,
		maxPages:       mojposaoMaxPages,
		deadlineMargin: mojposaoDeadlineMargin,
	}
}

func (mojposao) Provider() string { return "mojposao" }

// One national catalogue, no per-tenant id.
func (mojposao) boardless() {}

// Thousands of employers post here, each named per posting.
func (mojposao) aggregator() {}

// mojposaoCard is one search result: what the gate and the job need before the body is bought.
type mojposaoCard struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Slug         string      `json:"slug"`
	PublishedAt  string      `json:"publishedAt"`
	Location     string      `json:"location"`
	Organization mojposaoOrg `json:"organization"`
}

// mojposaoOrg is a posting's employer. An employer that hides its name is not an object but the
// bare string "incognito" (4 of 1,449 postings on 2026-10-28); it decodes to an empty name, and
// such a posting is dropped — filing it under the label would invent one employer collecting
// every anonymous posting.
type mojposaoOrg struct {
	Name string `json:"name"`
}

func (o *mojposaoOrg) UnmarshalJSON(b []byte) error {
	if len(b) > 0 && b[0] != '{' {
		*o = mojposaoOrg{}
		return nil
	}
	type plain mojposaoOrg
	return json.Unmarshal(b, (*plain)(o))
}

type mojposaoPage struct {
	Total      int `json:"total"`
	TotalPages int `json:"totalPages"`
	Items      []struct {
		Jobs []mojposaoCard `json:"jobs"`
	} `json:"items"`
}

// mojposaoDetail is the part of the posting API the adapter reads.
type mojposaoDetail struct {
	ID           string `json:"id"`
	Title        string `json:"title"`
	Slug         string `json:"slug"`
	IsActive     bool   `json:"isActive"`
	Description  string `json:"description"`
	Requirements string `json:"requirements"`
	Benefits     string `json:"benefits"`
	PublishedAt  string `json:"publishedAt"`
	Location     struct {
		Items []struct {
			Address string `json:"address"`
		} `json:"items"`
		Summary string `json:"summary"`
	} `json:"location"`
	Salary *struct {
		From     *int   `json:"from"`
		To       *int   `json:"to"`
		Currency string `json:"currency"`
		Tax      string `json:"tax"`
	} `json:"salary"`
	Organization           mojposaoOrg `json:"organization"`
	WorkTypes              []string    `json:"workTypes"`
	EmploymentTypes        []string    `json:"employmentTypes"`
	RequiredWorkExperience string      `json:"requiredWorkExperience"`
	RootPosition           *struct {
		ID int `json:"id"`
	} `json:"rootPosition"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s mojposao) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s mojposao) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole search, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest first. A posting the pipeline will discard anyway is judged on
// its search result and yielded body-less without a request. A posting whose body cannot be read
// is not yielded; a later run tries again.
func (s mojposao) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, mojposaoClean(c.Organization.Name))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []mojposaoCard
	for _, c := range cards {
		base := mojposaoBaseJob(c)
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if covered[base.Company] || (gate.Rejected != nil && gate.Rejected(base)) {
			jobs = append(jobs, base)
			continue
		}
		unseen = append(unseen, c)
	}
	// The search orders by ad type, not by date; the newest postings are bought first so a
	// budget-bounded run keeps the freshest part of the catalogue current.
	slices.SortStableFunc(unseen, func(a, b mojposaoCard) int { return strings.Compare(b.PublishedAt, a.PublishedAt) })
	deferred := 0
	if len(unseen) > s.maxNewPerRun {
		deferred = len(unseen) - s.maxNewPerRun
		unseen = unseen[:s.maxNewPerRun]
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, mojposaoDetailWorkers, func(c mojposaoCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("mojposao: posting %s failed; deferring: %v", c.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no posting at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("mojposao: requested %d postings and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("mojposao: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("mojposao: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s mojposao) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the search to the last page its own totalPages names and returns every named
// posting. Any page failing fails the crawl — a partial listing would let the company-scoped
// sweep close the postings of the employers it did see on the pages it did not — and so does a
// page inside the stated range that carries nothing.
func (s mojposao) list(ctx context.Context) ([]mojposaoCard, error) {
	seen := make(map[string]bool)
	var out []mojposaoCard
	pages := 1
	for page := 1; page <= pages; page++ {
		if page > s.maxPages {
			return nil, fmt.Errorf("mojposao: listing still not exhausted after %d pages", s.maxPages)
		}
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var p mojposaoPage
		if err := s.get.GetJSON(ctx, fmt.Sprintf(mojposaoListURL, page), &p); err != nil {
			return nil, fmt.Errorf("mojposao: listing page %d: %w", page, err)
		}
		if page == 1 {
			if p.TotalPages <= 0 || len(p.Items) == 0 {
				return nil, fmt.Errorf("mojposao: listing carries no posting")
			}
			pages = p.TotalPages
		} else if len(p.Items) == 0 {
			return nil, fmt.Errorf("mojposao: listing page %d of %d carries no posting", page, pages)
		}
		for _, g := range p.Items {
			for _, c := range g.Jobs {
				if c.ID == "" || seen[c.ID] || mojposaoClean(c.Title) == "" || mojposaoClean(c.Organization.Name) == "" {
					continue
				}
				seen[c.ID] = true
				out = append(out, c)
			}
		}
	}
	return out, nil
}

func mojposaoClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// mojposaoPostingURL is the posting's public page. The page resolves by the id; the slug is the
// site's own.
func mojposaoPostingURL(id, slug string) string {
	if slug = strings.TrimSpace(slug); slug == "" {
		return mojposaoBaseURL + "/posao/" + id
	}
	return mojposaoBaseURL + "/posao/" + id + "/" + slug
}

// mojposaoBaseJob is the body-less Job a search result alone supports: the identity, and the
// facts the gate judges on.
func mojposaoBaseJob(c mojposaoCard) Job {
	j := Job{
		ExternalID:        c.ID,
		URL:               mojposaoPostingURL(c.ID, c.Slug),
		Title:             mojposaoClean(c.Title),
		Company:           mojposaoClean(c.Organization.Name),
		Location:          mojposaoClean(c.Location),
		FallbackCountries: []string{"hr"},
	}
	if t, ok := mojposaoTime(c.PublishedAt); ok {
		j.PostedAt = NotFuture(&t)
	}
	return j
}

// detail reads one posting. ok=false with a nil error is the site's answer that the posting is
// gone (404/410, or no longer active) or carries no usable posting; an error is the absence of an
// answer.
func (s mojposao) detail(ctx context.Context, c mojposaoCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	var d mojposaoDetail
	if err := s.get.GetJSON(ctx, fmt.Sprintf(mojposaoDetailURL, c.ID), &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if !d.IsActive {
		return Job{}, false, nil
	}
	job, ok := mojposaoJob(d, c)
	return job, ok, nil
}

// mojposaoJob maps a posting and its search result to a Job. ok is false for one with no title,
// employer or body.
func mojposaoJob(d mojposaoDetail, c mojposaoCard) (Job, bool) {
	j := mojposaoBaseJob(c)
	if t := mojposaoClean(d.Title); t != "" {
		j.Title = t
	}
	if co := mojposaoClean(d.Organization.Name); co != "" {
		j.Company = co
	}
	j.Description = mojposaoBody(d)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	if d.Slug != "" {
		j.URL = mojposaoPostingURL(c.ID, d.Slug)
	}
	if place := mojposaoClean(d.Location.Summary); place != "" {
		j.Location = place
	}
	j.Countries = mojposaoCountries(d)
	if t, ok := mojposaoTime(d.PublishedAt); ok {
		j.PostedAt = NotFuture(&t)
	}
	if m := mojposaoWorkMode(d.WorkTypes); m != "" {
		j.WorkMode, j.Remote = m, m == "remote"
	}
	j.EmploymentType = mojposaoEmploymentType(d.EmploymentTypes)
	if years, ok := mojposaoExperience(d.RequiredWorkExperience); ok {
		j.ExperienceYearsMin = &years
	}
	j.IsTechHint = d.RootPosition != nil && d.RootPosition.ID == mojposaoITPosition
	return j, true
}

// mojposaoBody joins the posting's three text sections under the headings the site's own page
// gives them, and leads with the salary line the page shows when the employer published one.
func mojposaoBody(d mojposaoDetail) string {
	var b strings.Builder
	if line := mojposaoSalaryLine(d); line != "" {
		b.WriteString("<p>" + line + "</p>")
	}
	for _, sec := range []struct{ heading, text string }{
		{"", d.Description},
		{"Potrebno obrazovanje, znanja i vještine", d.Requirements},
		{"Poslodavac ti nudi", d.Benefits},
	} {
		body := mojposaoText(sec.text)
		if body == "" {
			continue
		}
		if sec.heading != "" {
			b.WriteString("<h3>" + sec.heading + "</h3>")
		}
		b.WriteString(body)
	}
	return sanitizeHTML(b.String())
}

// mojposaoInlineTag matches the inline emphasis employers type into the otherwise plain-text
// sections ("<b>prvi kontakt</b>"), after escaping; mojposaoOtherTag is any other tag, which is
// dropped (a stray "<div>{BENEFITS_2}</div>" template remnant was measured).
var (
	mojposaoInlineTag = regexp.MustCompile(`&lt;(/?)(b|strong|i|em|u)&gt;`)
	mojposaoOtherTag  = regexp.MustCompile(`&lt;/?[a-zA-Z][a-zA-Z0-9]*( [^&]*)?/?&gt;`)
)

// mojposaoText renders a plain-text section — blank-line paragraphs, "- " bullets, the odd
// inline <b> — into HTML.
func mojposaoText(s string) string {
	s = strings.TrimSpace(strings.ReplaceAll(s, "\r\n", "\n"))
	if s == "" {
		return ""
	}
	out := plainTextToHTML(s)
	out = mojposaoInlineTag.ReplaceAllString(out, "<$1$2>")
	return mojposaoOtherTag.ReplaceAllString(out, "")
}

// mojposaoSalaryLine is the salary as the posting page states it ("Plaća: Od 1080 eur do 1600
// eur bruto"). It is kept as text, never as structured salary fields: the API states the
// currency and, on most postings, whether the figure is gross or net, but never the period.
func mojposaoSalaryLine(d mojposaoDetail) string {
	sal := d.Salary
	if sal == nil || sal.From == nil || *sal.From <= 0 {
		return ""
	}
	cur := strings.ToUpper(strings.TrimSpace(sal.Currency))
	line := "Plaća: od " + strconv.Itoa(*sal.From) + " " + cur
	if sal.To != nil && *sal.To > *sal.From {
		line += " do " + strconv.Itoa(*sal.To) + " " + cur
	}
	switch strings.ToLower(strings.TrimSpace(sal.Tax)) {
	case "gross":
		line += " bruto"
	case "net":
		line += " neto"
	}
	return line
}

// mojposaoCountries reads the country closing each structured address ("Zagreb County,
// Croatia", "Frankfurt am Main, Germany") — the platform's own geocoding, in English.
func mojposaoCountries(d mojposaoDetail) []string {
	var out []string
	for _, it := range d.Location.Items {
		addr := strings.TrimSpace(it.Address)
		if i := strings.LastIndex(addr, ","); i >= 0 {
			addr = addr[i+1:]
		}
		if code := location.NormalizeCountry(strings.TrimSpace(addr)); code != "" && !slices.Contains(out, code) {
			out = append(out, code)
		}
	}
	return out
}

// mojposaoTime reads the API's RFC 3339 timestamps ("2026-10-28T07:55:05.000Z").
func mojposaoTime(s string) (time.Time, bool) {
	t, err := time.Parse(time.RFC3339, strings.TrimSpace(s))
	return t, err == nil
}

// mojposaoWorkMode reads the posting's structured arrangement, preferring the most remote one a
// posting offers ("hybrid, on-site" is a hybrid posting).
func mojposaoWorkMode(types []string) string {
	set := map[string]bool{}
	for _, t := range types {
		set[strings.ToLower(strings.TrimSpace(t))] = true
	}
	switch {
	case set["remote"]:
		return "remote"
	case set["hybrid"]:
		return "hybrid"
	case set["on-site"]:
		return "onsite"
	}
	return ""
}

// mojposaoEmploymentType maps the contract type when the posting names exactly one that says
// something about the hours or the engagement. "Permanent" and "Temporary" are contract
// durations, not hours, and say nothing a vocabulary value could carry.
func mojposaoEmploymentType(types []string) string {
	if len(types) != 1 {
		return ""
	}
	switch strings.TrimSpace(types[0]) {
	case "Part-time":
		return "part_time"
	case "Internship":
		return "internship"
	}
	return ""
}

// mojposaoExperience reads the required years ("1-3", "5+", "2"); "<1" and an empty field state
// no floor.
func mojposaoExperience(s string) (int, bool) {
	s = strings.TrimSpace(s)
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}
