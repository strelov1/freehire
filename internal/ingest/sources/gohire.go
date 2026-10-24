package sources

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// gohire adapts GoHire career sites (jobs.gohire.io). A board is the site's path segment,
// "<name slug>-<8-character code>" (jobs.gohire.io/dexerto-de5jlhjo), stored as the platform
// spells it. Only the code addresses the site: any other slug in front of it 301s onto the
// canonical one, and so does the bare code (measured 2026-10-24), so two spellings differing in
// the slug are one crawl target and boardIdentity folds them onto the code. An older numeric
// form (dexerto-19265) still appears in crawled links and answers 404 — it is not a board.
//
// The listing is server-rendered PHP, ten cards a page under ?page=N, and states its own extent
// ("Page 1 of 3, Total 21 jobs"); a page past the end renders no card. A card carries the title
// and a location line but no body, which only the posting page has, as a schema.org JobPosting
// with the remote flag (jobLocationType), the countries a remote posting is open to, the
// employment type, the posted date and a structured salary. So this is a HydratingSource.
//
// The platform meters a request quota (see gohireRequestInterval), so every request goes
// through gohirePacedGetter and the first refusal stops the run's requests.
type gohire struct {
	http HTMLGetter
	// refused latches the first 429 of a run. The refusal is about the IP and outlasts the run,
	// so every further request would be refused alike while still spending the quota — and the
	// shared client retries each one. Once set, the rest of the run asks for nothing.
	refused *atomic.Bool
}

// NewGoHire builds the GoHire adapter over the given HTTP client. The caller paces it (see
// gohirePacedGetter); the registry does.
func NewGoHire(c HTMLGetter) Source { return gohire{http: c, refused: new(atomic.Bool)} }

// errGoHireRefused stands in for every request skipped after the run's first refusal. It is a
// 429 so that a skipped posting page reads exactly like a refused one: unreadable, not gone.
var errGoHireRefused = &StatusError{Method: http.MethodGet, URL: gohireBase, Code: http.StatusTooManyRequests}

// get fetches one page unless the run has already been refused.
func (g gohire) get(ctx context.Context, u string) (*html.Node, error) {
	if g.refused.Load() {
		return nil, errGoHireRefused
	}
	root, err := g.http.GetHTML(ctx, u)
	if err != nil && isRateLimited(err) && !g.refused.Swap(true) {
		log.Printf("gohire: refused (%v); asking for nothing more this run", err)
	}
	return root, err
}

func (gohire) Provider() string { return "gohire" }

const gohireBase = "https://jobs.gohire.io/"

// gohireMaxPages is a safety ceiling on the ?page=N walk. The largest of 108 sites measured
// held 21 postings (3 pages); the walk normally ends on the listing's own page count.
const gohireMaxPages = 100

// gohireBoardPattern is the site segment: a slug ending in the 8-character site code.
var gohireBoardPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*-[a-z0-9]{8}$`)

// gohireCard is one listing card.
type gohireCard struct {
	ID, URL, Title, Location string
}

// Fetch reads a site's listing and every posting's page.
func (g gohire) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	cards, err := g.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(cards, defaultDetailWorkers, func(c gohireCard) (Job, bool) {
		return g.detail(ctx, e, c)
	}), nil
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold; a
// seen posting is re-listed as a SeenRefresh, the listing itself proving it is still open.
func (g gohire) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	cards, err := g.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(cards, defaultDetailWorkers, func(c gohireCard) (Job, bool) {
		if seen(c.ID) {
			return Job{ExternalID: c.ID, URL: c.URL, Title: c.Title, Company: e.Company, SeenRefresh: true}, true
		}
		return g.detail(ctx, e, c)
	}), nil
}

// list walks the listing pages. ANY page failing fails the board — an exception to the
// repository-wide "a later page ends the walk with what was gathered" rule (crawlPagedLinks),
// because here a later page failing is the expected shape of the quota running out mid-board
// (see gohireRequestInterval): salvaging would leave every posting past that page unseen run
// after run, and the unseen sweep would close them while they are still live. The walk ends
// on the page count the listing states, or on a page that adds no new card.
func (g gohire) list(ctx context.Context, board string) ([]gohireCard, error) {
	if !gohireBoardPattern.MatchString(board) {
		return nil, fmt.Errorf("gohire: board %q is not a <slug>-<code> site segment", board)
	}
	var cards []gohireCard
	seen := map[string]bool{}
	for page := 1; page <= gohireMaxPages; page++ {
		root, err := g.get(ctx, fmt.Sprintf("%s%s/?page=%d", gohireBase, board, page))
		if err != nil {
			return nil, fmt.Errorf("gohire: list site %s page %d: %w", board, page, err)
		}
		added := 0
		for _, c := range gohireCards(root) {
			if !seen[c.ID] {
				seen[c.ID] = true
				cards = append(cards, c)
				added++
			}
		}
		if added == 0 || page >= gohirePageCount(root) {
			break
		}
	}
	return cards, nil
}

// gohireJobPath matches a posting link's path and captures the posting id, the number its slug
// ends with (/dexerto-de5jlhjo/marketing-manager-304991/) — the same id the page's JobPosting
// states as its identifier.
var gohireJobPath = regexp.MustCompile(`^/[^/]+/[^/]*-(\d+)/?$`)

// gohireCards reads every <a class="gohire-job"> card on a listing page.
func gohireCards(root *html.Node) []gohireCard {
	var cards []gohireCard
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" || !HasClass(n, "gohire-job") {
			return true
		}
		u, err := url.Parse(Attr(n, "href"))
		if err != nil {
			return false
		}
		id := firstSubmatch(gohireJobPath, u.Path)
		if id == "" {
			return false
		}
		c := gohireCard{ID: id, URL: "https://jobs.gohire.io" + u.Path}
		if t := firstByClass(n, "job-title"); t != nil {
			c.Title = textContent(t)
		}
		if l := firstByClass(n, "careers-location"); l != nil {
			c.Location = textContent(l)
		}
		cards = append(cards, c)
		return false
	})
	return cards
}

// gohirePageTotal reads "Page 1 of <strong>3</strong>" from the pagination footer.
var gohirePageTotal = regexp.MustCompile(`Page\s+\d+\s+of\s+(\d+)`)

// gohirePageCount is the number of listing pages the page states, or 1 when it states none —
// a site with a single page of postings renders no pagination footer.
func gohirePageCount(root *html.Node) int {
	footer := firstByClass(root, "gohire-job-pagination-results")
	if footer == nil {
		return 1
	}
	n, err := strconv.Atoi(firstSubmatch(gohirePageTotal, strings.Join(strings.Fields(textContent(footer)), " ")))
	if err != nil {
		return 1
	}
	return n
}

// gohirePosting is the posting page's schema.org JobPosting.
type gohirePosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation                   schemaPlaces     `json:"jobLocation"`
	ApplicantLocationRequirements schemaNamedAreas `json:"applicantLocationRequirements"`
	BaseSalary                    *struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string  `json:"unitText"`
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail fetches one posting page and maps its JobPosting, with the card, to a Job. A 404 is
// the platform's own "no such posting" and drops it; any other failure, or a page carrying no
// JobPosting, is an unreadableDetail marker — the page is the only source of the body.
func (g gohire) detail(ctx context.Context, e CompanyEntry, c gohireCard) (Job, bool) {
	root, err := g.get(ctx, c.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(c.ID, c.URL, e.Company), true
		}
		return Job{}, false
	}
	var p gohirePosting
	if !ldJobPosting(root, &p) || strings.TrimSpace(p.Description) == "" {
		return unreadableDetail(c.ID, c.URL, e.Company), true
	}

	remote := strings.EqualFold(p.JobLocationType, "TELECOMMUTE")
	loc := c.Location
	if loc == "" && len(p.JobLocation) > 0 {
		loc = p.JobLocation[0].Address.Location()
	}
	j := Job{
		ExternalID:     c.ID,
		URL:            c.URL,
		Title:          firstNonEmpty(strings.TrimSpace(p.Title), c.Title),
		Company:        firstNonEmpty(e.Company, p.HiringOrganization.Name),
		Location:       loc,
		Description:    sanitizeHTML(p.Description),
		Remote:         remote || isRemote(loc),
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: schemaEmploymentType(p.EmploymentType),
		PostedAt:       parseRFC3339OrDate(p.DatePosted),
		Countries:      gohireCountries(p, remote),
	}
	if s := p.BaseSalary; s != nil {
		if period := schemaSalaryPeriod(s.Value.UnitText); period != "" && s.Currency != "" {
			j.SalaryMin, j.SalaryMax = roundSalaryPart(s.Value.MinValue), roundSalaryPart(s.Value.MaxValue)
			if j.SalaryMin != nil || j.SalaryMax != nil {
				j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(s.Currency), period
			}
		}
	}
	return j, true
}

// gohireCountries reads the structured countries: for a remote posting the ones it is open to
// (applicantLocationRequirements), otherwise the workplace address's country. GoHire writes
// both as country NAMES ("United Kingdom"), which NormalizeCountry resolves; a name it cannot
// place is left to the location dictionary.
func gohireCountries(p gohirePosting, remote bool) []string {
	var names []string
	if remote {
		names = p.ApplicantLocationRequirements.Names()
	}
	if len(names) == 0 {
		for _, pl := range p.JobLocation {
			names = append(names, pl.Address.AddressCountry)
		}
	}
	var out []string
	for _, n := range names {
		if c := location.NormalizeCountry(n); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// gohireSiteCode folds a board onto the site code that alone addresses it, so
// "dexerto-de5jlhjo" and "dexerto-media-de5jlhjo" are recognised as one crawl target.
func gohireSiteCode(board string) string {
	if i := strings.LastIndex(board, "-"); i >= 0 {
		return board[i+1:]
	}
	return board
}

// gohireRequestInterval paces every GoHire request in a run. GoHire meters a request QUOTA,
// not a rate: measured 2026-10-24 from the crawl host, one request every ~1.3s was served 400
// times in a row and refused with 429 from the 401st, and a residential IP refused at ~270
// requests was still refused 30 minutes later. So the pace is set by the quota, not by the
// origin's speed: one request every 10s is 360 an hour, under 400 even if the window is an
// hour, and a 3000s run spends at most ~300 — the fleet's ~110 listing pages plus the new
// postings' pages, with a first crawl's backlog carried over a few runs (each board persists
// as it finishes). If the quota is longer than an hour, the refusal latch (gohire.refused)
// is what keeps a run from spending the rest of it on retries.
const (
	gohireRequestInterval = 10 * time.Second
	gohireRequestBurst    = 1
)

// gohirePacedGetter is the limiter the registry builds GoHire over.
func gohirePacedGetter(c HTMLGetter) HTMLGetter {
	return pacedHTMLGetter(c, gohireRequestInterval, gohireRequestBurst)
}
