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

// itjobs adapts ITJobs.pt (www.itjobs.pt), Portugal's IT-only job board: ~825 public postings,
// every one technical, posted by employers and IT staffing consultancies alike. It is boardless
// and an aggregator — the employer is read per posting. See the "ITJobs.pt traps" section of
// AGENTS.md for the measurements behind each rule.
//
// The listing is the site's own paginated /emprego walk, 16 cards a page, which spell each
// posting's title, employer, places and work model — everything but the body. The body is bought
// from the posting page's schema.org JobPosting, only for postings the catalogue lacks, newest id
// first, within a per-run budget; the rest are refreshed by identity.
type itjobs struct {
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
	itjobsBaseURL = "https://www.itjobs.pt"
	// itjobsListURL is the unfiltered listing; page N is <list>?page=N. robots.txt disallows the
	// list's FILTER parameters (location, work_model, ...), not page.
	itjobsListURL = itjobsBaseURL + "/emprego"

	// itjobsRequestInterval paces every request of a run, listing and posting page alike, at one
	// a second — the site's own robots.txt Crawl-delay.
	itjobsRequestInterval = time.Second
	// itjobsDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the
	// pace.
	itjobsDetailWorkers = 2
	// itjobsMaxPages is a safety ceiling on the walk: the listing ran to 52 pages on 2026-10-27
	// and answers a card-less page (HTTP 200) past its end.
	itjobsMaxPages = 200
	// itjobsMaxNewPerRun bounds how many unseen postings one run reads: the ~52-page walk takes
	// about a minute and 1,200 posting pages twenty more, inside the scheduler's default 2,400 s
	// crawl budget with room for the writes. It covers the whole catalogue, so the first run
	// reads all of it; later runs read the day's ~30-50 new postings.
	itjobsMaxNewPerRun   = 1200
	itjobsDeadlineMargin = 5 * time.Minute
)

// NewITJobs builds the adapter over the given client.
func NewITJobs(c HTMLGetter) Source {
	return itjobs{
		http:           c,
		limiter:        rate.NewLimiter(rate.Every(itjobsRequestInterval), 1),
		maxNewPerRun:   itjobsMaxNewPerRun,
		maxPages:       itjobsMaxPages,
		deadlineMargin: itjobsDeadlineMargin,
	}
}

func (itjobs) Provider() string { return "itjobs" }

// One national board, no per-tenant id.
func (itjobs) boardless() {}

// Hundreds of employers and IT consultancies post here, each named per posting.
func (itjobs) aggregator() {}

// itjobsCard is one listing card: what the gate and the job need before the page is bought.
type itjobsCard struct {
	id      int64
	url     string
	title   string
	company string
	// places is the card's comma-separated place line ("Aveiro, Coimbra, Porto"); a remote
	// posting often has none.
	places string
	// workModel is the card's work-model line: "Remoto", "Híbrido" or "Presencial".
	workModel string
}

func (c itjobsCard) externalID() string { return strconv.FormatInt(c.id, 10) }

// itjobsPostingPath matches a posting URL, /oferta/<id>/<title-slug>. The slug is cosmetic; the
// id is the identity.
var itjobsPostingPath = regexp.MustCompile(`^/oferta/(\d+)(?:/[^/?#]*)?$`)

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s itjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s itjobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated walks the whole listing, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest id first. A posting the pipeline will discard anyway — its
// employer already covered by a non-aggregator source, or its title rejected — is judged on its
// card and yielded body-less without a request. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s itjobs) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
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
	var unseen []itjobsCard
	deferred := 0
	for _, c := range cards {
		base := itjobsBaseJob(c)
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
	read := fetchDetails(unseen, itjobsDetailWorkers, func(c itjobsCard) (Job, bool) {
		if s.nearDeadline(ctx) {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, c)
		if err != nil {
			log.Printf("itjobs: posting %d failed; deferring: %v", c.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	// A crawl that read no page at all is broken, not quiet. "Answered" counts a posting the
	// site reported gone; postings left for want of time are not failures.
	if int64(len(unseen)) > outOfTime.Load() && answered.Load() == 0 {
		return nil, fmt.Errorf("itjobs: requested %d posting pages and none answered", int64(len(unseen))-outOfTime.Load())
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("itjobs: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("itjobs: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s itjobs) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// list walks the listing pages until one carries no card, and returns the postings newest id
// first. The first page failing fails the crawl; a later one ends the walk with what was read,
// since the adapter is not swept by source.
func (s itjobs) list(ctx context.Context) ([]itjobsCard, error) {
	seen := make(map[int64]bool)
	var out []itjobsCard
	for page := 1; page <= s.maxPages; page++ {
		if err := s.limiter.Wait(ctx); err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		u := itjobsListURL
		if page > 1 {
			u = fmt.Sprintf("%s?page=%d", itjobsListURL, page)
		}
		root, err := s.http.GetHTML(ctx, u)
		if err != nil {
			if page == 1 {
				return nil, fmt.Errorf("itjobs: listing: %w", err)
			}
			log.Printf("itjobs: listing page %d failed; keeping the %d postings read: %v", page, len(out), err)
			break
		}
		cards := itjobsCards(root)
		if len(cards) == 0 {
			if page == 1 {
				return nil, fmt.Errorf("itjobs: listing page carries no posting card")
			}
			break
		}
		for _, c := range cards {
			// Every page repeats a promoted posting above its own list.
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

// itjobsCards reads the posting cards of one listing page: each div.list-title, with the
// div.list-name and div.list-details beside it.
func itjobsCards(root *html.Node) []itjobsCard {
	var out []itjobsCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "div" || !HasClass(n, "list-title") {
			return true
		}
		if c, ok := itjobsCardFrom(n); ok {
			out = append(out, c)
		}
		return false
	})
	return out
}

func itjobsCardFrom(heading *html.Node) (itjobsCard, bool) {
	a := firstByTag(heading, "a")
	if a == nil || heading.Parent == nil {
		return itjobsCard{}, false
	}
	href := strings.TrimSpace(Attr(a, "href"))
	m := itjobsPostingPath.FindStringSubmatch(href)
	if m == nil {
		return itjobsCard{}, false
	}
	id, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || id <= 0 {
		return itjobsCard{}, false
	}
	c := itjobsCard{id: id, url: itjobsBaseURL + href, title: itjobsClean(textContent(a))}
	if name := firstByClass(heading.Parent, "list-name"); name != nil {
		c.company = itjobsClean(textContent(name))
	}
	if details := firstByClass(heading.Parent, "list-details"); details != nil {
		facts := itjobsDetailFacts(details)
		c.places = facts["fa-map-marker"]
		c.workModel = facts["fa-car-building"]
	}
	if c.title == "" || c.company == "" {
		return itjobsCard{}, false
	}
	return c, true
}

// itjobsDetailFacts reads a card's detail line, where each fact is an icon followed by its text
// (`<i class="fa fa-map-marker"></i>&nbsp;Lisboa`), keyed by the icon's fa-* class. A commented-
// out fact (the schedule) is a comment node and is not read.
func itjobsDetailFacts(details *html.Node) map[string]string {
	facts := map[string]string{}
	key := ""
	for n := details.FirstChild; n != nil; n = n.NextSibling {
		switch {
		case n.Type == html.ElementNode && n.Data == "i":
			key = ""
			for _, cls := range strings.Fields(Attr(n, "class")) {
				if strings.HasPrefix(cls, "fa-") {
					key = cls
					break
				}
			}
		case n.Type == html.TextNode && key != "":
			if v := itjobsClean(n.Data); v != "" {
				facts[key] = v
				key = ""
			}
		}
	}
	return facts
}

// itjobsClean collapses whitespace, the &nbsp; padding the site puts around every fact included.
func itjobsClean(s string) string { return strings.Join(strings.Fields(s), " ") }

// itjobsBaseJob is the body-less Job a card alone supports: the identity, and the facts the gate
// judges on.
func itjobsBaseJob(c itjobsCard) Job {
	j := Job{
		ExternalID:        c.externalID(),
		URL:               c.url,
		Title:             c.title,
		Company:           c.company,
		Location:          itjobsJoinPlaces(strings.Split(c.places, ",")),
		WorkMode:          itjobsWorkMode(c.workModel, ""),
		FallbackCountries: []string{"pt"},
		// The board lists IT postings only.
		IsTechHint: true,
	}
	j.Remote = j.WorkMode == "remote"
	return j
}

func itjobsJoinPlaces(places []string) string {
	var out []string
	for _, p := range places {
		if p = itjobsClean(p); p != "" && !slices.Contains(out, p) {
			out = append(out, p)
		}
	}
	return strings.Join(out, "; ")
}

// itjobsLD is the part of the posting page's schema.org JobPosting the adapter reads.
type itjobsLD struct {
	Title           string          `json:"title"`
	Description     string          `json:"description"`
	DatePosted      string          `json:"datePosted"`
	EmploymentType  json.RawMessage `json:"employmentType"`
	JobLocationType string          `json:"jobLocationType"`
	// JobLocation is an array of Places — or ONE Place, carrying only a country, on a remote
	// posting.
	JobLocation        schemaPlaces `json:"jobLocation"`
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

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410) or a page carrying no usable posting; an error is the absence of an answer.
func (s itjobs) detail(ctx context.Context, c itjobsCard) (Job, bool, error) {
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
	var ld itjobsLD
	if !ldJobPosting(root, &ld) {
		// The posting was listed this run, so a page without its JobPosting is not an answer
		// that it is gone (a gone posting answers 404); it is a page this run could not read.
		return Job{}, false, fmt.Errorf("page carries no JobPosting")
	}
	job, ok := itjobsJob(ld, c)
	return job, ok, nil
}

// itjobsJob maps a posting's JobPosting and its card to a Job. ok is false for one with no
// title, employer or body.
func itjobsJob(ld itjobsLD, c itjobsCard) (Job, bool) {
	j := itjobsBaseJob(c)
	if t := itjobsClean(ld.Title); t != "" {
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
	for _, p := range ld.JobLocation {
		city, region := strings.TrimSpace(p.Address.AddressLocality), strings.TrimSpace(p.Address.AddressRegion)
		switch {
		case city != "" && region != "" && !strings.EqualFold(city, region):
			places = append(places, city+", "+region)
		case city != "":
			places = append(places, city)
		case region != "":
			places = append(places, region)
		}
		if code := location.NormalizeCountry(strings.TrimSpace(p.Address.AddressCountry)); code != "" && !slices.Contains(j.Countries, code) {
			j.Countries = append(j.Countries, code)
		}
	}
	if len(places) > 0 {
		j.Location = itjobsJoinPlaces(places)
	}
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	j.WorkMode = itjobsWorkMode(c.workModel, ld.JobLocationType)
	j.Remote = j.WorkMode == "remote"
	j.EmploymentType = itjobsEmploymentType(ld.EmploymentType)
	sal := ld.BaseSalary
	if sal.Currency != "" && strings.EqualFold(sal.Value.UnitText, "YEAR") && sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(strings.TrimSpace(sal.Currency))
		j.SalaryPeriod = "year"
	}
	return j, true
}

// itjobsWorkMode reads the card's work-model line — the site's own structured field, which every
// posting carries — and falls back to the JobPosting's TELECOMMUTE, which the site sets on fully
// remote postings only.
func itjobsWorkMode(workModel, locationType string) string {
	switch strings.ToLower(strings.TrimSpace(workModel)) {
	case "remoto":
		return "remote"
	case "híbrido", "hibrido":
		return "hybrid"
	case "presencial":
		return "onsite"
	}
	if strings.EqualFold(strings.TrimSpace(locationType), "TELECOMMUTE") {
		return "remote"
	}
	return ""
}

// itjobsEmploymentType reads employmentType, one enum or a list. A list is the employer offering
// several contracts ("Contrato sem termo, Prestação de Serviços" is FULL_TIME and CONTRACTOR), so
// only a list that maps to ONE value states it; one that does not states nothing.
func itjobsEmploymentType(raw json.RawMessage) string {
	var types []string
	var one string
	if json.Unmarshal(raw, &one) == nil {
		types = []string{one}
	} else if json.Unmarshal(raw, &types) != nil {
		return ""
	}
	out := ""
	for _, t := range types {
		mapped := schemaEmploymentType(t)
		if mapped == "" {
			continue
		}
		if out != "" && out != mapped {
			return ""
		}
		out = mapped
	}
	return out
}
