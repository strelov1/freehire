package sources

import (
	"context"
	"encoding/xml"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// coveto adapts coveto (coveto.de), a German ATS whose hosted career sites live at
// https://<board>.coveto.de, the board being the customer number spelled "k<digits>"
// (k60360). The tenant label is the customer number, never a name, so a careers page that
// names the employer is the only place the name appears.
//
// Every tenant publishes its whole board as one RSS document, /public/rss/, one <item> per
// open posting (title, link, publication date, and a "job-id-<n>" guid), unpaginated — the same
// postings, measured on a 71-posting board, that its paged /public/jobs/ listing walks — and not
// capped on the largest tenants (k61491: 4,375 items, 175 listing pages of 25). An item
// carries no body: that is on the posting page, as a schema.org JobPosting with the employer
// (a tenant can post for a subsidiary), the workplace address, the employment type, a salary
// and — rarely — jobLocationType TELECOMMUTE. So this is a HydratingSource.
//
// A white-labelled tenant's feed links its postings on the tenant's own domain
// (jobs.rocket-match.de for k43552); the link is the posting's address and is fetched as given.
// 131 of 850 live tenants do, mostly recruitment agencies, holding a third of the postings.
//
// A switched-off tenant answers its RSS address with an HTML page titled "Diese Seite ist
// nicht mehr aktiv." under HTTP 200 (ErrBoardGone); a tenant that never enabled the feed
// answers "Diese Funktion wurde nicht aktiviert.", which is NOT gone — its postings are live,
// this adapter just cannot list them — and stays an ordinary failure.
type coveto struct {
	http covetoGetter
}

// covetoGetter is the transport the adapter needs: the RSS feed as XML, the same address as
// text when it does not decode (to read which notice the tenant serves instead), and each
// posting's page as HTML.
type covetoGetter interface {
	XMLGetter
	TextGetter
	HTMLGetter
}

// NewCoveto builds the coveto adapter over the given HTTP client.
func NewCoveto(c covetoGetter) Source { return coveto{http: c} }

func (coveto) Provider() string { return "coveto" }

// fullBoardListing: the RSS feed is one unpaginated document carrying every open posting.
func (coveto) fullBoardListing() {}

// covetoBoard is the shape of a tenant label: the customer number.
var covetoBoard = regexp.MustCompile(`^k[0-9]{3,7}$`)

// covetoGoneNotice is the switched-off tenant's page, served in place of the feed.
const covetoGoneNotice = "Diese Seite ist nicht mehr aktiv"

// covetoFeed is the RSS document. Items is a pointer so a feed without a <channel> is told
// apart from a board with no openings, which the platform serves as a channel with no item.
type covetoFeed struct {
	XMLName xml.Name `xml:"rss"`
	Channel *struct {
		Title string       `xml:"title"`
		Items []covetoItem `xml:"item"`
	} `xml:"channel"`
}

type covetoItem struct {
	Title       string `xml:"title"`
	Description string `xml:"description"`
	Link        string `xml:"link"`
	PubDate     string `xml:"pubDate"`
	GUID        string `xml:"guid"`
}

// covetoGUID captures the posting id from the guid (https://k60360.coveto.de/job-id-1934.html),
// covetoLinkID the same id from the end of the posting link (…-sachsen-1934.html).
var (
	covetoGUID   = regexp.MustCompile(`/job-id-([0-9]+)\.html$`)
	covetoLinkID = regexp.MustCompile(`-([0-9]+)\.html$`)
)

// covetoOpenApplication matches the feed title of an open application ("Initiativbewerbung",
// "Initiativ-Bewerbung Berlin"), the one item a tenant's feed carries that is not a posting.
var covetoOpenApplication = regexp.MustCompile(`(?i)initiativ`)

// covetoPosting is one feed item, reduced to what the posting page is fetched for.
type covetoPosting struct {
	ID, URL, Title, PubDate string
}

// Fetch reads the feed and every posting's page.
func (s coveto) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	posts, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(posts, defaultDetailWorkers, func(p covetoPosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold; a
// seen posting is re-listed as a SeenRefresh, the feed itself proving it is still open.
func (s coveto) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	posts, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(posts, defaultDetailWorkers, func(p covetoPosting) (Job, bool) {
		if seen(p.ID) {
			return Job{ExternalID: p.ID, URL: p.URL, Title: p.Title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, p)
	}), nil
}

// list reads the tenant's RSS feed.
func (s coveto) list(ctx context.Context, board string) ([]covetoPosting, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !covetoBoard.MatchString(board) {
		return nil, fmt.Errorf("coveto: board %q is not a k<number> customer label", board)
	}
	feedURL := "https://" + board + ".coveto.de/public/rss/"
	var feed covetoFeed
	if err := s.http.GetXML(ctx, feedURL, &feed); err != nil {
		// A switched-off tenant serves an HTML notice where the feed was; the decode fails on
		// it, and the same address read as text says which notice it was.
		if body, terr := s.http.GetText(ctx, feedURL); terr == nil && strings.Contains(body, covetoGoneNotice) {
			return nil, fmt.Errorf("coveto: board %s: %w", board, ErrBoardGone)
		}
		return nil, fmt.Errorf("coveto: board %s feed: %w", board, err)
	}
	if feed.Channel == nil {
		return nil, fmt.Errorf("coveto: board %s feed carries no channel", board)
	}
	var posts []covetoPosting
	seen := map[string]bool{}
	for _, it := range feed.Channel.Items {
		link := strings.TrimSpace(it.Link)
		id := firstNonEmpty(firstSubmatch(covetoGUID, strings.TrimSpace(it.GUID)), firstSubmatch(covetoLinkID, link))
		if id == "" || link == "" || seen[id] {
			continue // no id → nothing a close could reach; no link → no page to read
		}
		seen[id] = true
		posts = append(posts, covetoPosting{ID: id, URL: link, Title: strings.TrimSpace(it.Title), PubDate: strings.TrimSpace(it.PubDate)})
	}
	return posts, nil
}

// covetoJobPosting is the posting page's schema.org JobPosting.
type covetoJobPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	EmploymentType     string `json:"employmentType"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
	BaseSalary  *struct {
		Currency string `json:"currency"`
		Value    struct {
			UnitText string  `json:"unitText"`
			MinValue float64 `json:"minValue"`
			MaxValue float64 `json:"maxValue"`
		} `json:"value"`
	} `json:"baseSalary"`
}

// detail fetches one posting page and maps its JobPosting, with the feed item, to a Job. A 404
// is the platform's own "no such posting" and drops it; any other failure, or a page carrying
// no JobPosting body, is an unreadableDetail marker — the page is the only source of the body.
func (s coveto) detail(ctx context.Context, e CompanyEntry, p covetoPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.ID, p.URL, e.Company), true
		}
		return Job{}, false
	}
	var jp covetoJobPosting
	if !ldJobPosting(root, &jp) || strings.TrimSpace(jp.Description) == "" {
		// coveto withholds the JobPosting from an open application's page — 40 of 40 sampled
		// across 158 tenants that list one — since it is not a posting. Its feed title says
		// what it is, and only then is the missing block an answer rather than a page this
		// crawl could not read: a marker for it would recur every run (a dropped item is never
		// stored, so never seen) and hold the board's unseen sweep shut.
		if covetoOpenApplication.MatchString(p.Title) {
			return Job{}, false
		}
		return unreadableDetail(p.ID, p.URL, e.Company), true
	}
	// TELECOMMUTE is the only work-arrangement statement the page makes, and it is a remote one:
	// measured 2026-10-25, all 17 sampled "(Remote) …" titles and the "100% Home Office" ones
	// carry it, while "Homeoffice möglich" postings and the 85 postings of a random sample do
	// not. Its absence says nothing, so it never yields "onsite".
	remote := strings.EqualFold(strings.TrimSpace(jp.JobLocationType), "TELECOMMUTE")
	j := Job{
		ExternalID:     p.ID,
		URL:            p.URL,
		Title:          firstNonEmpty(strings.TrimSpace(jp.Title), p.Title),
		Company:        firstNonEmpty(strings.TrimSpace(jp.HiringOrganization.Name), e.Company),
		Location:       covetoLocation(jp.JobLocation),
		Description:    sanitizeHTML(jp.Description),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: covetoEmploymentType(jp.EmploymentType),
		PostedAt:       parsePubDate(p.PubDate),
		Countries:      covetoCountries(jp.JobLocation),
	}
	if sal := jp.BaseSalary; sal != nil {
		if period := schemaSalaryPeriod(sal.Value.UnitText); period != "" && sal.Currency != "" {
			j.SalaryMin, j.SalaryMax = roundSalaryPart(sal.Value.MinValue), roundSalaryPart(sal.Value.MaxValue)
			if j.SalaryMin != nil || j.SalaryMax != nil {
				j.SalaryCurrency, j.SalaryPeriod = strings.ToUpper(sal.Currency), period
			}
		}
	}
	return j, true
}

// covetoLocation joins the workplace addresses. The country is left out of the text: it is a
// bare ISO code ("DE") and travels in Countries instead.
func covetoLocation(places schemaPlaces) string {
	var parts []string
	for _, pl := range places {
		if l := joinNonEmpty(pl.Address.AddressLocality, pl.Address.AddressRegion); l != "" && !slices.Contains(parts, l) {
			parts = append(parts, l)
		}
	}
	return strings.Join(parts, "; ")
}

// covetoCountries reads the workplace addresses' countries, which coveto writes as ISO codes.
func covetoCountries(places schemaPlaces) []string {
	var out []string
	for _, pl := range places {
		if c := location.NormalizeCountry(pl.Address.AddressCountry); c != "" && !slices.Contains(out, c) {
			out = append(out, c)
		}
	}
	return out
}

// covetoEmploymentType maps the JobPosting employmentType, which coveto writes as one enum or a
// comma-joined list of them ("FULL_TIME,PART_TIME"). A list names several arrangements a
// posting offers, so only a single value is mapped; a list is left to the description parser.
func covetoEmploymentType(t string) string {
	if strings.Contains(t, ",") {
		return ""
	}
	return schemaEmploymentType(t)
}
