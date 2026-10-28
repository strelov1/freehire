package sources

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/normalize"
)

// bumeran adapts Bumeran (www.bumeran.com.ar), Argentina's largest general job board, and with it
// ZonaJobs (www.zonajobs.com.ar): both belong to Jobint/Navent and are ONE posting pool behind
// one API — the same search answers the same ~9k postings on either host, a ZonaJobs-born
// posting keeping its own (shorter) id. Every trade, mostly staffing agencies and consultancies,
// ~5% technical. It is boardless and an aggregator — the employer is read per posting. See the
// "Bumeran traps" section of AGENTS.md for the measurements behind each rule.
//
// The listing is the web app's own search API, 100 postings a request, newest first; it carries
// everything but a usable body (its "detalle" is the body with every tag and line break
// stripped). The body, salary and experience come from the posting API, bought only for postings
// the catalogue lacks, newest first, within a per-run budget; the rest are refreshed by
// identity.
type bumeran struct {
	http           HeaderJSONPoster
	get            HeaderJSONGetter
	limiter        waiter
	maxNewPerRun   int
	maxPages       int
	deadlineMargin time.Duration
}

const (
	bumeranBaseURL = "https://www.bumeran.com.ar"
	// bumeranListURL is the search, newest first. The edge answers 403 to an API request
	// without the site's own Referer (measured from the crawl host, any User-Agent).
	bumeranListURL   = bumeranBaseURL + "/api/avisos/searchV2?pageSize=%d&page=%d&sort=RECIENTES"
	bumeranDetailURL = bumeranBaseURL + "/api/candidates/fichaAvisoNormalizada/%d"
	bumeranReferer   = bumeranBaseURL + "/empleos.html"
	// bumeranSiteID selects the Argentine Bumeran site; ZJAR (ZonaJobs) answers the same pool.
	bumeranSiteID = "BMAR"
	// bumeranPageSize: the API honours up to at least 200; 100 keeps a page ~270 KB.
	bumeranPageSize = 100

	// bumeranRequestInterval paces every request of a run at one a second — a courtesy ceiling,
	// not a measured limit.
	bumeranRequestInterval = time.Second
	bumeranDetailWorkers   = 2
	// bumeranMaxPages is a safety ceiling: the pool was 8,941 postings (90 pages) on 2026-10-27.
	bumeranMaxPages = 400
	// bumeranMaxNewPerRun bounds the posting requests of one run: ~90 listing requests and 1,200
	// posting requests at one a second, plus the writes that follow, sit well inside the
	// scheduler's default 2,400 s crawl budget. A cold catalogue is taken in slices, newest first.
	bumeranMaxNewPerRun   = 1200
	bumeranDeadlineMargin = 5 * time.Minute
)

// NewBumeran builds the adapter over the given client.
func NewBumeran(c HTTPClient) Source {
	return bumeran{
		http:           c,
		get:            c,
		limiter:        rate.NewLimiter(rate.Every(bumeranRequestInterval), 1),
		maxNewPerRun:   bumeranMaxNewPerRun,
		maxPages:       bumeranMaxPages,
		deadlineMargin: bumeranDeadlineMargin,
	}
}

func (bumeran) Provider() string { return "bumeran" }

// One national pool, no per-tenant id.
func (bumeran) boardless() {}

// Thousands of employers and agencies post here, each named per posting.
func (bumeran) aggregator() {}

var bumeranHeaders = map[string]string{"x-site-id": bumeranSiteID, "Referer": bumeranReferer}

// bumeranCard is one search result: what the gate and the job need before the body is bought.
type bumeranCard struct {
	ID           int64  `json:"id"`
	Title        string `json:"titulo"`
	Company      string `json:"empresa"`
	Confidential bool   `json:"confidencial"`
	Place        string `json:"localizacion"`
	WorkMode     string `json:"modalidadTrabajo"`
	Schedule     string `json:"tipoTrabajo"`
	AreaID       int    `json:"idArea"`
	SubareaID    int    `json:"idSubarea"`
	Published    string `json:"fechaHoraPublicacion"`
}

type bumeranPage struct {
	Total   int           `json:"total"`
	Content []bumeranCard `json:"content"`
}

// bumeranDetail is the part of the posting API the adapter reads.
type bumeranDetail struct {
	Aviso struct {
		ID          int64  `json:"id"`
		Status      string `json:"estado"`
		Title       string `json:"titulo"`
		Description string `json:"descripcion"`
		Published   string `json:"fechaHoraPublicacion"`
		Company     struct {
			Name         string `json:"denominacion"`
			Confidential bool   `json:"confidencial"`
		} `json:"empresa"`
		Location struct {
			Detail  string `json:"detalle"`
			Country string `json:"paisDetalle"`
		} `json:"localizacion"`
		Area struct {
			ID int `json:"id"`
		} `json:"area"`
		Subarea struct {
			ID int `json:"id"`
		} `json:"subArea"`
		Requirements struct {
			Experience *struct {
				MinMonths int `json:"minimo"`
			} `json:"experiencia"`
			Salary *bumeranSalaryField `json:"salario"`
		} `json:"requisitos"`
		Schedule struct {
			Name string `json:"nombre"`
		} `json:"tipoTrabajo"`
		WorkMode struct {
			Name string `json:"nombre"`
		} `json:"modalidadTrabajo"`
	} `json:"aviso"`
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s bumeran) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s bumeran) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole search, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest first. A posting the pipeline will discard anyway is judged
// on its search result and yielded body-less without a request; one with a hidden employer is
// dropped before any. A posting whose body cannot be read is not yielded; a later run tries again.
func (s bumeran) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var covered map[string]bool
	if gate.Covered != nil {
		companies := make([]string, 0, len(cards))
		for _, c := range cards {
			companies = append(companies, bumeranClean(c.Company))
		}
		covered = gate.Covered(companies)
	}
	var jobs []Job
	var unseen []bumeranCard
	deferred := 0
	for _, c := range cards {
		base := bumeranBaseJob(c)
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
		unseen = append(unseen, c)
	}
	var answered, outOfTime atomic.Int64
	read := fetchDetails(unseen, bumeranDetailWorkers, func(c bumeranCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("bumeran: posting %d failed; deferring: %v", c.ID, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no posting at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("bumeran: requested %d postings and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("bumeran: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("bumeran: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes: a hydrating crawl writes nothing until FetchNewGated returns, so one
// that runs into its deadline stores nothing at all.
func (s bumeran) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the search until a page carries no result or the search's own total is read, and
// returns the named-employer postings in the search's own order, newest first. Ids do not order
// them: a ZonaJobs-born posting's 7-digit id is older-looking than any Bumeran one. Any page
// failing fails the crawl — a partial listing would let the company-scoped sweep close the
// postings of the employers it did see on the pages it did not.
func (s bumeran) list(ctx context.Context) ([]bumeranCard, error) {
	seen := make(map[int64]bool)
	var out []bumeranCard
	read := 0
	for page := 0; page < s.maxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		var p bumeranPage
		body := map[string]any{"filtros": []any{}, "query": "", "internacional": false}
		if err := s.http.PostJSONWithHeaders(ctx, fmt.Sprintf(bumeranListURL, bumeranPageSize, page), bumeranHeaders, body, &p); err != nil {
			return nil, fmt.Errorf("bumeran: listing page %d: %w", page, err)
		}
		if len(p.Content) == 0 {
			if page == 0 {
				return nil, fmt.Errorf("bumeran: listing carries no posting")
			}
			// Postings closing during the walk shift the pages, so the end can come a little
			// before the total stated on page 0.
			return out, nil
		}
		for _, c := range p.Content {
			read++
			if c.ID <= 0 || seen[c.ID] || strings.TrimSpace(c.Title) == "" || bumeranHiddenEmployer(c.Company, c.Confidential) {
				continue
			}
			seen[c.ID] = true
			out = append(out, c)
		}
		if p.Total > 0 && read >= p.Total {
			return out, nil
		}
	}
	return nil, fmt.Errorf("bumeran: listing still not exhausted after %d pages", s.maxPages)
}

// bumeranHiddenEmployer reports whether a posting hides its employer: the confidential flag
// (20% of the pool, shown as "Confidencial"), or a description standing in for a name
// ("Importante empresa de retail", "Empresa Alimenticia", a bare "Consultora"). Filing those under
// the label would invent one employer collecting every anonymous posting of its kind.
func bumeranHiddenEmployer(name string, confidential bool) bool {
	n := strings.ToLower(bumeranClean(name))
	switch {
	case confidential, n == "", n == "confidencial", n == "consultora", n == "empresa":
		return true
	case strings.HasPrefix(n, "importante empresa"), strings.HasPrefix(n, "reconocida empresa"):
		return true
	case strings.HasPrefix(n, "empresa ") && !bumeranLegalForm(n):
		return true
	}
	return false
}

// bumeranLegalForm reports whether a name opening with "Empresa" carries a registered legal form,
// which makes it a name rather than a description ("Empresa Distribuidora Sur S.A.").
func bumeranLegalForm(n string) bool {
	for _, f := range []string{" s.a.", " s.a", " sa", " s.r.l.", " srl", " sas", " s.a.s."} {
		if strings.HasSuffix(n, f) {
			return true
		}
	}
	return false
}

func bumeranClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// bumeranPostingURL is the posting's public page. The page is a single-page app that reads only
// the trailing id, so the slug is cosmetic; it is built the site's way, title then employer.
func bumeranPostingURL(id int64, title, company string) string {
	slug := normalize.Slug(title + " " + company)
	if slug == "" {
		return fmt.Sprintf("%s/empleos/%d.html", bumeranBaseURL, id)
	}
	return fmt.Sprintf("%s/empleos/%s-%d.html", bumeranBaseURL, slug, id)
}

// bumeranBaseJob is the body-less Job a search result alone supports: the identity, and the
// facts the gate judges on.
func bumeranBaseJob(c bumeranCard) Job {
	title, company := bumeranClean(c.Title), bumeranClean(c.Company)
	return Job{
		ExternalID:        strconv.FormatInt(c.ID, 10),
		URL:               bumeranPostingURL(c.ID, title, company),
		Title:             title,
		Company:           company,
		Location:          bumeranClean(c.Place),
		WorkMode:          bumeranWorkMode(c.WorkMode),
		Remote:            bumeranWorkMode(c.WorkMode) == "remote",
		EmploymentType:    bumeranEmploymentType(c.Schedule),
		IsTechHint:        bumeranTechSubarea(c.AreaID, c.SubareaID),
		FallbackCountries: []string{"ar"},
	}
}

// detail reads one posting. ok=false with a nil error is the site's answer that the posting is
// gone (404, or a state other than "activo") or carries no usable posting; an error is the absence
// of an answer.
func (s bumeran) detail(ctx context.Context, c bumeranCard) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	var d bumeranDetail
	if err := s.get.GetJSONWithHeaders(ctx, fmt.Sprintf(bumeranDetailURL, c.ID), bumeranHeaders, &d); err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	if !strings.EqualFold(strings.TrimSpace(d.Aviso.Status), "activo") {
		return Job{}, false, nil
	}
	job, ok := bumeranJob(d, c)
	return job, ok, nil
}

// bumeranJob maps a posting and its search result to a Job. ok is false for one with no title,
// employer or body, or whose employer the posting hides.
func bumeranJob(d bumeranDetail, c bumeranCard) (Job, bool) {
	a := d.Aviso
	j := bumeranBaseJob(c)
	if t := bumeranClean(a.Title); t != "" {
		j.Title = t
	}
	if co := bumeranClean(a.Company.Name); co != "" {
		j.Company = co
	}
	if bumeranHiddenEmployer(j.Company, a.Company.Confidential) {
		return Job{}, false
	}
	j.Description = sanitizeHTML(a.Description)
	if j.Title == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	j.URL = bumeranPostingURL(c.ID, j.Title, j.Company)
	if place := bumeranClean(a.Location.Detail); place != "" {
		j.Location = place
	}
	if code := location.NormalizeCountry(strings.TrimSpace(a.Location.Country)); code != "" && !slices.Contains(j.Countries, code) {
		j.Countries = append(j.Countries, code)
	}
	if t, ok := bumeranTime(a.Published); ok {
		j.PostedAt = NotFuture(&t)
	} else if t, ok := bumeranTime(c.Published); ok {
		j.PostedAt = NotFuture(&t)
	}
	if m := bumeranWorkMode(a.WorkMode.Name); m != "" {
		j.WorkMode, j.Remote = m, m == "remote"
	}
	if et := bumeranEmploymentType(a.Schedule.Name); et != "" {
		j.EmploymentType = et
	}
	if a.Area.ID != 0 {
		j.IsTechHint = bumeranTechSubarea(a.Area.ID, a.Subarea.ID)
	}
	if exp := a.Requirements.Experience; exp != nil && exp.MinMonths >= 12 {
		years := exp.MinMonths / 12
		j.ExperienceYearsMin = &years
	}
	if lo, hi, ok := bumeranSalary(a.Requirements.Salary); ok {
		j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = &lo, &hi, "ARS", "month"
	}
	return j, true
}

// bumeranArgentina is the zone the site's timestamps are written in (UTC-3, no daylight saving).
var bumeranArgentina = time.FixedZone("ART", -3*60*60)

// bumeranTime reads the site's "27-10-2026 20:02:10", Argentine local time.
func bumeranTime(s string) (time.Time, bool) {
	t, err := time.ParseInLocation("02-01-2006 15:04:05", strings.TrimSpace(s), bumeranArgentina)
	return t, err == nil
}

// bumeranWorkMode reads the posting's structured arrangement. "Presencial" is the form's default
// — 85% of the pool, including postings whose own title says "Home office" or "(Remoto)" — so it
// says nothing and is left to the description.
func bumeranWorkMode(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "remoto":
		return "remote"
	case "híbrido", "hibrido":
		return "hybrid"
	}
	return ""
}

func bumeranEmploymentType(s string) string {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "full-time":
		return "full_time"
	case "part-time", "por horas", "fines de semana":
		return "part_time"
	case "pasantia", "pasantía":
		return "internship"
	case "por contrato", "temporario":
		return "contract"
	}
	return ""
}

// bumeranTechArea is "Tecnología, Sistemas y Telecomunicaciones". Its subareas are the poster's
// own structured choice, so they set IsTechHint — all but field telecommunications (107) and data
// entry (2608), which are not technical work.
const bumeranTechArea = 19

func bumeranTechSubarea(area, subarea int) bool {
	return area == bumeranTechArea && subarea != 107 && subarea != 2608
}

// bumeranSalaryField is the posting's structured salary; every field may be null.
type bumeranSalaryField struct {
	Min    *int `json:"salarioMinimo"`
	Max    *int `json:"salarioMaximo"`
	Period *struct {
		Name string `json:"nombre"`
	} `json:"frecuenciaPago"`
}

// bumeranMinSalary is the floor under which a stated monthly salary is a placeholder, not a
// figure: posters type 1, 2, 11111 or 99999 to get past the form (Argentina's minimum wage is
// several hundred thousand pesos a month). It also keeps out a figure stated in dollars, which
// the form does not mark.
const bumeranMinSalary = 100000

// bumeranSalary reads the structured monthly gross salary, in pesos. Both ends must clear the
// placeholder floor; a range opening at 1 is a placeholder with a ceiling, not a range.
func bumeranSalary(sal *bumeranSalaryField) (lo, hi int, ok bool) {
	if sal == nil || sal.Min == nil || sal.Max == nil || sal.Period == nil {
		return 0, 0, false
	}
	if !strings.EqualFold(strings.TrimSpace(sal.Period.Name), "por mes") {
		return 0, 0, false
	}
	lo, hi = *sal.Min, *sal.Max
	if lo < bumeranMinSalary || hi < lo {
		return 0, 0, false
	}
	return lo, hi, true
}
