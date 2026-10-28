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
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
	"golang.org/x/time/rate"
)

// digitalrecruiters adapts Cegid DigitalRecruiters, a French ATS whose customers — Clariane,
// O2, Segula, Decathlon, Newrest, Burger King France, Quebec's health networks — each publish a
// career site, almost always on their OWN domain (careers.segulatechnologies.com,
// emploi.o2recrute.fr), CNAMEd to external-career.digitalrecruiters.com. The board is that
// HOST. Nothing about it is derivable from a URL, so boards are found from the vendor's DNS and
// from employers' career links.
//
// Every site is a Nuxt app over one public JSON API on api.digitalrecruiters.com, which this
// adapter reads directly:
//
//	GET  /careers/v1/careers-sites/<host>                     the site: locale, online, internal
//	POST /public/v1/careers-site/job-ads?domainName=<host>&…  the listing, count + items
//	GET  /public/v1/careers-site/job-ads/<job ad id>?…        one posting, with its body
//
// The listing takes any page size, so a site is one request and its stated count is checked
// exactly (fullBoardListing). It carries no body, so postings are hydrated, new ones only.
//
// A site may AGGREGATE other sites' postings (recrute.ouicare.com lists 3,802, of which 208 are
// its own and 3,098 are emploi.o2recrute.fr's); each item names the site that owns it in
// career_domain, and only a board's own postings are read — the owner's board carries the rest,
// and two boards storing one posting would be a duplicate the pipeline cannot see.
//
// The API host publishes no robots.txt; the sites declare "Crawl-delay: 10" and are not read.
// Every request of the run goes through one limiter at drVendorRate, so the run's posting reads
// are bounded across ALL boards (runBudget), not only per board: the first crawl spent its 40
// minutes hydrating the first ~45 sites, 300 postings each, and 160 sites never got their
// listing read before the deadline (2026-10-28).
type digitalrecruiters struct {
	http           digitalrecruitersHTTP
	limiter        waiter
	maxNewPerRun   int
	runBudget      *atomic.Int64
	deadlineMargin time.Duration
}

// digitalrecruitersHTTP is the transport the adapter needs: JSON GETs and the listing's POST.
type digitalrecruitersHTTP interface {
	JSONGetter
	JSONPoster
}

// NewDigitalrecruiters builds the DigitalRecruiters adapter over the given HTTP client.
func NewDigitalrecruiters(c digitalrecruitersHTTP) Source {
	return digitalrecruiters{
		http:           c,
		limiter:        rate.NewLimiter(drVendorRate, 1),
		maxNewPerRun:   drMaxNewPerBoard,
		runBudget:      drRunBudget(),
		deadlineMargin: drDeadlineMargin,
	}
}

func (digitalrecruiters) Provider() string { return "digitalrecruiters" }

// fullBoardListing: the listing is read whole and its item count is checked against the count
// the API states; any shortfall fails the crawl. See the fullBoardListing interface.
func (digitalrecruiters) fullBoardListing() {}

const (
	drAPI = "https://api.digitalrecruiters.com"
	// drVendorRate is one request a second across the whole run: one API host serves every site.
	drVendorRate = rate.Limit(1)
	// drPageSize is the listing page. The API honours any size, but answers slowly: the
	// 3,802-posting recrute.ouicare.com took 33 s in one page and 6 s for a page of 1,000.
	drPageSize = 1000
	// drMaxPages bounds the walk; needing more fails the crawl.
	drMaxPages = 50
	// drMaxNewPerBoard bounds the postings one site hydrates in a run, so a 3,000-posting site
	// does not spend the run's whole budget.
	drMaxNewPerBoard = 150
	// drMaxNewPerRun bounds the postings the whole run hydrates. At one request a second the
	// ~350 sites' listings take two requests each (~12 minutes); 1,200 posting reads (20
	// minutes) leave the rest of the 50-minute run for the pipeline's writes and slow pages.
	drMaxNewPerRun = 1200
	// drDeadlineMargin is the time hydration leaves the pipeline to write what it read.
	drDeadlineMargin = 5 * time.Minute
)

var (
	// drBoard is a bare lowercase host.
	drBoard = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+$`)
	// drSlug is a posting's URL slug: "<job ad id>-<title words>", the words in any script
	// ("…-vandœuvre-les-nancy", "…-niepołomice").
	drSlug = regexp.MustCompile(`^[0-9]+(-[^/?#\s]*)?$`)
)

// drLocales is the site app's own mapping from a site's locale code to the API's locale: a code
// the API does not know is refused with HTTP 400.
var drLocales = map[string]string{
	"cs": "cs_CZ", "da": "da_DK", "de_CH": "de_CH", "de": "de_DE", "en": "en_GB", "es": "es_ES",
	"it_CH": "it_CH", "it": "it_IT", "ja": "ja_JP", "ko": "ko_KR", "fr_CA": "fr_CA", "fr_CH": "fr_CH",
	"fr": "fr_FR", "nl": "nl_BE", "nl_NL": "nl_NL", "pl": "pl_PL", "pt": "pt_PT", "pt_BR": "pt_BR",
	"ro": "ro_RO", "sv": "sv_SE", "tr": "tr_TR", "zh_CN": "zh_CN", "zh_HK": "zh_HK",
}

// DigitalrecruitersAPILocale maps a site locale code to the API's, as the site app does: the
// full code, then its language, then the code itself. Exported for cmd/harvest-boards' prober,
// which must ask the API in the same locale.
func DigitalrecruitersAPILocale(code string) string {
	code = strings.ReplaceAll(strings.TrimSpace(code), "-", "_")
	if l, ok := drLocales[code]; ok {
		return l
	}
	if len(code) >= 2 {
		if l, ok := drLocales[code[:2]]; ok {
			return l
		}
	}
	return code
}

// drSite is the careers-site configuration.
type drSite struct {
	DomainName    string `json:"domain_name"`
	IsOnline      bool   `json:"is_online"`
	IsInternal    bool   `json:"is_internal"`
	DefaultLocale struct {
		ISOCode string `json:"iso_code"`
	} `json:"default_locale"`
}

// drListing is one page of the listing.
type drListing struct {
	Count *int     `json:"count"`
	Items []drItem `json:"items"`
}

// drItem is one listed posting.
type drItem struct {
	JobAdID      int64  `json:"job_ad_id"`
	Title        string `json:"title"`
	Location     string `json:"location"`
	URL          string `json:"url"`
	CareerDomain string `json:"career_domain"`
}

// drPosting is a listed posting the board owns, with the page address it is published at.
type drPosting struct {
	id, url string
	item    drItem
	// shell marks an item listed without a slug, whose page cannot be read (see list).
	shell bool
}

// drDetail is one posting from the job-ad endpoint, the fields the adapter maps.
type drDetail struct {
	Title          string          `json:"title"`
	BrandName      string          `json:"brand_name"`
	Description    string          `json:"description"`
	Profile        string          `json:"profile"`
	AdditionalInfo json.RawMessage `json:"additional_info"`
	WorkingTime    string          `json:"working_time"`
	Contract       string          `json:"contract"`
	Address        *struct {
		City    string `json:"city"`
		State   string `json:"state"`
		Country string `json:"country"`
	} `json:"address"`
	JSONLD *struct {
		DatePosted     string `json:"datePosted"`
		EmploymentType any    `json:"employmentType"`
		BaseSalary     *struct {
			Currency string `json:"currency"`
			Value    struct {
				MinValue *float64 `json:"minValue"`
				MaxValue *float64 `json:"maxValue"`
				UnitText string   `json:"unitText"`
			} `json:"value"`
		} `json:"baseSalary"`
	} `json:"jsonld"`
}

// Fetch is the list-only fallback HydratingSource specifies; it hydrates every posting.
func (s digitalrecruiters) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the site and its listing, and buys a posting only for an id the catalogue does
// not hold, at most maxNewPerRun of them and none once the run nears its deadline. A seen posting
// is re-listed as a SeenRefresh; an unseen one left unread waits for a later run.
func (s digitalrecruiters) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.ToLower(strings.TrimSuffix(strings.TrimSpace(e.Board), "/"))
	if !drBoard.MatchString(board) {
		return nil, fmt.Errorf("digitalrecruiters: board %q is not a careers host", e.Board)
	}
	site, err := s.site(ctx, board)
	if err != nil {
		return nil, err
	}
	locale := DigitalrecruitersAPILocale(site.DefaultLocale.ISOCode)
	postings, err := s.list(ctx, board, locale, site.DefaultLocale.ISOCode)
	if err != nil {
		return nil, err
	}
	var jobs []Job
	var fresh []drPosting
	deferred := 0
	for _, p := range postings {
		switch {
		case seen(p.id):
			jobs = append(jobs, Job{ExternalID: p.id, URL: p.url, Title: strings.TrimSpace(p.item.Title), Company: e.Company, SeenRefresh: true})
		case p.shell:
			// Nothing to buy: an unstored shell has no page.
		case len(fresh) < s.maxNewPerRun:
			fresh = append(fresh, p)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("digitalrecruiters: board %q: hydration budget %d reached; %d new postings deferred to a later run",
			board, s.maxNewPerRun, deferred)
	}
	outOfTime, outOfBudget := 0, 0
	for _, p := range fresh {
		switch {
		case s.nearDeadline(ctx):
			outOfTime++
			continue
		case !s.spend():
			outOfBudget++
			continue
		}
		if j, ok := s.detail(ctx, board, locale, e, p); ok {
			jobs = append(jobs, j)
		}
	}
	if outOfTime > 0 || outOfBudget > 0 {
		log.Printf("digitalrecruiters: board %q: %d new postings left unread (%d near the run's deadline, %d past the run's budget of %d), deferred to later runs",
			board, outOfTime+outOfBudget, outOfTime, outOfBudget, drMaxNewPerRun)
	}
	return jobs, nil
}

// drRunBudget is a fresh run-wide posting budget; the registry builds the adapter once per run.
func drRunBudget() *atomic.Int64 {
	b := new(atomic.Int64)
	b.Store(drMaxNewPerRun)
	return b
}

// spend takes one posting read from the run-wide budget, reporting false once it is gone.
func (s digitalrecruiters) spend() bool {
	return s.runBudget == nil || s.runBudget.Add(-1) >= 0
}

// nearDeadline reports whether the run's deadline is closer than the margin hydration leaves to
// the pipeline's own writes.
func (s digitalrecruiters) nearDeadline(ctx context.Context) bool {
	d, ok := ctx.Deadline()
	return ok && time.Until(d) < s.deadlineMargin
}

// wait spaces every request of the run. When the limiter can already tell that no slot opens
// before the run's deadline it refuses at once, ahead of the deadline itself; this request is
// then never going to be sent, so wait sits the deadline out and returns the context's own
// error, which the pipeline reads as an interrupted board rather than a failed one.
func (s digitalrecruiters) wait(ctx context.Context) error {
	if s.limiter == nil {
		return nil
	}
	if err := s.limiter.Wait(ctx); err != nil {
		<-ctx.Done()
		return ctx.Err()
	}
	return nil
}

// site reads the careers-site configuration. A site the platform does not know (404) or has
// taken offline is gone; an internal-mobility site is for the employer's own staff and is not a
// board.
func (s digitalrecruiters) site(ctx context.Context, board string) (drSite, error) {
	var site drSite
	if err := s.wait(ctx); err != nil {
		return site, err
	}
	err := s.http.GetJSON(ctx, drAPI+"/careers/v1/careers-sites/"+url.PathEscape(board), &site)
	var se *StatusError
	switch {
	case errors.As(err, &se) && se.Code == http.StatusNotFound:
		return site, fmt.Errorf("digitalrecruiters: site %s: %w", board, ErrBoardGone)
	case err != nil:
		return site, fmt.Errorf("digitalrecruiters: site %s: %w", board, err)
	case !site.IsOnline:
		return site, fmt.Errorf("digitalrecruiters: site %s is offline: %w", board, ErrBoardGone)
	case site.IsInternal:
		return site, fmt.Errorf("digitalrecruiters: site %s is an internal-mobility site, not a public board", board)
	case strings.TrimSpace(site.DefaultLocale.ISOCode) == "":
		return site, fmt.Errorf("digitalrecruiters: site %s states no locale", board)
	}
	return site, nil
}

// list reads the whole listing, checks it against the stated count, and keeps the postings the
// board owns.
func (s digitalrecruiters) list(ctx context.Context, board, locale, pathLocale string) ([]drPosting, error) {
	var items []drItem
	total := -1
	for page := 1; ; page++ {
		if page > drMaxPages {
			return nil, fmt.Errorf("digitalrecruiters: listing %s: %d postings need more than %d pages", board, total, drMaxPages)
		}
		if err := s.wait(ctx); err != nil {
			return nil, err
		}
		q := url.Values{
			"domainName": {board}, "locale": {locale},
			"limit": {strconv.Itoa(drPageSize)}, "page": {strconv.Itoa(page)},
		}
		var resp drListing
		if err := s.http.PostJSON(ctx, drAPI+"/public/v1/careers-site/job-ads?"+q.Encode(), map[string]any{"filters": map[string]any{}}, &resp); err != nil {
			return nil, fmt.Errorf("digitalrecruiters: listing %s page %d: %w", board, page, err)
		}
		if resp.Count == nil {
			return nil, fmt.Errorf("digitalrecruiters: listing %s: the answer states no count", board)
		}
		total = *resp.Count
		items = append(items, resp.Items...)
		if len(items) >= total || len(resp.Items) == 0 {
			break
		}
	}
	if len(items) != total {
		return nil, fmt.Errorf("digitalrecruiters: listing %s: read %d postings of the %d it states", board, len(items), total)
	}
	var out []drPosting
	seen := map[int64]bool{}
	usable, unusable := 0, 0
	for _, it := range items {
		if !strings.EqualFold(strings.TrimSpace(it.CareerDomain), board) || seen[it.JobAdID] {
			continue // another site's posting this one aggregates, or a second address of one
		}
		// A listed ad can be an empty shell — no slug and no title (emploi.o2recrute.fr's
		// 4636315, 2026-10-28) — whose page the site cannot render. Failing the board over it
		// would stop the site's every other posting too, so it is kept as a shell: if the
		// catalogue already holds that id (a posting whose slug went missing later) its liveness
		// is refreshed, and otherwise nothing is bought for it.
		if it.JobAdID <= 0 {
			unusable++
			continue
		}
		seen[it.JobAdID] = true
		p := drPosting{id: strconv.FormatInt(it.JobAdID, 10), item: it}
		if drSlug.MatchString(it.URL) {
			page := url.URL{Scheme: "https", Host: board, Path: "/" + pathLocale + "/annonce/" + it.URL}
			p.url = page.String()
			usable++
		} else {
			p.shell = true
			p.url = (&url.URL{Scheme: "https", Host: board, Path: "/" + pathLocale + "/annonces"}).String()
			unusable++
		}
		out = append(out, p)
	}
	if unusable > 0 {
		log.Printf("digitalrecruiters: listing %s: %d item(s) with no usable id or slug", board, unusable)
		if usable == 0 {
			return nil, fmt.Errorf("digitalrecruiters: listing %s: none of its %d own items has a usable id or slug", board, unusable)
		}
	}
	return out, nil
}

// detail buys one posting. A 404/410 drops it (withdrawn since the listing); any other failure,
// or a posting without a body, is an unreadable marker.
func (s digitalrecruiters) detail(ctx context.Context, board, locale string, e CompanyEntry, p drPosting) (Job, bool) {
	if err := s.wait(ctx); err != nil {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	q := url.Values{"domainName": {board}, "locale": {locale}, "withJsonld": {"1"}}
	var d drDetail
	if err := s.http.GetJSON(ctx, drAPI+"/public/v1/careers-site/job-ads/"+p.id+"?"+q.Encode(), &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		return Job{}, false
	}
	infoTitle, infoText := drAdditionalInfo(d.AdditionalInfo)
	description := sanitizeHTML(d.Description + headedSection("Profile", d.Profile) + headedSection(firstNonEmpty(infoTitle, "Additional information"), infoText))
	title := firstNonEmpty(strings.TrimSpace(d.Title), strings.TrimSpace(p.item.Title))
	if strings.TrimSpace(description) == "" || title == "" {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	j := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          title,
		Company:        firstNonEmpty(strings.TrimSpace(d.BrandName), e.Company),
		Description:    description,
		Location:       strings.TrimSpace(p.item.Location),
		EmploymentType: drEmploymentType(d),
	}
	if a := d.Address; a != nil {
		j.Location = firstNonEmpty(joinNonEmpty(a.City, a.State, a.Country), j.Location)
		if c := location.NormalizeCountry(a.Country); c != "" {
			j.Countries = []string{c}
		}
	}
	if ld := d.JSONLD; ld != nil {
		j.PostedAt = NotFuture(parseRFC3339OrDate(ld.DatePosted))
		if sal := ld.BaseSalary; sal != nil && sal.Currency != "" {
			if period := schemaSalaryPeriod(sal.Value.UnitText); period != "" {
				if sal.Value.MinValue != nil {
					j.SalaryMin = roundSalaryPart(*sal.Value.MinValue)
				}
				if sal.Value.MaxValue != nil {
					j.SalaryMax = roundSalaryPart(*sal.Value.MaxValue)
				}
				if j.SalaryMin != nil || j.SalaryMax != nil {
					j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(strings.TrimSpace(sal.Currency)), period
				}
			}
		}
	}
	return j, true
}

// drAdditionalInfo reads the posting's extra section, which the API sends as an object
// ({"title": "L'établissement", "text": "<p>…"}) or null.
func drAdditionalInfo(raw json.RawMessage) (title, text string) {
	var info struct {
		Title string `json:"title"`
		Text  string `json:"text"`
	}
	if json.Unmarshal(raw, &info) != nil {
		return "", ""
	}
	return strings.TrimSpace(info.Title), info.Text
}

// drEmploymentType reads the posting's working time ("Full-time", "Temps partiel"), then its
// contract for the kinds a working time cannot state (an internship, a freelance contract).
func drEmploymentType(d drDetail) string {
	text := strings.ToLower(d.Contract + " " + d.WorkingTime)
	switch {
	case strings.Contains(text, "stage"), strings.Contains(text, "internship"), strings.Contains(text, "stagiaire"):
		return "internship"
	case strings.Contains(text, "freelance"), strings.Contains(text, "intérim"), strings.Contains(text, "interim"):
		return "contract"
	}
	switch strings.ToLower(strings.TrimSpace(d.WorkingTime)) {
	case "full-time", "temps plein", "temps complet", "vollzeit", "tiempo completo", "tempo pieno", "temps plein / full-time":
		return "full_time"
	case "part-time", "temps partiel", "teilzeit", "tiempo parcial", "tempo parziale":
		return "part_time"
	}
	return ""
}
