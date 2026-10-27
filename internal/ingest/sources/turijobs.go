package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/url"
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

// turijobs adapts Turijobs (www.turijobs.com), Spain's hospitality and tourism job board: ~2.5k
// open postings from hotel chains, restaurants and travel agencies, most in Spain and the rest
// in Portugal, France and the Caribbean. It is boardless and an aggregator — the employer is read
// per posting. See the "Turijobs traps" section of AGENTS.md for the measurements behind each
// rule.
//
// The posting set is the active-offers sitemap, which carries only each posting's URL, so every
// fact is bought with the posting page's schema.org JobPosting. The pages sit behind Akamai bot
// management, which holds a plain client's request open without answering; the impersonating
// fetcher of the Scrapling tier is served (see scraplingProviders). The sitemap is not behind it.
// The adapter hydrates only postings the catalogue lacks, newest id first, and refreshes the
// rest by identity.
type turijobs struct {
	sitemap        XMLGetter
	pages          HTMLGetter
	limiter        waiter
	maxNewPerRun   int
	deadlineMargin time.Duration
}

const (
	turijobsHost    = "www.turijobs.com"
	turijobsBaseURL = "https://" + turijobsHost
	// turijobsSitemapURL lists every open posting; the pt-pt and es-mx editions list the same
	// set under their own prefixes.
	turijobsSitemapURL = turijobsBaseURL + "/es-es/sitemap/active-offers.xml"

	// turijobsRequestInterval paces every posting page at one a second — a courtesy ceiling,
	// not a measured limit.
	turijobsRequestInterval = time.Second
	turijobsDetailWorkers   = 2
	// turijobsMaxNewPerRun bounds how many unseen postings one run reads: 600 pages at one a
	// second is ten minutes. The first runs face the whole ~2.5k catalogue, so it is taken in
	// slices, newest first.
	turijobsMaxNewPerRun   = 600
	turijobsDeadlineMargin = 5 * time.Minute
)

// NewTurijobs builds the adapter: the sitemap is read over sitemap, the posting pages over pages.
func NewTurijobs(sitemap XMLGetter, pages HTMLGetter) Source {
	return turijobs{
		sitemap:        sitemap,
		pages:          pages,
		limiter:        rate.NewLimiter(rate.Every(turijobsRequestInterval), 1),
		maxNewPerRun:   turijobsMaxNewPerRun,
		deadlineMargin: turijobsDeadlineMargin,
	}
}

func (turijobs) Provider() string { return "turijobs" }

// One national board, no per-tenant id.
func (turijobs) boardless() {}

// Hundreds of hotel groups, restaurants and agencies post here, each named per posting.
func (turijobs) aggregator() {}

// turijobsPosting is one sitemap entry: the id, the URL, and the title its path spells — all the
// title gate needs before the page is bought. The path names no employer.
type turijobsPosting struct {
	id    int64
	url   string
	title string
}

func (p turijobsPosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// turijobsPostingPath matches a posting URL, /<locale>/oferta-trabajo/<place>/<title>/<id>. The
// slugs are cosmetic; the id is the identity.
var turijobsPostingPath = regexp.MustCompile(`^/[a-z]{2}-[a-z]{2}/oferta-trabajo/[^/]+/([^/]+)/(\d+)/?$`)

func turijobsParsePosting(raw string) (turijobsPosting, bool) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Host != turijobsHost {
		return turijobsPosting{}, false
	}
	m := turijobsPostingPath.FindStringSubmatch(u.Path)
	if m == nil {
		return turijobsPosting{}, false
	}
	id, err := strconv.ParseInt(m[2], 10, 64)
	if err != nil || id <= 0 {
		return turijobsPosting{}, false
	}
	title := strings.Join(strings.Fields(strings.ReplaceAll(m[1], "-", " ")), " ")
	return turijobsPosting{id: id, url: raw, title: title}, true
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s turijobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s turijobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	return s.FetchNewGated(ctx, e, seen, DetailGate{})
}

// FetchNewGated lists the sitemap, refreshes the postings already stored, and reads up to
// maxNewPerRun of the rest, newest id first. A posting whose title the catalogue filter will
// reject is yielded body-less without a request; the coverage half of the gate cannot be asked,
// since nothing before the page names the employer. A posting whose page cannot be read is not
// yielded; a later run tries again.
func (s turijobs) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var unseen []turijobsPosting
	deferred := 0
	for _, p := range postings {
		base := Job{ExternalID: p.externalID(), URL: p.url, Title: p.title}
		if seen(base.ExternalID) {
			base.SeenRefresh = true
			jobs = append(jobs, base)
			continue
		}
		if gate.Rejected != nil && gate.Rejected(base) {
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
	read := fetchDetails(unseen, turijobsDetailWorkers, func(p turijobsPosting) (Job, bool) {
		if d, ok := ctx.Deadline(); ok && time.Until(d) < s.deadlineMargin {
			outOfTime.Add(1)
			return Job{}, false
		}
		job, ok, err := s.detail(ctx, p)
		if err != nil {
			log.Printf("turijobs: posting %d failed; deferring: %v", p.id, err)
			return Job{}, false
		}
		answered.Add(1)
		return job, ok
	})
	if asked := int64(len(unseen)) - outOfTime.Load(); asked > 0 && answered.Load() == 0 {
		return nil, fmt.Errorf("turijobs: requested %d posting pages and none answered", asked)
	}
	if n := outOfTime.Load(); n > 0 {
		log.Printf("turijobs: %d new postings left unread as the run neared its deadline, deferred to later runs", n)
	}
	if deferred > 0 {
		log.Printf("turijobs: %d new postings past this run's bound of %d, deferred to later runs", deferred, s.maxNewPerRun)
	}
	return append(jobs, read...), nil
}

// list reads the active-offers sitemap, newest id first. It failing fails the crawl.
func (s turijobs) list(ctx context.Context) ([]turijobsPosting, error) {
	doc, err := getSitemap(ctx, s.sitemap, turijobsSitemapURL)
	if err != nil {
		return nil, fmt.Errorf("turijobs: sitemap: %w", err)
	}
	ids := make(map[int64]bool)
	var out []turijobsPosting
	for _, u := range doc.URLs {
		p, ok := turijobsParsePosting(strings.TrimSpace(u.Loc))
		if !ok || ids[p.id] {
			continue
		}
		ids[p.id] = true
		out = append(out, p)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("turijobs: sitemap lists no posting")
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// turijobsLD is the part of the posting page's schema.org JobPosting the adapter reads.
type turijobsLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
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

type turijobsPlace struct {
	Address struct {
		Locality string `json:"addressLocality"`
		Region   string `json:"addressRegion"`
		Country  string `json:"addressCountry"`
	} `json:"address"`
}

// oneOrMany decodes a schema.org field the site writes as one value or as a list.
func turijobsOneOrMany[T any](raw json.RawMessage) []T {
	var one T
	if err := json.Unmarshal(raw, &one); err == nil {
		return []T{one}
	}
	var many []T
	_ = json.Unmarshal(raw, &many)
	return many
}

// errTurijobsNoPosting marks a page that answered without a JobPosting. It is not the site's
// answer that the posting is gone — a bot-management interstitial answers the same way — so it
// counts as no answer.
var errTurijobsNoPosting = errors.New("page carries no JobPosting")

// detail reads one posting page. ok=false with a nil error is the site's answer that the posting
// is gone (404/410); an error is the absence of an answer.
func (s turijobs) detail(ctx context.Context, p turijobsPosting) (Job, bool, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false, err
	}
	root, err := s.pages.GetHTML(ctx, p.url)
	if err != nil {
		var se *StatusError
		if errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
			return Job{}, false, nil
		}
		return Job{}, false, err
	}
	var ld turijobsLD
	if !ldJobPosting(root, &ld) {
		// ~10% of pages (measured on the first crawl, 56 of 600) carry no JobPosting; the
		// page's own Next.js state always holds the offer.
		next, found, blind := turijobsFromNextData(root)
		if !found {
			return Job{}, false, errTurijobsNoPosting
		}
		if blind {
			// The employer chose to stay hidden: a posting under no company is not stored.
			return Job{}, false, nil
		}
		ld = next
	}
	job, ok := turijobsJob(ld, p)
	return job, ok, nil
}

// turijobsNextData is the part of the page's __NEXT_DATA__ the adapter reads.
type turijobsNextData struct {
	Props struct {
		PageProps struct {
			OfferData struct {
				OfferDetail *struct {
					Title           string `json:"title"`
					Description     string `json:"description"`
					PublicationDate string `json:"publicationDate"`
					IsBlind         bool   `json:"isBlind"`
					Location        struct {
						City    string `json:"cityName"`
						Region  string `json:"regionName"`
						Country string `json:"countryISO"`
					} `json:"location"`
					Company struct {
						BrandName      string `json:"brandName"`
						EnterpriseName string `json:"enterpriseName"`
					} `json:"company"`
				} `json:"offerDetail"`
			} `json:"offerData"`
		} `json:"pageProps"`
	} `json:"props"`
}

// turijobsFromNextData reads the offer from the page's Next.js state, as the JobPosting the
// adapter otherwise reads. found is false when the page holds no offer; blind reports an offer
// whose employer is hidden.
func turijobsFromNextData(root *html.Node) (ld turijobsLD, found, blind bool) {
	raw := scriptTextByID(root, "__NEXT_DATA__")
	if raw == "" {
		return turijobsLD{}, false, false
	}
	var nd turijobsNextData
	if err := json.Unmarshal([]byte(raw), &nd); err != nil {
		return turijobsLD{}, false, false
	}
	o := nd.Props.PageProps.OfferData.OfferDetail
	if o == nil || strings.TrimSpace(o.Title) == "" {
		return turijobsLD{}, false, false
	}
	if o.IsBlind {
		return turijobsLD{}, true, true
	}
	ld.Title = o.Title
	ld.Description = o.Description
	if len(o.PublicationDate) >= 10 {
		ld.DatePosted = o.PublicationDate[:10]
	}
	ld.HiringOrganization.Name = o.Company.BrandName
	if strings.TrimSpace(ld.HiringOrganization.Name) == "" {
		ld.HiringOrganization.Name = o.Company.EnterpriseName
	}
	var place turijobsPlace
	place.Address.Locality, place.Address.Region, place.Address.Country = o.Location.City, o.Location.Region, o.Location.Country
	ld.JobLocation, _ = json.Marshal(place)
	return ld, true, false
}

// turijobsJob maps a posting's JobPosting to a Job. ok is false for one with no title, employer
// or body.
func turijobsJob(ld turijobsLD, p turijobsPosting) (Job, bool) {
	title := strings.Join(strings.Fields(ld.Title), " ")
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	body := sanitizeHTML(ld.Description)
	if title == "" || company == "" || strings.TrimSpace(body) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:  p.externalID(),
		URL:         p.url,
		Title:       title,
		Company:     company,
		Description: body,
	}
	var places []string
	for _, pl := range turijobsOneOrMany[turijobsPlace](ld.JobLocation) {
		city, region := strings.TrimSpace(pl.Address.Locality), strings.TrimSpace(pl.Address.Region)
		switch {
		case city != "" && region != "" && !strings.EqualFold(city, region):
			places = append(places, city+", "+region)
		case city != "":
			places = append(places, city)
		case region != "":
			places = append(places, region)
		}
		if code := location.NormalizeCountry(strings.TrimSpace(pl.Address.Country)); code != "" && !slices.Contains(j.Countries, code) {
			j.Countries = append(j.Countries, code)
		}
	}
	j.Location = strings.Join(places, "; ")
	if t, err := time.Parse("2006-01-02", strings.TrimSpace(ld.DatePosted)); err == nil {
		j.PostedAt = NotFuture(&t)
	}
	if strings.EqualFold(strings.TrimSpace(ld.JobLocationType), "TELECOMMUTE") {
		j.WorkMode, j.Remote = "remote", true
	}
	for _, t := range turijobsOneOrMany[string](ld.EmploymentType) {
		switch strings.ToUpper(strings.TrimSpace(t)) {
		case "FULL_TIME":
			j.EmploymentType = "full_time"
		case "PART_TIME":
			j.EmploymentType = "part_time"
		case "INTERN", "INTERNSHIP":
			j.EmploymentType = "internship"
		}
		if j.EmploymentType != "" {
			break
		}
	}
	sal := ld.BaseSalary
	period := map[string]string{"YEAR": "year", "MONTH": "month"}[strings.ToUpper(strings.TrimSpace(sal.Value.UnitText))]
	if sal.Currency != "" && period != "" && sal.Value.MinValue > 0 && sal.Value.MaxValue >= sal.Value.MinValue {
		lo, hi := int(sal.Value.MinValue), int(sal.Value.MaxValue)
		j.SalaryMin, j.SalaryMax = &lo, &hi
		j.SalaryCurrency = strings.ToUpper(strings.TrimSpace(sal.Currency))
		j.SalaryPeriod = period
	}
	return j, true
}
