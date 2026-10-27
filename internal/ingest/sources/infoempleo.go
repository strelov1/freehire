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
	"sort"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"

	"github.com/strelov1/freehire/internal/dict/location"
)

// infoempleo adapts Infoempleo (www.infoempleo.com), one of Spain's large general job boards:
// ~8k public postings across every trade, most of them posted by staffing agencies (ETTs) and
// recruitment consultancies, a few percent technical. It is boardless and an aggregator — the
// employer is read per posting. See the "Infoempleo traps" section of AGENTS.md for the
// measurements behind each rule.
//
// The listing is the site's own paginated /trabajo/ walk, 20 cards a page, which spell each
// posting's title, employer, place, contract and remote arrangement — everything but the body.
// The body is bought from the posting page's schema.org JobPosting, only for postings the
// catalogue lacks, newest id first, within a per-run budget; the rest are refreshed by identity.
type infoempleo struct {
	http         HTMLGetter
	limiter      waiter
	maxNewPerRun int
	maxPages     int
	// deadlineMargin is how much of the run's deadline hydration leaves to the pipeline: a
	// hydrating crawl writes nothing until FetchNew returns, so one that runs into its deadline
	// stores nothing at all. Pages still unread when the margin is reached wait for a later run.
	deadlineMargin time.Duration
}

const (
	infoempleoBaseURL = "https://www.infoempleo.com"
	// infoempleoListURL is the all-areas listing; page N is <list>pagina_N/.
	infoempleoListURL = infoempleoBaseURL + "/trabajo/"

	// infoempleoRequestInterval paces every request of a run, listing and posting page alike, at
	// one a second — a courtesy ceiling toward a national site, not a measured limit.
	infoempleoRequestInterval = time.Second
	// infoempleoDetailWorkers keeps enough pages in flight that the limiter, not latency, sets
	// the pace.
	infoempleoDetailWorkers = 2
	// infoempleoMaxPages is a safety ceiling on the walk: the listing ran to ~390 pages on
	// 2026-10-27 and answers an empty page (HTTP 200, no cards) past its end.
	infoempleoMaxPages = 800
	// infoempleoMaxNewPerRun bounds how many unseen postings one run reads: the ~390-page walk
	// takes ~7 minutes at one a second and 900 posting pages ~15 more, well inside the
	// scheduler's default 2,400 s crawl budget. The first runs face the whole catalogue, so it is
	// taken in slices, newest first; a posting past the bound is read by a later run.
	infoempleoMaxNewPerRun   = 900
	infoempleoDeadlineMargin = 5 * time.Minute
)

// NewInfoempleo builds the adapter over the given client.
func NewInfoempleo(c HTMLGetter) Source {
	return infoempleo{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(infoempleoRequestInterval), 1),
		maxNewPerRun:   infoempleoMaxNewPerRun,
		maxPages:       infoempleoMaxPages,
		deadlineMargin: infoempleoDeadlineMargin,
	}
}

func (infoempleo) Provider() string { return "infoempleo" }

// One national board, no per-tenant id.
func (infoempleo) boardless() {}

// Thousands of employers and agencies post here, each named per posting.
func (infoempleo) aggregator() {}

// infoempleoCard is one listing card: what the gate and the job need before the page is bought.
type infoempleoCard struct {
	id       int64
	url      string
	title    string
	company  string
	place    string
	contract string
	remote   string
	// experience is the card's experience line ("Al menos 2 años de experiencia").
	experience string
}

func (c infoempleoCard) externalID() string { return strconv.FormatInt(c.id, 10) }

// infoempleoPostingPath matches a posting URL, /ofertasdetrabajo/<title-slug>/<place-slug>/<id>/.
// Both slugs are cosmetic; the id is the identity.
var infoempleoPostingPath = regexp.MustCompile(`^/ofertasdetrabajo/[^/]+/[^/]+/(\d+)/?$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s infoempleo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s infoempleo) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole listing, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest id first. A posting the pipeline will discard anyway — its
// employer already covered by a non-aggregator source, or its title rejected — is judged on its
// card and yielded body-less without a request. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s infoempleo) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	cards, err := s.list(ctx)
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
	var unseen []infoempleoCard
	deferred := 0
	for _, c := range cards {
		base := infoempleoBaseJob(c)
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
	read := fetchDetails(unseen, infoempleoDetailWorkers, func(c infoempleoCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("infoempleo: posting %d failed; deferring: %v", c.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("infoempleo: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("infoempleo: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("infoempleo: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s infoempleo) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the listing pages until one carries no card, and returns the postings newest id
// first. The first page failing fails the crawl; a later one ends the walk with what was read,
// since the adapter is not swept by source.
func (s infoempleo) list(ctx context.Context) ([]infoempleoCard, error) {
	seen := make(map[int64]bool)
	var out []infoempleoCard
	for page := 1; page <= s.maxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		u := infoempleoListURL
		if page > 1 {
			u = fmt.Sprintf("%spagina_%d/", infoempleoListURL, page)
		}
		root, err := s.http.GetHTML(ctx, u)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("infoempleo: listing: %w", err)
			}
			log.Printf("infoempleo: listing page %d failed; keeping the %d postings read: %v", page, len(out), err)
			break
		}
		cards := infoempleoCards(root)
		if len(cards) == 0 {
			if page == 1 {
				return nil, fmt.Errorf("infoempleo: listing page carries no posting card")
			}
			break
		}
		for _, c := range cards {
			if seen[c.id] {
				continue
			}
			seen[c.id] = true
			out = append(out, c)
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// infoempleoCards reads the posting cards of one listing page. The page also links postings
// outside the cards (a "related" rail), which are not read: a card is the unit that names the
// employer.
func infoempleoCards(root *html.Node) []infoempleoCard {
	var out []infoempleoCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "job-offer-card") {
			return true
		}
		if c, ok := infoempleoCardFrom(n); ok {
			out = append(out, c)
		}
		return false
	})
	return out
}

func infoempleoCardFrom(n *html.Node) (infoempleoCard, bool) {
	heading := firstByClass(n, "offer-title")
	if heading == nil {
		return infoempleoCard{}, false
	}
	a := firstByTag(heading, "a")
	if a == nil {
		return infoempleoCard{}, false
	}
	href := strings.TrimSpace(Attr(a, "href"))
	m := infoempleoPostingPath.FindStringSubmatch(href)
	if m == nil {
		return infoempleoCard{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return infoempleoCard{}, false
	}
	fact := func(class string) string {
		if f := firstByClass(n, class); f != nil {
			if span := firstByTag(f, "span"); span != nil {
				return infoempleoClean(textContent(span))
			}
		}
		return ""
	}
	c := infoempleoCard{
		id:         id,
		url:        infoempleoBaseURL + href,
		title:      infoempleoClean(textContent(a)),
		place:      fact("offer-location"),
		contract:   fact("offer-contract-fact"),
		remote:     fact("offer-remote-fact"),
		experience: fact("offer-experience-fact"),
	}
	if co := firstByClass(n, "company-name"); co != nil {
		c.company = infoempleoClean(textContent(co))
	}
	if c.title == "" {
		return infoempleoCard{}, false
	}
	return c, true
}

func infoempleoClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// infoempleoBaseJob is the body-less Job a card alone supports: the identity, and the facts the
// gate judges on.
func infoempleoBaseJob(c infoempleoCard) Job {
	return Job{
		ExternalID:        c.externalID(),
		URL:               c.url,
		Title:             c.title,
		Company:           c.company,
		Location:          c.place,
		FallbackCountries: []string{"es"},
	}
}

// infoempleoLD is the part of the posting page's schema.org JobPosting the adapter reads.
type infoempleoLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     string          `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	Industry           string          `json:"industry"`
	JobLocation        json.RawMessage `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string  `json:"unitText"`
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

type infoempleoPlace struct {
	Address struct {
		Locality string `json:"addressLocality"`
		Region   string `json:"addressRegion"`
		Country  string `json:"addressCountry"`
	} `json:"address"`
}

// places reads jobLocation, which the site writes as one Place; an array is accepted too.
func (ld infoempleoLD) places() []infoempleoPlace {
	var one infoempleoPlace
	if err := json.Unmarshal(ld.JobLocation, &one); err == nil {
		return []infoempleoPlace{one}
	}
	var many []infoempleoPlace
	_ = json.Unmarshal(ld.JobLocation, &many)
	return many
}

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or a page carrying no usable posting; an error is the absence of an answer.
func (s infoempleo) detail(ctx context.Context, c infoempleoCard) (Job, bool, error) {
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
	var ld infoempleoLD
	if !ldJobPosting(root, &ld) {
		// A withdrawn posting answers 200 with the site's "offer no longer available" page,
		// which carries no JobPosting.
		return Job{}, false, nil
	}
	job, ok := infoempleoJob(ld, c)
	return job, ok, nil
}

// infoempleoTechIndustry is the employer area the site files technical postings under. It is
// the poster's own structured choice, so it sets IsTechHint; "Telecomunicaciones" is not
// included, since most of it is field installation work.
const infoempleoTechIndustry = "tecnología e informática"

var infoempleoYears = regexp.MustCompile(`(\d+)\s+años?`)

// infoempleoJob maps a posting's JobPosting and its card to a Job. ok is false for one with no
// title, employer or body.
func infoempleoJob(ld infoempleoLD, c infoempleoCard) (Job, bool) {
	j := infoempleoBaseJob(c)
	if t := infoempleoClean(ld.Title); t != "" {
		j.Title = t
	}
	if co := strings.TrimSpace(ld.HiringOrganization.Name); co != "" {
		j.Company = co
	}
	j.Description = sanitizeHTML(ld.Description)
	if j.Title == "" || j.Company == "" || strings.TrimSpace(j.Description) == "" {
		return Job{}, false
	}
	var places []string
	for _, p := range ld.places() {
		city, region := strings.TrimSpace(p.Address.Locality), strings.TrimSpace(p.Address.Region)
		switch {
		case city != "" && region != "" && !strings.EqualFold(city, region):
			places = append(places, city+", "+region)
		case city != "":
			places = append(places, city)
		case region != "":
			places = append(places, region)
		}
		if code := location.NormalizeCountry(strings.TrimSpace(p.Address.Country)); code != "" && !slices.Contains(j.Countries, code) {
			j.Countries = append(j.Countries, code)
		}
	}
	if len(places) > 0 {
		j.Location = strings.Join(places, "; ")
	}
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	j.WorkMode = infoempleoWorkMode(ld.JobLocationType, c.remote)
	j.Remote = j.WorkMode == "remote"
	j.EmploymentType = infoempleoEmploymentType(ld.EmploymentType, c.contract)
	if m := infoempleoYears.FindStringSubmatch(c.experience); m != nil {
		if years, err := strconv.Atoi(m[1]); err == nil && years > 0 {
			j.ExperienceYearsMin = &years
		}
	}
	j.IsTechHint = strings.EqualFold(strings.TrimSpace(ld.Industry), infoempleoTechIndustry)
	sal := ld.BaseSalary
	if sal.Currency != "" && strings.EqualFold(sal.Value.UnitText, "YEAR") && sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(strings.TrimSpace(sal.Currency))
		j.SalaryPeriod = "year"
	}
	return j, true
}

// infoempleoWorkMode reads the two structured remote signals: the JobPosting's TELECOMMUTE (set
// only on fully remote postings) and the card's remote line ("Teletrabajo parcial" is hybrid).
// A posting stating neither says nothing — most are on-site, but the site does not say so.
func infoempleoWorkMode(locationType, cardRemote string) string {
	r := strings.ToLower(cardRemote)
	switch {
	case strings.Contains(r, "parcial"):
		return "hybrid"
	case strings.EqualFold(strings.TrimSpace(locationType), "TELECOMMUTE"),
		strings.Contains(r, "total"), strings.Contains(r, "100"):
		return "remote"
	}
	return ""
}

// infoempleoEmploymentType reads the JobPosting's employmentType, falling back to the card's
// contract line ("Prácticas · Jornada completa"). "Indefinido" and "De duración determinada"
// name the contract's term, not the arrangement, and set nothing.
func infoempleoEmploymentType(ldType, contract string) string {
	switch strings.ToUpper(strings.TrimSpace(ldType)) {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	case "INTERN", "INTERNSHIP":
		return "internship"
	case "CONTRACTOR":
		return "contract"
	}
	c := strings.ToLower(contract)
	switch {
	case strings.Contains(c, "prácticas"):
		return "internship"
	case strings.Contains(c, "autónomo"), strings.Contains(c, "freelance"):
		return "contract"
	case strings.Contains(c, "jornada parcial"):
		return "part_time"
	case strings.Contains(c, "jornada completa"):
		return "full_time"
	}
	return ""
}
