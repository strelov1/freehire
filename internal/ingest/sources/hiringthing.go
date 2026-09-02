package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// hiringthing adapts HiringThing career sites. The board is the full careers HOST, not the
// tenant slug — the platform is white-labelled, so the same application serves a tenant under
// the vendor's own domain (`skijapan.hiringthing.com`) and under each reseller's
// (`crown-shredding-llc.prismhr-hire.com`, `<tenant>.oasisrecruit.com`,
// `<tenant>.verahr-hiring.com`, `<tenant>.gnahiring.com`, and two dozen more). A slug alone
// therefore does not name a board: nothing in it says which domain answers for it, and the same
// slug can exist under two resellers. Every reseller domain serves the identical application,
// so one adapter covers all of them and no per-domain registry is needed.
//
// A board is listed on its site ROOT in full: HiringThing renders every open posting on one
// page (no pagination — the listing's own filter state declares showAll, and across 80 live
// boards the posting anchors matched the filter state's position count exactly), so enumeration
// costs one request and the permalinks it carries are the complete live set.
//
// A posting page carries the platform's own job record as JSON in the props of its
// ApplyButtonGroup React component: title, html_description, posted_at, location, and the
// structured location_info / remote / salary fields. The page also renders a schema.org
// JobPosting ld+json block, but that one states neither the remote flag nor the salary, so the
// record is what the adapter reads. (The record also carries the platform's application-form
// questions. Job.ApplyForm is for a form the LIST endpoint carries; this one is on the posting
// page, so it is left nil — mapping it would be a change to that contract, not to this adapter.)
//
// The description lives only on that page, so the adapter hydrates (HydratingSource): a detail
// request is spent on a posting the catalogue does not already have, never on re-listing one it
// has. The boards are small — a median of four postings, measured over 80 live ones — but there
// are thousands of them, and a posting page is ~40 KB against the listing's ~15 KB, so the
// difference is a run that costs a request per POSTING and one that costs a request per NEW
// posting.
type hiringthing struct {
	http HTMLGetter
}

// NewHiringThing builds the HiringThing adapter over the given HTTP client.
func NewHiringThing(c HTMLGetter) Source { return hiringthing{http: c} }

func (hiringthing) Provider() string { return "hiringthing" }

// Fetch is the list-only fallback HydratingSource specifies for a caller that cannot supply a
// seen set; it hydrates every posting. FetchNew is the path cmd/ingest drives.
func (s hiringthing) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew is the hydrating crawl: it enumerates the whole board from the site root but fetches
// a posting page only for an id the catalogue does not already have. A seen posting is emitted
// as a liveness refresh (identity only, no detail request, no content rewrite); an unseen one is
// hydrated.
func (s hiringthing) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	// base carries the scheme+host; the listing's relative "/job/<id>/<slug>" hrefs resolve
	// against it into fetchable absolute URLs.
	base, err := url.Parse(fmt.Sprintf("https://%s/", e.Board))
	if err != nil {
		return nil, fmt.Errorf("hiringthing: board %q: %w", e.Board, err)
	}
	root, err := s.http.GetHTML(ctx, base.String())
	if err != nil {
		return nil, fmt.Errorf("hiringthing: listing %s: %w", e.Board, err)
	}
	// Each unseen posting's record comes from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(htJobLinks(base, root), defaultDetailWorkers, func(l htLink) (Job, bool) {
		// Already ingested: refresh liveness by identity only. Re-upserting it content-less
		// would wipe the description and the facets derived from it, so the pipeline routes a
		// SeenRefresh to a liveness touch instead of a write.
		if seen(l.id) {
			return Job{ExternalID: l.id, URL: l.url, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, l)
	}), nil
}

// htLink is one posting the listing links: its native id and the URL to fetch it at.
type htLink struct {
	id  string
	url string
}

// htJobLinks returns the postings a listing page links, de-duplicated BY ID rather than by URL —
// the slug is not part of a posting's identity, so the same posting linked under two slugs is one
// posting, and keying the walk on the URL would buy its page twice and emit two jobs sharing one
// dedup key. First-seen order, first-seen URL.
//
// A link off the board's own host is dropped. On this platform the board IS a host, so a posting
// linked on another one is another board's: crawling it would file a second employer's postings
// under this board's company, and would send the crawl to a host the board file never named.
func htJobLinks(base *url.URL, root *html.Node) []htLink {
	var links []htLink
	seen := make(map[string]bool)
	for _, u := range jobLinks(base, root, func(href string) bool { return htJobID(href) != "" }) {
		parsed, err := url.Parse(u)
		if err != nil || parsed.Host != base.Host {
			continue
		}
		id := htJobID(u)
		if seen[id] {
			continue
		}
		seen[id] = true
		links = append(links, htLink{id: id, url: u})
	}
	return links
}

// detail fetches one posting page and maps its job record to a Job, returning ok=false when the
// page fetch fails or carries no record, so the caller skips just that posting.
func (s hiringthing) detail(ctx context.Context, e CompanyEntry, l htLink) (Job, bool) {
	root, err := s.http.GetHTML(ctx, l.url)
	if err != nil {
		return Job{}, false
	}
	rec, ok := htJobRecord(root)
	if !ok {
		return Job{}, false
	}

	salaryMin, salaryMax, currency, period := rec.salary()
	return Job{
		ExternalID: l.id,
		URL:        l.url,
		Title:      rec.Title,
		Company:    e.Company,
		// The record's location already reads "Remote - Windsor Locks, CT" for a remote
		// posting, so it is carried verbatim rather than rebuilt from location_info.
		Location:    rec.Location,
		Description: sanitizeHTML(rec.HTMLDescription),
		// remote is the platform's own structured flag (null on records predating it, which
		// decodes to false — the same "not stated" the flag itself means). The location text is
		// not consulted: the platform derives its "Remote - " prefix from this very flag, so
		// reading it back would add no signal and would false-positive on a place name.
		Remote:         rec.Remote,
		WorkMode:       workModeFromRemote(rec.Remote),
		Countries:      countryFromCode(rec.LocationInfo.Country),
		PostedAt:       parseRFC3339(rec.PostedAt),
		SalaryMin:      salaryMin,
		SalaryMax:      salaryMax,
		SalaryCurrency: currency,
		SalaryPeriod:   period,
	}, true
}

// htJobIDPattern captures the native posting id from a posting permalink's /job/<id>/<slug>
// path. The slug is not part of the id: the platform serves the posting under any slug. Nothing
// is anchored after the id, so a permalink carrying a query or fragment still resolves — and the
// harvest prober's own liveness pattern matches the same shape, so a board it accepts as live is
// one this adapter can enumerate.
var htJobIDPattern = regexp.MustCompile(`^/job/(\d+)(?:/|$)`)

// htJobID extracts the native numeric posting id from a posting page URL, or "" when the URL is
// not a posting permalink. Only the PATH is matched, so absolute and relative hrefs both resolve
// and an id that appears in some other link's query string ("/privacy?next=/job/1052705") is not
// mistaken for a posting.
func htJobID(u string) string {
	p := u
	if parsed, err := url.Parse(u); err == nil {
		p = parsed.Path
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return firstSubmatch(htJobIDPattern, p)
}

// htReactClass is the React component whose props carry a posting's job record. The listing and
// the posting page both mount several components this way (a salary widget per card, an email
// subscription form, a cookie banner), and their order varies, so the record is found by
// component name rather than by taking the first data-react-props on the page.
const htReactClass = "HiringThing.Components.ApplyButtonGroup"

// htPosting is the platform's own job record, as the posting page inlines it. Only the fields
// the adapter maps are decoded.
type htPosting struct {
	Title           string  `json:"title"`
	HTMLDescription string  `json:"html_description"`
	PostedAt        string  `json:"posted_at"`
	Location        string  `json:"location"`
	LocationInfo    htPlace `json:"location_info"`
	Remote          bool    `json:"remote"`
	MinSalary       htMoney `json:"min_salary"`
	MaxSalary       htMoney `json:"max_salary"`
	PayFrequency    string  `json:"pay_frequency"`
}

// htPlace is the record's structured location. Only the country is read: it is already an
// ISO alpha-2 code, while the city and state are carried in the free-text location.
type htPlace struct {
	Country string `json:"country"`
}

// htMoney is one bound of the record's salary range. amount is a decimal STRING ("13.00",
// "90000.00"); an unset bound is an empty object rather than a null.
type htMoney struct {
	Amount   string `json:"amount"`
	Currency string `json:"currency"`
}

// salary maps the record's compensation onto freehire's structured salary fields, returning all
// four empty when the record states no bound or states one in a period freehire has no value
// for. pay_frequency is populated even on a posting with no salary at all, so the bounds decide
// whether there is anything to state.
func (p htPosting) salary() (salaryMin, salaryMax *int, currency, period string) {
	lo, hi := p.MinSalary.amount(), p.MaxSalary.amount()
	if lo == nil && hi == nil {
		return nil, nil, "", ""
	}
	// Weekly is the one frequency the platform emits that freehire's vocabulary has no value
	// for. Dropping the whole range leaves the figure to the enrichment pass, which reads it
	// from the description; keeping the bounds under another period would restate it wrongly.
	period = htSalaryPeriod(p.PayFrequency)
	if period == "" {
		return nil, nil, "", ""
	}
	currency = p.MinSalary.Currency
	if currency == "" {
		currency = p.MaxSalary.Currency
	}
	return lo, hi, currency, period
}

// amount parses one salary bound's decimal string into freehire's integer bound, reporting
// absent (nil) for an unset bound or an unparseable figure.
func (m htMoney) amount() *int {
	v, err := strconv.ParseFloat(strings.TrimSpace(m.Amount), 64)
	if err != nil {
		return nil
	}
	return roundSalaryPart(v)
}

// htSalaryPeriod maps the platform's pay-frequency vocabulary onto freehire's salary_period
// values, returning "" for one it has no value for ("weekly") or for an absent frequency.
func htSalaryPeriod(freq string) string {
	switch strings.ToLower(strings.TrimSpace(freq)) {
	case "hourly":
		return "hour"
	case "monthly":
		return "month"
	case "annually":
		return "year"
	default:
		return ""
	}
}

// htJobRecord decodes the job record from the posting page's ApplyButtonGroup props, returning
// ok=false when the page mounts no such component or its props do not carry a record.
func htJobRecord(root *html.Node) (htPosting, bool) {
	props := htRecordProps(root)
	if props == "" {
		return htPosting{}, false
	}
	var envelope struct {
		JobObj struct {
			Table htPosting `json:"table"`
		} `json:"jobObj"`
	}
	if err := json.Unmarshal([]byte(props), &envelope); err != nil {
		return htPosting{}, false
	}
	return envelope.JobObj.Table, true
}

// htRecordProps returns the data-react-props JSON of the element mounting the record-bearing
// React component, or "" when the page mounts none.
func htRecordProps(root *html.Node) string {
	var props string
	walk(root, func(n *html.Node) bool {
		if props != "" {
			return false
		}
		if n.Type == html.ElementNode && attr(n, "data-react-class") == htReactClass {
			props = attr(n, "data-react-props")
			return false
		}
		return true
	})
	return props
}
