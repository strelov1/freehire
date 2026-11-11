package sources

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// idealtraits adapts IdealTraits career sites (app.idealtraits.com), a hiring platform used
// almost entirely by US insurance agencies. Live-verified 2026-10-20.
//
// The board is the employer's numeric BUSINESS id, decimal ("1162292"). The platform's own
// career site is "/<slug>/<base64 of that id>/careers": the id is the identity and the slug is
// decoration — a wrong slug 301s to the canonical one, an unknown id answers 403 "Access
// denied". A posting link ("/career/<slug>/<posting id><source tag>") carries the slug and NOT
// the business id, so a board is only ever learned from a career-site URL; every posting page
// links its employer's career site, which is how internal/ingest/boardresolve reaches one from a
// pasted posting link.
//
// The listing is the career site's own pagination XHR: the same URL with
// "X-Requested-With: XMLHttpRequest" answers JSON — {"joblist": <the cards' HTML>, "jobcount":
// <the board's total>} — for any slug, without the redirect, and a page past the end answers an
// empty joblist rather than the 404 the plain HTML page gives. A board with no openings answers
// jobcount 0. Every board sampled (39 of them, 1-5 postings each) fit one page, so a second page
// has not been observed carrying cards; the walk therefore trusts jobcount rather than a page
// size, and fails the crawl when the pages it read do not add up to it.
//
// A posting page carries a schema.org JobPosting ld+json (title, HTML body, datePosted,
// employmentType, a single street address, baseSalary) and a "Remote Type" field the ld+json
// lacks. Its hiringOrganization.name is the URL slug ("Comma-Insurance"), not a display name,
// so the employer comes from the catalogue.
type idealtraits struct {
	http idealtraitsHTTP
}

// idealtraitsHTTP is the transport the adapter needs: the listing is JSON gated behind a header,
// the posting pages are HTML. Both go through one limiter (pacedIdealTraitsGetter), because both
// hit the one host every board shares.
type idealtraitsHTTP interface {
	HeaderJSONGetter
	HTMLGetter
}

// NewIdealTraits builds the IdealTraits adapter over the given HTTP client.
func NewIdealTraits(c idealtraitsHTTP) Source { return idealtraits{http: c} }

func (idealtraits) Provider() string { return "idealtraits" }

// fullBoardListing: list reads the board until it holds exactly the jobcount the platform
// reports, and fails the crawl on any page error or shortfall. See the fullBoardListing
// interface for the bar.
func (idealtraits) fullBoardListing() {}

func (idealtraits) emptyBoardListing() {}

const idealtraitsOrigin = "https://app.idealtraits.com"

// idealtraitsMaxPages is a safety ceiling on the listing walk, never a way to end it: reaching
// it before the board's jobcount is read fails the crawl.
const idealtraitsMaxPages = 50

// idealtraitsXHR is the header that makes the career site answer its listing as JSON.
var idealtraitsXHR = map[string]string{"X-Requested-With": "XMLHttpRequest"}

func (s idealtraits) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p idealtraitsPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew is the hydrating crawl: the listing proves a posting is live, so a page is fetched
// only for a posting the catalogue lacks. Every board shares one rate-limited host, and steady
// state is then one listing request per board plus the genuinely new postings.
func (s idealtraits) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p idealtraitsPosting) (Job, bool) {
		if seen(p.id) {
			// Identity and the card's title only: the pipeline refreshes liveness without
			// rewriting content, and the title is what the catalogue filter weighs on a refresh.
			return Job{ExternalID: p.id, URL: p.url, Title: p.title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// IdealTraitsJobCount reports how many open postings a board's career site lists, in one
// request — the listing's own total. It exists for cmd/harvest-boards, so a candidate board is
// validated by exactly the request the adapter's crawl starts with.
func IdealTraitsJobCount(ctx context.Context, c HeaderJSONGetter, board string) (int, error) {
	u, err := idealtraitsListURL(board, 1)
	if err != nil {
		return 0, err
	}
	var page idealtraitsPage
	if err := c.GetJSONWithHeaders(ctx, u, idealtraitsXHR, &page); err != nil {
		return 0, err
	}
	return page.JobCount, nil
}

// idealtraitsPage is one page of the career site's listing XHR.
type idealtraitsPage struct {
	JobList  string `json:"joblist"`
	JobCount int    `json:"jobcount"`
}

func (p *idealtraitsPage) UnmarshalJSON(raw []byte) error {
	var v struct {
		JobList  *string `json:"joblist"`
		JobCount *int    `json:"jobcount"`
	}
	if err := json.Unmarshal(raw, &v); err != nil {
		return err
	}
	if v.JobList == nil || v.JobCount == nil || *v.JobCount < 0 {
		return fmt.Errorf("idealtraits: listing requires joblist and a nonnegative jobcount")
	}
	if *v.JobCount == 0 && strings.TrimSpace(*v.JobList) != "" {
		return fmt.Errorf("idealtraits: empty listing carries unexpected card markup")
	}
	p.JobList, p.JobCount = *v.JobList, *v.JobCount
	return nil
}

// idealtraitsPosting is one listing card: the posting's id, its page, and the title it states.
type idealtraitsPosting struct {
	id, url, title string
}

// idealtraitsBoardID matches a board: the platform's decimal business id.
var idealtraitsBoardID = regexp.MustCompile(`^[1-9][0-9]*$`)

// idealtraitsListURL builds the listing XHR for a board. The path's first segment is the
// career site's slug, which the XHR ignores (any value answers the same board, unredirected),
// so a fixed one stands in for a name the catalogue does not hold.
func idealtraitsListURL(board string, page int) (string, error) {
	if !idealtraitsBoardID.MatchString(board) {
		return "", fmt.Errorf("idealtraits: board %q is not a numeric business id", board)
	}
	key := base64.StdEncoding.EncodeToString([]byte(board))
	return fmt.Sprintf("%s/company/%s/careers?page=%d", idealtraitsOrigin, key, page), nil
}

// list reads the board's listing until it holds every posting the platform says it has.
func (s idealtraits) list(ctx context.Context, board string) ([]idealtraitsPosting, error) {
	var out []idealtraitsPosting
	seen := map[string]bool{}
	total := -1
	for page := 1; page <= idealtraitsMaxPages; page++ {
		u, err := idealtraitsListURL(board, page)
		if err != nil {
			return nil, err
		}
		var p idealtraitsPage
		if err := s.http.GetJSONWithHeaders(ctx, u, idealtraitsXHR, &p); err != nil {
			return nil, fmt.Errorf("idealtraits: listing %s page %d: %w", board, page, err)
		}
		if total < 0 {
			total = p.JobCount
		} else if p.JobCount != total {
			return nil, fmt.Errorf("idealtraits: listing %s changed jobcount during pagination", board)
		}
		cards, err := idealtraitsCards(p.JobList)
		if err != nil {
			return nil, fmt.Errorf("idealtraits: listing %s page %d: %w", board, page, err)
		}
		added := 0
		for _, c := range cards {
			if !seen[c.id] {
				seen[c.id] = true
				out = append(out, c)
				added++
			}
		}
		if len(out) >= total || added == 0 {
			break
		}
	}
	if len(out) != total {
		return nil, fmt.Errorf("idealtraits: listing %s: read %d postings, the board reports %d", board, len(out), total)
	}
	return out, nil
}

// idealtraitsCardLink matches a card's posting page and captures the posting id. The id is
// followed by a letters-only source tag ("CPG" on the career site, "gog" in the page's own
// canonical link) that names where the visitor came from, not the posting.
var idealtraitsCardLink = regexp.MustCompile(`^https://app\.idealtraits\.com/career/[^/]+/(\d+)[A-Za-z]*$`)

// idealtraitsCards reads the listing cards out of a joblist fragment. A card is the element
// carrying data-landingpage; the fragment also holds a commented-out copy of each card's opening
// tag, which the parser keeps as a comment and the walk never sees.
func idealtraitsCards(fragment string) ([]idealtraitsPosting, error) {
	if strings.TrimSpace(fragment) == "" {
		return nil, nil
	}
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return nil, err
	}
	var out []idealtraitsPosting
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !HasClass(n, "opportunity-card") {
			return true
		}
		link := strings.TrimSpace(Attr(n, "data-landingpage"))
		if id := firstSubmatch(idealtraitsCardLink, link); id != "" {
			title := ""
			if h := firstByTag(n, "h5"); h != nil {
				title = strings.Join(strings.Fields(textContent(h)), " ")
			}
			out = append(out, idealtraitsPosting{id: id, url: link, title: title})
		}
		return false
	})
	return out, nil
}

// idealtraitsJobPosting is the subset of a posting page's schema.org JobPosting the adapter maps.
type idealtraitsJobPosting struct {
	Title          string       `json:"title"`
	Description    string       `json:"description"`
	DatePosted     string       `json:"datePosted"`
	EmploymentType string       `json:"employmentType"`
	JobLocation    schemaPlaces `json:"jobLocation"`
	BaseSalary     struct {
		Currency string `json:"currency"`
		Value    struct {
			MinValue string `json:"minValue"`
			MaxValue string `json:"maxValue"`
			UnitText string `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail fetches one posting page and maps it to a Job. A page the platform answers 404/410 for
// is gone and dropped; any other failure, or a page carrying no posting, is an unreadable marker,
// since the page is the posting's only source.
func (s idealtraits) detail(ctx context.Context, e CompanyEntry, p idealtraitsPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.id, p.url, e.Company), true
		}
		return Job{}, false
	}
	job, ok := parseIdealTraitsPosting(root, p, e.Company)
	if !ok {
		return unreadableDetail(p.id, p.url, e.Company), true
	}
	return job, true
}

// parseIdealTraitsPosting maps a posting page, reporting ok=false when it carries no usable
// JobPosting.
func parseIdealTraitsPosting(root *html.Node, p idealtraitsPosting, company string) (Job, bool) {
	var jp idealtraitsJobPosting
	if !ldJobPosting(root, &jp) {
		return Job{}, false
	}
	title := strings.Join(strings.Fields(jp.Title), " ")
	if title == "" {
		return Job{}, false
	}
	var loc string
	var countries []string
	if len(jp.JobLocation) > 0 {
		addr := jp.JobLocation[0].Address
		loc = addr.Location()
		if c := location.NormalizeCountry(strings.TrimSpace(addr.AddressCountry)); c != "" {
			countries = []string{c}
		}
	}
	workMode := ""
	if n := firstByID(root, "remote-type"); n != nil {
		workMode = idealtraitsWorkModes[strings.TrimSpace(textContent(n))]
	}
	job := Job{
		ExternalID:     p.id,
		URL:            p.url,
		Title:          title,
		Company:        company,
		Location:       loc,
		Description:    sanitizeHTML(idealtraitsBenefitLists(jp.Description)),
		WorkMode:       workMode,
		Remote:         workMode == "remote",
		PostedAt:       NotFuture(parseRFC3339(strings.TrimSpace(jp.DatePosted))),
		EmploymentType: schemaEmploymentType(jp.EmploymentType),
		Countries:      countries,
	}
	idealtraitsSalary(&job, jp)
	return job, true
}

// idealtraitsWorkModes maps the page's "Remote Type" field. "No" is what an office posting
// states, so it is onsite rather than unknown. Any other value stays unknown for the parser.
var idealtraitsWorkModes = map[string]string{
	"No":            "onsite",
	"Hybrid Remote": "hybrid",
	"Fully Remote":  "remote",
}

// idealtraitsBenefitRun matches a run of text the platform joined with "###" — the posting's
// benefits picklist, stored as one string ("Paid Time Off (PTO)###Health Insurance###").
var idealtraitsBenefitRun = regexp.MustCompile(`[^<>]*###[^<>]*`)

// idealtraitsBenefitLists turns each "###"-joined run in the body into the list it is.
func idealtraitsBenefitLists(body string) string {
	return idealtraitsBenefitRun.ReplaceAllStringFunc(body, func(run string) string {
		var b strings.Builder
		for _, item := range strings.Split(run, "###") {
			if item = strings.TrimSpace(item); item != "" {
				b.WriteString("<li>" + item + "</li>")
			}
		}
		if b.Len() == 0 {
			return ""
		}
		return "<ul>" + b.String() + "</ul>"
	})
}

// idealtraitsSalary takes the JobPosting's bounds, which the platform writes as decimal strings
// ("55000.00"). A bound that is absent or not positive is left unset.
func idealtraitsSalary(job *Job, jp idealtraitsJobPosting) {
	period := schemaSalaryPeriod(jp.BaseSalary.Value.UnitText)
	currency := strings.ToUpper(strings.TrimSpace(jp.BaseSalary.Currency))
	lo, hi := idealtraitsBound(jp.BaseSalary.Value.MinValue), idealtraitsBound(jp.BaseSalary.Value.MaxValue)
	if period == "" || len(currency) != 3 || (lo == nil && hi == nil) {
		return
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = lo, hi, currency, period
}

func idealtraitsBound(s string) *int {
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || v <= 0 {
		return nil
	}
	n := int(math.Round(v))
	return &n
}
