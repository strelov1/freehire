package aggregators

import (
	"github.com/strelov1/freehire/internal/ingest/sources/kit"

	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"time"

	"golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// indeed reads Indeed's public job search the way a signed-out visitor sees it. Board is the
// search keyword (`q`), Region the location (`l`, optional: empty searches the whole country).
// tenants.site picks the country site — `www` (the US, the default) or a two-letter subdomain
// (`de`, `uk`, `ca`) — and tenants.fromage the age window in days (1, 3, 7 or 14; default 1).
//
// What a guest gets, measured 2026-11-24 through the crawl host's browser: the FIRST page of a
// search, 15 cards, newest first. Page two (`start=10`) and the posting page (`/viewjob`) both
// redirect to sign-in, so a search is a 15-card slice and nothing deeper. The body is not on the
// card; the same search page carries the full description of ONE selected posting, and `vjk=<key>`
// selects which — that is how the site's own two-pane view opens a card. So a new posting costs one
// search-page navigation with its key selected, and the listing page itself pays for its first card.
//
// Every request goes through the Scrapling browser from the crawl host's own IP: plain and
// fingerprinted HTTP get Cloudflare's "Security Check" (403) and the proxy pool answers 401.
//
// The data is inline script JSON; the URL the browser ended on is how a sign-in redirect is told
// from a search page.
type indeed struct {
	http kit.HTMLResolvedGetter
	pace interface{ Wait(context.Context) error }
}

const (
	// indeedRequestInterval spaces navigations across all boards. A warm browser session takes
	// ~7 s a page anyway; the spacing keeps a burst of quick answers from reading as a bot.
	indeedRequestInterval = 3 * time.Second
	// indeedSweepGrace is how long a posting may go unseen before the sweep (and cmd/liveness's
	// age rule) presume it filled. A search lists only its 15 newest cards, so a live posting is
	// normally listed for a day or two and never again; its own page is behind sign-in, so no
	// probe can tell live from gone. Indeed postings commonly run about a month.
	indeedSweepGrace  = 30 * 24 * time.Hour
	indeedDefaultSite = "www"
	indeedDefaultAge  = "1"
)

// NewIndeed builds the Indeed adapter over c, with one request budget shared by every board.
func NewIndeed(c kit.HTMLResolvedGetter) kit.Source {
	return indeed{http: c, pace: rate.NewLimiter(rate.Every(indeedRequestInterval), 1)}
}

func (indeed) Provider() string { return "indeed" }

// Indeed lists postings from many employers, so it stays in the source facet and the dedup
// pass's aggregator set.
func (indeed) Aggregator() {}

func (indeed) SweepGrace() time.Duration { return indeedSweepGrace }

var indeedJobKey = regexp.MustCompile(`^[0-9a-f]{16}$`)

// IndeedJobKey returns id when it is an Indeed job key (16 lowercase hex digits), else "". The key
// names the posting across every search and country site, so the pipeline stores it un-namespaced:
// one posting found by two keywords is one row.
func IndeedJobKey(id string) string {
	if indeedJobKey.MatchString(id) {
		return id
	}
	return ""
}

var indeedSite = regexp.MustCompile(`^(www|[a-z]{2})$`)

// indeedSearch is one board's resolved search.
type indeedSearch struct {
	host string
	list string // the search URL, newest first
}

func indeedSearchFor(e kit.CompanyEntry) (indeedSearch, error) {
	keyword := strings.TrimSpace(e.Board)
	if keyword == "" {
		return indeedSearch{}, errors.New("indeed: board keyword is required")
	}
	site := kit.FirstNonEmpty(e.Tenants["site"], indeedDefaultSite)
	if !indeedSite.MatchString(site) {
		return indeedSearch{}, fmt.Errorf("indeed: tenants.site=%q must be www or a two-letter country subdomain", site)
	}
	age := kit.FirstNonEmpty(e.Tenants["fromage"], indeedDefaultAge)
	switch age {
	case "1", "3", "7", "14":
	default:
		return indeedSearch{}, fmt.Errorf("indeed: tenants.fromage=%q must be 1, 3, 7 or 14", age)
	}
	host := site + ".indeed.com"
	q := url.Values{"q": {keyword}, "l": {strings.TrimSpace(e.Region)}, "sort": {"date"}, "fromage": {age}}
	return indeedSearch{host: host, list: "https://" + host + "/jobs?" + q.Encode()}, nil
}

func (s indeedSearch) postingURL(key string) string {
	return "https://" + s.host + "/viewjob?jk=" + key
}

// Fetch is the list-only fallback used when the pipeline supplies no seen-set: every card's body
// is read.
func (s indeed) Fetch(ctx context.Context, e kit.CompanyEntry) ([]kit.Job, error) {
	return s.fetch(ctx, e, nil)
}

// FetchNew reads the body only of cards the catalogue does not hold; a seen card is re-listed as
// an identity-only SeenRefresh, which the pipeline turns into a liveness touch.
func (s indeed) FetchNew(ctx context.Context, e kit.CompanyEntry, seen func(externalID string) bool) ([]kit.Job, error) {
	return s.fetch(ctx, e, seen)
}

func (s indeed) fetch(ctx context.Context, e kit.CompanyEntry, seen func(string) bool) ([]kit.Job, error) {
	search, err := indeedSearchFor(e)
	if err != nil {
		return nil, err
	}
	page, err := s.page(ctx, search, search.list)
	if err != nil {
		return nil, fmt.Errorf("indeed: listing: %w", err)
	}
	cards, err := indeedCards(page)
	if err != nil {
		return nil, err
	}
	// The listing page already opened one posting; reading it costs no navigation.
	opened, _ := page.viewjob()
	var jobs []kit.Job
	read, refreshed := 0, 0
	for _, c := range cards {
		j := c.job(search)
		if seen != nil && seen(j.ExternalID) {
			j.SeenRefresh = true
			refreshed++
			jobs = append(jobs, j)
			continue
		}
		detail, err := s.detail(ctx, search, opened, c.Key)
		if err != nil {
			if indeedRefused(err) {
				// A refusal ends the board. Keep what was read: the pipeline drops every job
				// returned beside an error, and nothing sweeps on this slice's absences.
				if read+refreshed > 0 {
					log.Printf("indeed: keeping %d postings after a refusal: %v", len(jobs), err)
					return jobs, nil
				}
				return nil, fmt.Errorf("indeed: posting %s: %w", c.Key, err)
			}
			j.Unreadable = true
			jobs = append(jobs, j)
			continue
		}
		detail.apply(&j)
		read++
		jobs = append(jobs, j)
	}
	if len(cards) > 0 && read+refreshed == 0 {
		return jobs, fmt.Errorf("indeed: none of %d listed postings could be read", len(cards))
	}
	log.Printf("indeed: search site=%s keyword=%q region=%q listed=%d read=%d refreshed=%d", search.host, e.Board, e.Region, len(cards), read, refreshed)
	return jobs, nil
}

// errIndeedSignIn is a page that ended on Indeed's sign-in wall instead of the search: what a
// guest gets for anything past the first search page, and what bot detection hands out.
var errIndeedSignIn = errors.New("redirected to sign-in")

func indeedRefused(err error) bool {
	var status *kit.StatusError
	return errors.Is(err, errIndeedSignIn) || (errors.As(err, &status) && (status.Code == 403 || status.Code == 429))
}

// page navigates to a search page and refuses anything that did not end on one.
func (s indeed) page(ctx context.Context, search indeedSearch, raw string) (indeedPage, error) {
	if err := s.pace.Wait(ctx); err != nil {
		return indeedPage{}, err
	}
	root, final, err := s.http.GetHTMLResolved(ctx, raw)
	if err != nil {
		return indeedPage{}, err
	}
	u, err := url.Parse(final)
	if err != nil || u.Host != search.host || u.Path != "/jobs" {
		if err == nil && strings.HasSuffix(u.Host, "indeed.com") && strings.HasPrefix(u.Path, "/auth") {
			return indeedPage{}, errIndeedSignIn
		}
		return indeedPage{}, fmt.Errorf("search page ended on %q", final)
	}
	return indeedPage{scripts: indeedScripts(root), selected: u.Query().Get("vjk")}, nil
}

// detail reads one posting's body: the listing's opened posting when it is this card, otherwise
// the same search with the card selected.
func (s indeed) detail(ctx context.Context, search indeedSearch, opened indeedViewjob, key string) (indeedViewjob, error) {
	if opened.JobKey == key {
		return opened, nil
	}
	page, err := s.page(ctx, search, search.list+"&vjk="+key)
	if err != nil {
		return indeedViewjob{}, err
	}
	if page.selected != "" && page.selected != key {
		return indeedViewjob{}, fmt.Errorf("selected %s, asked for %s", page.selected, key)
	}
	v, err := page.viewjob()
	if err != nil {
		return indeedViewjob{}, err
	}
	if v.JobKey != key {
		return indeedViewjob{}, fmt.Errorf("opened %s, asked for %s", v.JobKey, key)
	}
	return v, nil
}

type indeedPage struct {
	scripts  []string // the inline <script> bodies
	selected string   // the vjk the page ended on
}

func indeedScripts(root *html.Node) []string {
	var out []string
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" && n.FirstChild != nil {
			out = append(out, n.FirstChild.Data)
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			visit(c)
		}
	}
	if root != nil {
		visit(root)
	}
	return out
}

// scriptJSON decodes the JSON object assigned right after marker in one of the page's scripts.
func (p indeedPage) scriptJSON(marker string, v any) error {
	var rest string
	for _, s := range p.scripts {
		if i := strings.Index(s, marker); i >= 0 {
			rest = s[i+len(marker):]
			break
		}
	}
	if rest == "" {
		return fmt.Errorf("indeed: page carries no %s", marker)
	}
	brace := strings.IndexByte(rest, '{')
	if brace < 0 {
		return fmt.Errorf("indeed: %s assigns no object", marker)
	}
	if err := json.NewDecoder(strings.NewReader(rest[brace:])).Decode(v); err != nil {
		return fmt.Errorf("indeed: decode %s: %w", marker, err)
	}
	return nil
}

func (p indeedPage) viewjob() (indeedViewjob, error) {
	var data struct {
		AutoOpen *struct {
			Body *indeedViewjob `json:"body"`
		} `json:"autoOpenTwoPaneViewjobResponse"`
	}
	if err := p.scriptJSON("window._initialData", &data); err != nil {
		return indeedViewjob{}, err
	}
	if data.AutoOpen == nil || data.AutoOpen.Body == nil {
		return indeedViewjob{}, errors.New("indeed: page opened no posting")
	}
	v := *data.AutoOpen.Body
	if strings.TrimSpace(v.JobInfo.Model.Description) == "" {
		return indeedViewjob{}, fmt.Errorf("indeed: posting %s has no description", v.JobKey)
	}
	return v, nil
}

// indeedCard is one search result (window.mosaic.providerData["mosaic-provider-jobcards"]).
type indeedCard struct {
	Key         string `json:"jobkey"`
	Title       string `json:"displayTitle"`
	Company     string `json:"company"`
	Location    string `json:"formattedLocation"`
	Country     string `json:"country"`
	PubDate     int64  `json:"pubDate"`
	IndeedApply bool   `json:"indeedApplyable"`
	RemoteWork  *struct {
		Type string `json:"type"`
	} `json:"remoteWorkModel"`
	Salary *struct {
		Min  float64 `json:"min"`
		Max  float64 `json:"max"`
		Type string  `json:"type"`
	} `json:"extractedSalary"`
	SalarySnippet *struct {
		Currency string `json:"currency"`
	} `json:"salarySnippet"`
	Taxonomy []struct {
		Label      string `json:"label"`
		Attributes []struct {
			Label string `json:"label"`
		} `json:"attributes"`
	} `json:"taxonomyAttributes"`
}

const indeedCardsMarker = `window.mosaic.providerData["mosaic-provider-jobcards"]`

// indeedCards reads a search page's cards. A page without the card model is a failed listing (a
// challenge, moved markup); a model with no results is an empty search.
func indeedCards(p indeedPage) ([]indeedCard, error) {
	var data struct {
		MetaData struct {
			Model *struct {
				Results *[]indeedCard `json:"results"`
			} `json:"mosaicProviderJobCardsModel"`
		} `json:"metaData"`
	}
	if err := p.scriptJSON(indeedCardsMarker, &data); err != nil {
		return nil, err
	}
	if data.MetaData.Model == nil || data.MetaData.Model.Results == nil {
		return nil, errors.New("indeed: search page carries no result list")
	}
	var cards []indeedCard
	listed := map[string]bool{}
	for _, c := range *data.MetaData.Model.Results {
		if IndeedJobKey(c.Key) == "" || listed[c.Key] || strings.TrimSpace(c.Title) == "" || strings.TrimSpace(c.Company) == "" {
			continue
		}
		listed[c.Key] = true
		cards = append(cards, c)
	}
	return cards, nil
}

func (c indeedCard) job(search indeedSearch) kit.Job {
	j := kit.Job{
		ExternalID:     c.Key,
		URL:            search.postingURL(c.Key),
		Title:          kit.CollapseSpace(c.Title),
		Company:        strings.TrimSpace(c.Company),
		Location:       strings.TrimSpace(c.Location),
		Countries:      kit.CountryFromCode(c.Country),
		EmploymentType: c.employmentType(),
		ApplyMethod:    kit.ApplyExternal,
	}
	if c.IndeedApply {
		j.ApplyMethod = kit.ApplyOnSite
	}
	if c.PubDate > 0 {
		j.PostedAt = kit.NotFuture(kit.ParseEpochMillis(c.PubDate))
	}
	if c.RemoteWork != nil {
		j.WorkMode = indeedWorkMode(c.RemoteWork.Type)
	}
	j.Remote = j.WorkMode == "remote"
	if c.Salary != nil && c.SalarySnippet != nil {
		indeedSalary(&j, c.Salary.Min, c.Salary.Max, c.Salary.Type, c.SalarySnippet.Currency)
	}
	return j
}

// employmentType reads the card's job-types facet, which states an employment type only when it
// names exactly one: "Full-time, Contract" is two offers, not one type.
func (c indeedCard) employmentType() string {
	for _, t := range c.Taxonomy {
		if t.Label != "job-types" {
			continue
		}
		if len(t.Attributes) != 1 {
			return ""
		}
		switch t.Attributes[0].Label {
		case "Full-time", "Vollzeit":
			return "full_time"
		case "Part-time", "Teilzeit":
			return "part_time"
		case "Contract", "Befristet":
			return "contract"
		case "Internship", "Praktikum":
			return "internship"
		}
	}
	return ""
}

// indeedWorkMode reads remoteWorkModel.type. REMOTE_ALWAYS is the only value the 2026-11-24
// sample carried; a hybrid type names itself. Anything else states nothing.
func indeedWorkMode(t string) string {
	switch {
	case t == "REMOTE_ALWAYS":
		return "remote"
	case strings.Contains(t, "HYBRID"):
		return "hybrid"
	}
	return ""
}

// indeedSalary sets a salary only when Indeed states a currency and a period freehire models.
// WEEKLY has no salary_period of ours and is left out rather than converted.
func indeedSalary(j *kit.Job, lo, hi float64, period, currency string) {
	periods := map[string]string{"YEARLY": "year", "MONTHLY": "month", "DAILY": "day", "HOURLY": "hour"}
	p, ok := periods[period]
	if !ok || currency == "" || lo <= 0 {
		return
	}
	if hi < lo {
		hi = lo
	}
	j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = kit.RoundSalaryPart(lo), kit.RoundSalaryPart(hi), currency, p
}

// indeedViewjob is the opened posting (window._initialData.autoOpenTwoPaneViewjobResponse.body).
type indeedViewjob struct {
	JobKey  string `json:"jobKey"`
	JobInfo struct {
		Model struct {
			Description string `json:"sanitizedJobDescription"`
			Header      struct {
				Company string `json:"companyName"`
			} `json:"jobInfoHeaderModel"`
		} `json:"jobInfoModel"`
	} `json:"jobInfoWrapperModel"`
	Salary *struct {
		Min      float64 `json:"salaryMin"`
		Max      float64 `json:"salaryMax"`
		Type     string  `json:"salaryType"`
		Currency string  `json:"salaryCurrency"`
	} `json:"salaryInfoModel"`
}

// apply adds what only the opened posting carries — the body — to a card's job.
func (v indeedViewjob) apply(j *kit.Job) {
	j.Description = kit.SanitizeHTML(v.JobInfo.Model.Description)
	if c := strings.TrimSpace(v.JobInfo.Model.Header.Company); c != "" {
		j.Company = c
	}
	if j.SalaryPeriod == "" && v.Salary != nil {
		indeedSalary(j, v.Salary.Min, v.Salary.Max, v.Salary.Type, v.Salary.Currency)
	}
}
