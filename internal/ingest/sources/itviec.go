package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"log"
	"math"
	"net/url"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// itviec adapts ITviec (itviec.com), Vietnam's IT-only job board. The whole catalogue is one
// server-rendered listing (/it-jobs, ~20 cards a page), so the provider is boardless. Each card
// carries the posting's slug and its own key; the body, employer, place and salary come from the
// posting page's schema.org JobPosting, so this is a HydratingSource that buys a page only for a
// posting the catalogue lacks.
//
// The walk follows the ordinary "which page failed decides" rule and does NOT claim
// fullBoardListing or fullCatalog: the end of the listing (a page with no cards) has not been
// proven against the stated total. See the ITviec traps in AGENTS.md.
type itviec struct {
	http HTMLGetter
}

const (
	itviecBase     = "https://itviec.com"
	itviecListURL  = itviecBase + "/it-jobs"
	itviecMaxPages = 100 // a ceiling three times the measured catalogue, never the expected end
)

// ITviec has refused back-to-back sequential listing sweeps with 429. The budget is unmeasured,
// so every request, listing and detail alike, is held to one a second.
const (
	itviecRequestInterval = time.Second
	itviecRequestBurst    = 2
)

func NewITviec(c HTMLGetter) Source { return itviec{http: c} }

func (itviec) Provider() string { return "itviec" }
func (itviec) Boardless()       {}
func (itviec) Aggregator()      {}

// itviecListing is what a listing card states about a posting: its key (the external id) and the
// slug its page lives under.
type itviecListing struct {
	ID  string
	URL string
}

func (s itviec) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, CompanyEntry{}, func(string) bool { return false })
}

func (s itviec) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	list, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, list, seen)
}

// list walks the listing to its first page without cards. The cards' own key attribute is what
// counts: every page also links two "company spotlight" postings that are not cards (and repeat
// past the last page), so reading posting LINKS would never see the listing end.
func (s itviec) list(ctx context.Context) ([]itviecListing, error) {
	keys := map[string]string{} // posting URL → card key
	urls, err := crawlPagedLinks(ctx, s.http, itviecMaxPages, itviecPageURL, func(root *xhtml.Node) []string {
		var out []string
		for _, l := range parseITviecCards(root) {
			keys[l.URL] = l.ID
			out = append(out, l.URL)
		}
		return out
	})
	if err != nil {
		return nil, fmt.Errorf("itviec: listing: %w", err)
	}
	// The platform always lists hundreds of postings, so a first page without a single card is a
	// markup change or a challenge page, never an empty market.
	if len(urls) == 0 {
		return nil, fmt.Errorf("itviec: listing page 1 carries no job cards")
	}
	out := make([]itviecListing, 0, len(urls))
	for _, u := range urls {
		out = append(out, itviecListing{ID: keys[u], URL: u})
	}
	return out, nil
}

func itviecPageURL(page int) string {
	if page == 1 {
		return itviecListURL
	}
	return itviecListURL + "?page=" + strconv.Itoa(page)
}

// parseITviecCards reads every job card on a listing page: an element carrying both the card's
// key (data-job-key) and its slug. A card missing either is skipped.
func parseITviecCards(root *xhtml.Node) []itviecListing {
	var out []itviecListing
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode {
			return true
		}
		slug := strings.TrimSpace(Attr(n, "data-search--job-selection-job-slug-value"))
		key := strings.TrimSpace(Attr(n, "data-job-key"))
		if slug == "" || key == "" {
			return true
		}
		out = append(out, itviecListing{ID: key, URL: itviecListURL + "/" + url.PathEscape(slug)})
		return false
	})
	return out
}

// hydrate buys a posting page only for a posting the catalogue lacks. A page that could not be
// read defers that posting to a later run rather than storing it body-less (a stored row is seen
// and would never be hydrated again). Once the edge refuses one request (403/429) the rest of the
// run's hydration is deferred too: the refusal is about the IP, and every further request would
// only prolong it.
func (s itviec) hydrate(ctx context.Context, list []itviecListing, seen func(string) bool) ([]Job, error) {
	var refused atomic.Bool
	var wanted, read atomic.Int64
	jobs := fetchDetails(list, defaultDetailWorkers, func(l itviecListing) (Job, bool) {
		if seen(l.ID) {
			return Job{ExternalID: l.ID, URL: l.URL, SeenRefresh: true}, true
		}
		wanted.Add(1)
		if refused.Load() {
			return Job{}, false
		}
		root, err := s.http.GetHTML(ctx, l.URL)
		if err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("itviec: posting pages refused (%v); deferring the rest of this run's hydration", err)
			}
			if detailUnreadable(err) {
				log.Printf("itviec: detail %s failed; deferring: %v", l.URL, err)
			}
			return Job{}, false
		}
		job, ok := parseITviecPosting(root, l)
		if !ok {
			log.Printf("itviec: detail %s carries no usable JobPosting; deferring", l.URL)
			return Job{}, false
		}
		read.Add(1)
		return job, true
	})
	// A crawl that listed new postings and read none of them is broken, not quiet.
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("itviec: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// itviecPosting is the subset of the posting page's schema.org JobPosting the adapter maps.
type itviecPosting struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	Skills             string          `json:"skills"`
	JobLocation        json.RawMessage `json:"jobLocation"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	BaseSalary struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue *float64 `json:"minValue"`
			MaxValue *float64 `json:"maxValue"`
			UnitText string   `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

type itviecPlace struct {
	Address struct {
		AddressLocality string `json:"addressLocality"`
		AddressRegion   string `json:"addressRegion"`
		AddressCountry  string `json:"addressCountry"`
	} `json:"address"`
}

func parseITviecPosting(root *xhtml.Node, l itviecListing) (Job, bool) {
	var p itviecPosting
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	// The page heading keeps the title's punctuation ("AI Software Engineer (Python/Go/C/C++ &
	// AI Agents)"); the JobPosting strips it ("... Python/Go/C/C++  AI Agents").
	title := ""
	if h1 := firstByTag(root, "h1"); h1 != nil {
		title = strings.Join(strings.Fields(textContent(h1)), " ")
	}
	if title == "" {
		title = strings.Join(strings.Fields(p.Title), " ")
	}
	company := strings.TrimSpace(p.HiringOrganization.Name)
	description := sanitizeHTML(itviecDescription(p.Description))
	if title == "" || company == "" || strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	places := itviecPlaces(p.JobLocation)
	workMode := itviecWorkMode(root)
	job := Job{
		ExternalID:  l.ID,
		URL:         l.URL,
		Title:       title,
		Company:     company,
		Location:    itviecLocation(places),
		Description: description,
		PostedAt:    NotFuture(parseDate(strings.TrimSpace(p.DatePosted))),
		WorkMode:    workMode,
		Remote:      workMode == "remote",
		Countries:   itviecCountries(places),
		Skills:      skilltag.Canonicalize(strings.Split(p.Skills, ",")),
		IsTechHint:  true, // ITviec lists IT postings only
	}
	itviecSalary(&job, p)
	return job, true
}

// itviecDescriptionHeadings are the fixed section headings ITviec writes into the JobPosting
// description as bare text lines between the employer's HTML sections.
var itviecDescriptionHeadings = map[string]bool{
	"Top 3 Reasons To Join Us":     true,
	"The Job":                      true,
	"Your Skills and Experience":   true,
	"Why You'll Love Working Here": true,
}

// itviecDescription rebuilds the JobPosting description as HTML. It is newline-separated lines,
// each either a fixed section heading, a bare text line (the "top 3 reasons"), or a section the
// employer authored as HTML.
func itviecDescription(raw string) string {
	var b strings.Builder
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		switch {
		case line == "":
		case itviecDescriptionHeadings[line]:
			b.WriteString("<h3>" + html.EscapeString(line) + "</h3>")
		case strings.HasPrefix(line, "<"):
			b.WriteString(line)
		default:
			b.WriteString("<p>" + html.EscapeString(line) + "</p>")
		}
	}
	return b.String()
}

// itviecPlaces decodes jobLocation, which schema.org allows as one Place or a list of them.
func itviecPlaces(raw json.RawMessage) []itviecPlace {
	var many []itviecPlace
	if json.Unmarshal(raw, &many) == nil {
		return many
	}
	var one itviecPlace
	if json.Unmarshal(raw, &one) == nil {
		return []itviecPlace{one}
	}
	return nil
}

// itviecLocation names each distinct city (addressRegion: "Hà Nội", "Hồ Chí Minh"), then the
// country. addressLocality is "Not Available" on the live pages and is read only when the region
// is empty.
func itviecLocation(places []itviecPlace) string {
	var parts []string
	seen := map[string]bool{}
	for _, pl := range places {
		city := strings.TrimSpace(pl.Address.AddressRegion)
		if city == "" && !strings.EqualFold(strings.TrimSpace(pl.Address.AddressLocality), "Not Available") {
			city = strings.TrimSpace(pl.Address.AddressLocality)
		}
		if city != "" && !seen[city] {
			seen[city] = true
			parts = append(parts, city)
		}
	}
	if countries := itviecCountries(places); len(countries) == 1 && countries[0] == "vn" {
		parts = append(parts, "Vietnam")
	}
	return strings.Join(parts, ", ")
}

func itviecCountries(places []itviecPlace) []string {
	var out []string
	seen := map[string]bool{}
	for _, pl := range places {
		if c := location.NormalizeCountry(pl.Address.AddressCountry); c != "" && !seen[c] {
			seen[c] = true
			out = append(out, c)
		}
	}
	return out
}

// itviecWorkModes maps the posting header's work-arrangement badge, the platform's own picklist.
var itviecWorkModes = map[string]string{
	"At office": "onsite",
	"Hybrid":    "hybrid",
	"Remote":    "remote",
}

// itviecWorkMode reads the badge from the posting header (the preview-header-item row beside the
// address and the "Posted" age). Two different badges state nothing, so neither is taken.
func itviecWorkMode(root *xhtml.Node) string {
	modes := map[string]bool{}
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || !HasClass(n, "preview-header-item") {
			return true
		}
		if mode, ok := itviecWorkModes[strings.Join(strings.Fields(textContent(n)), " ")]; ok {
			modes[mode] = true
		}
		return false
	})
	if len(modes) != 1 {
		return ""
	}
	for mode := range modes {
		return mode
	}
	return ""
}

var itviecSalaryPeriods = map[string]string{
	"HOUR":  "hour",
	"DAY":   "day",
	"MONTH": "month",
	"YEAR":  "year",
}

// itviecSalary takes only the JobPosting's numeric bounds. A posting hiding its pay states a
// phrase instead ("You'll love it") and no bound, and yields no salary.
func itviecSalary(job *Job, p itviecPosting) {
	period := itviecSalaryPeriods[strings.ToUpper(strings.TrimSpace(p.BaseSalary.Value.UnitText))]
	currency := strings.ToUpper(strings.TrimSpace(p.BaseSalary.Currency))
	lo, hi := itviecBound(p.BaseSalary.Value.MinValue), itviecBound(p.BaseSalary.Value.MaxValue)
	if period == "" || len(currency) != 3 || (lo == nil && hi == nil) {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}

func itviecBound(v *float64) *int {
	if v == nil || *v <= 0 {
		return nil
	}
	n := int(math.Round(*v))
	return &n
}
