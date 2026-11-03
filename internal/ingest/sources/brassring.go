package sources

import (
	"context"
	"fmt"
	"html"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"

	xhtml "golang.org/x/net/html"
)

// brassring adapts IBM Kenexa BrassRing "Talent Gateway" career sites, served from regional
// hosts of brassring.com (sjobs, xjobs, krb-sjobs, ...).
//
// A board is "<host>?partnerid=<n>&siteid=<n>": the host plus the two numbers the site's own
// URL carries (https://sjobs.brassring.com/TGnewUI/Search/Home/Home?partnerid=25416&siteid=6007).
// The host is part of the identity because the same partner/site pair is meaningless on
// another region's host, and the platform offers no host-free address. brassringBoardIdentity
// folds the spellings of one site (key case, key order, host case) into one board.
//
// The site is an AngularJS app, but it talks to a JSON API under /TgNewUI/Search/Ajax/ that a
// plain client can replay, within the session the site's home page opens:
//
//   - a GET of the home page sets the session cookies and embeds two hidden inputs — the
//     anti-forgery token (sent back as an "RFT" header) and the encrypted session value (sent
//     back inside every request body);
//   - ProcessSortAndShowMoreJobs lists the board, 50 postings a page, and states the total;
//   - JobDetails returns one posting's full body.
//
// The listing's own "jobdescription" is only a summary on many tenants (150-400 characters on
// Caterpillar's), so this is a HydratingSource: the detail is read for each posting the
// catalogue lacks. The fields a tenant shows are configured per tenant (formtext8 is a city on
// one site and a free-text title on another), so nothing is read by field number: the body is
// every long-text field the detail carries, in the order the site shows them under the label
// the site gives them, and the location is built from the fields the site LABELS as city,
// state, province or country (or, failing those, as a location). A tenant that leaves its
// location fields unlabelled (ADM's carry no label at all) yields no location rather than a
// guess at which unnamed field holds one.
//
// The listing carries only a last-updated date, which an employer's edit moves, so no PostedAt
// is set: reading it as the posting date would age a freshly edited posting backwards.
type brassring struct {
	http  brassringHTTP
	hosts *keyedMutex
}

// brassringHTTP is the transport the adapter needs: a GET for the home page and a
// header-carrying JSON POST, over ONE cookie jar, because the API only answers inside the
// session the home page opened.
type brassringHTTP interface {
	HTMLGetter
	HeaderJSONPoster
}

const (
	// brassringMaxPages bounds the walk; reaching it before the stated total fails the crawl.
	// It covers 10,000 postings — the largest sampled board holds 131.
	brassringMaxPages = 200
	// brassringDetailWorkers is the detail fan-out. Every request of a board rides one session
	// and the platform documents no request limit, so it stays under the shared default.
	brassringDetailWorkers = 4
)

// NewBrassring builds the BrassRing adapter over a cookie-persisting client (see
// cookieSessionSource).
func NewBrassring(c brassringHTTP) Source { return brassring{http: c, hosts: newKeyedMutex()} }

func (brassring) Provider() string { return "brassring" }

// fullBoardListing: the listing states its total, and the walk keeps paging until it holds that
// many rows. A page that fails, an empty page before the total is reached, and the page cap all
// fail the crawl rather than returning a truncated board as a complete one.
func (brassring) fullBoardListing() {}

// brassringBoard is a parsed board id.
type brassringBoard struct {
	host, partner, site string
}

var (
	brassringHostRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*(\.[a-z0-9][a-z0-9-]*)*\.brassring\.com$`)
	brassringIDRe   = regexp.MustCompile(`^[0-9]+$`)
)

// parseBrassringBoard splits "<host>?partnerid=<n>&siteid=<n>". The host is spliced into a URL,
// so it must be a brassring.com host; the two ids must be plain numbers.
func parseBrassringBoard(board string) (brassringBoard, error) {
	bad := func() (brassringBoard, error) {
		return brassringBoard{}, fmt.Errorf("brassring: board %q is not <host>.brassring.com?partnerid=<n>&siteid=<n>", board)
	}
	host, query, found := strings.Cut(board, "?")
	if !found {
		return bad()
	}
	host = strings.ToLower(host)
	q, err := url.ParseQuery(query)
	if err != nil || !brassringHostRe.MatchString(host) {
		return bad()
	}
	// Keys are case-insensitive on the platform ("partnerid" in site URLs, "PartnerId" in its API).
	var partner, site string
	for k, v := range q {
		if len(v) != 1 {
			return bad()
		}
		switch strings.ToLower(k) {
		case "partnerid":
			partner = v[0]
		case "siteid":
			site = v[0]
		default:
			return bad()
		}
	}
	if !brassringIDRe.MatchString(partner) || !brassringIDRe.MatchString(site) {
		return bad()
	}
	return brassringBoard{host: host, partner: partner, site: site}, nil
}

// String renders the canonical board id.
func (b brassringBoard) String() string {
	return b.host + "?partnerid=" + b.partner + "&siteid=" + b.site
}

// brassringBoardIdentity is boardIdentity's entry for this provider: every spelling of one
// site is one crawl target. A board that does not parse is returned unchanged; Fetch rejects it.
func brassringBoardIdentity(board string) string {
	b, err := parseBrassringBoard(board)
	if err != nil {
		return board
	}
	return b.String()
}

func (b brassringBoard) homeURL() string {
	return "https://" + b.host + "/TGnewUI/Search/Home/Home?partnerid=" + b.partner + "&siteid=" + b.site
}

func (b brassringBoard) apiURL(endpoint string) string {
	return "https://" + b.host + "/TgNewUI/Search/Ajax/" + endpoint
}

// postingURL is the posting's public page, what a visitor opens.
func (b brassringBoard) postingURL(id string) string {
	return "https://" + b.host + "/TGnewUI/Search/home/HomeWithPreLoad?partnerid=" + b.partner +
		"&siteid=" + b.site + "&PageType=JobDetails&jobid=" + url.QueryEscape(id)
}

// brassringSession is what the home page hands the API: the anti-forgery token and the
// encrypted session value.
type brassringSession struct {
	headers map[string]string
	value   string
}

// open GETs the home page, which sets the session cookies in the client's jar, and reads the
// two hidden inputs the API requires.
func (s brassring) open(ctx context.Context, b brassringBoard) (brassringSession, error) {
	root, err := s.http.GetHTML(ctx, b.homeURL())
	if err != nil {
		return brassringSession{}, fmt.Errorf("brassring: home page %s: %w", b, err)
	}
	var token, value string
	walk(root, func(n *xhtml.Node) bool {
		if n.Type != xhtml.ElementNode || n.Data != "input" {
			return true
		}
		switch {
		case Attr(n, "name") == "__RequestVerificationToken":
			token = Attr(n, "value")
		case Attr(n, "id") == "CookieValue":
			value = Attr(n, "value")
		}
		return true
	})
	if token == "" || value == "" {
		return brassringSession{}, fmt.Errorf("brassring: home page %s carries no session token (not a Talent Gateway site?)", b)
	}
	return brassringSession{
		headers: map[string]string{
			"RFT":              token,
			"X-Requested-With": "XMLHttpRequest",
			"Referer":          b.homeURL(),
		},
		value: value,
	}, nil
}

// brassringListRequest is the listing POST body: the empty search, newest first.
type brassringListRequest struct {
	PartnerID                string `json:"partnerId"`
	SiteID                   string `json:"siteId"`
	Keyword                  string `json:"keyword"`
	Location                 string `json:"location"`
	KeywordCustomSolrFields  string `json:"keywordCustomSolrFields"`
	LocationCustomSolrFields string `json:"locationCustomSolrFields"`
	LinkID                   string `json:"linkId"`
	Latitude                 int    `json:"Latitude"`
	Longitude                int    `json:"Longitude"`
	FacetFilterFields        struct {
		Facet []string `json:"Facet"`
	} `json:"facetfilterfields"`
	PowerSearchOptions struct {
		PowerSearchOption []string `json:"PowerSearchOption"`
	} `json:"powersearchoptions"`
	SortType              string `json:"SortType"`
	PageNumber            int    `json:"pageNumber"`
	EncryptedSessionValue string `json:"encryptedSessionValue"`
}

// brassringListResponse is the part of a listing page the adapter reads. Jobs is null on an
// empty board.
type brassringListResponse struct {
	Jobs *struct {
		Job []brassringListed `json:"Job"`
	} `json:"Jobs"`
	JobsCount int `json:"JobsCount"`
}

// brassringListed is one posting of a listing page: a bag of the tenant's configured fields.
type brassringListed struct {
	Questions []struct {
		Name  string `json:"QuestionName"`
		Value string `json:"Value"`
	} `json:"Questions"`
}

func (l brassringListed) field(name string) string {
	for _, q := range l.Questions {
		if q.Name == name {
			return strings.TrimSpace(q.Value)
		}
	}
	return ""
}

// list walks the board to its stated total.
func (s brassring) list(ctx context.Context, b brassringBoard, sess brassringSession) ([]brassringListed, error) {
	var rows []brassringListed
	for page := 1; ; page++ {
		if page > brassringMaxPages {
			return nil, fmt.Errorf("brassring: %s: %d rows after the %d-page cap: walk truncated, not exhausted", b, len(rows), brassringMaxPages)
		}
		req := brassringListRequest{
			PartnerID: b.partner, SiteID: b.site, SortType: "LastUpdated",
			PageNumber: page, EncryptedSessionValue: sess.value,
		}
		req.FacetFilterFields.Facet = []string{}
		req.PowerSearchOptions.PowerSearchOption = []string{}
		var resp brassringListResponse
		if err := s.http.PostJSONWithHeaders(ctx, b.apiURL("ProcessSortAndShowMoreJobs"), sess.headers, req, &resp); err != nil {
			return nil, fmt.Errorf("brassring: %s page %d: %w", b, page, err)
		}
		if resp.Jobs != nil {
			rows = append(rows, resp.Jobs.Job...)
		}
		switch {
		case len(rows) >= resp.JobsCount:
			return rows, nil
		case resp.Jobs == nil || len(resp.Jobs.Job) == 0:
			return nil, fmt.Errorf("brassring: %s: %d of %d rows, then an empty page %d", b, len(rows), resp.JobsCount, page)
		}
	}
}

func (s brassring) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s brassring) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parseBrassringBoard(e.Board)
	if err != nil {
		return nil, err
	}
	// The jar is host-scoped, so two boards on one host would overwrite each other's session
	// cookies mid-crawl. Boards of different hosts proceed concurrently.
	defer s.hosts.lock(b.host)()

	sess, err := s.open(ctx, b)
	if err != nil {
		return nil, err
	}
	rows, err := s.list(ctx, b, sess)
	if err != nil {
		return nil, err
	}

	// A posting listed twice is one posting; one with no id could never be stored.
	var uniq []brassringListed
	ids := map[string]bool{}
	for _, r := range rows {
		if id := r.field("reqid"); id != "" && !ids[id] {
			ids[id] = true
			uniq = append(uniq, r)
		}
	}

	var wanted, read atomic.Int64
	jobs := fetchDetails(uniq, brassringDetailWorkers, func(r brassringListed) (Job, bool) {
		id := r.field("reqid")
		link := b.postingURL(id)
		title := html.UnescapeString(r.field("jobtitle"))
		if seen(id) {
			return Job{ExternalID: id, URL: link, Title: title, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		d, err := s.detail(ctx, b, sess, id)
		if err != nil {
			log.Printf("brassring: %s posting %s: %v; deferring", b, id, err)
			if detailUnreadable(err) {
				return unreadableDetail(id, link, e.Company), true
			}
			return Job{}, false // the platform answers 404/410: the posting is gone
		}
		if d == nil {
			// Listed, yet the detail endpoint names no posting: stated nothing about it.
			log.Printf("brassring: %s posting %s answered no posting; deferring", b, id)
			return unreadableDetail(id, link, e.Company), true
		}
		read.Add(1)
		if title == "" {
			title = html.UnescapeString(strings.TrimSpace(d.Title))
		}
		location := d.location()
		remote := isRemote(location)
		return Job{
			ExternalID:  id,
			URL:         link,
			Title:       title,
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(d.description()),
			Remote:      remote,
			WorkMode:    workModeFromRemote(remote),
		}, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("brassring: %s: listed %d new postings and read none of their details", b, wanted.Load())
	}
	return jobs, nil
}

// brassringDetailRequest is the JobDetails POST body.
type brassringDetailRequest struct {
	PartnerID             string `json:"partnerId"`
	SiteID                string `json:"siteId"`
	JobID                 string `json:"jobid"`
	ConfigMode            string `json:"configMode"`
	JobSiteID             string `json:"jobSiteId"`
	EncryptedSessionValue string `json:"encryptedSessionValue"`
}

// brassringDetail is one posting's detail. Jobdetails is null for an id the platform does not
// know.
type brassringDetail struct {
	ServiceResponse struct {
		Jobdetails *brassringPosting `json:"Jobdetails"`
	} `json:"ServiceResponse"`
}

type brassringPosting struct {
	Title     string `json:"Title"`
	Questions []struct {
		Name  string `json:"QuestionName"`
		Value string `json:"AnswerValue"`
		Type  string `json:"QuestionType"`
	} `json:"JobDetailQuestions"`
}

// detail reads one posting; a nil posting with a nil error means the platform names none.
func (s brassring) detail(ctx context.Context, b brassringBoard, sess brassringSession, id string) (*brassringPosting, error) {
	req := brassringDetailRequest{
		PartnerID: b.partner, SiteID: b.site, JobID: id, JobSiteID: b.site, EncryptedSessionValue: sess.value,
	}
	var resp brassringDetail
	if err := s.http.PostJSONWithHeaders(ctx, b.apiURL("JobDetails"), sess.headers, req, &resp); err != nil {
		return nil, err
	}
	return resp.ServiceResponse.Jobdetails, nil
}

// brassringLabel is a field's label as the site shows it. Tenants also use the label to leave
// themselves a note in the admin ("<!--Pay Range-->"), which is not for a reader.
func brassringLabel(name string) string {
	name = strings.TrimSpace(name)
	if strings.HasPrefix(name, "<!--") && strings.HasSuffix(name, "-->") {
		name = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(name, "<!--"), "-->"))
	}
	return name
}

// description is every long-text field, in the order the site shows them, under its label.
func (p *brassringPosting) description() string {
	var sb strings.Builder
	for _, q := range p.Questions {
		if q.Type != "textarea" || strings.TrimSpace(q.Value) == "" {
			continue
		}
		if label := brassringLabel(q.Name); label != "" {
			sb.WriteString("<h3>" + html.EscapeString(label) + "</h3>")
		}
		sb.WriteString("<div>" + q.Value + "</div>")
	}
	return sb.String()
}

// location joins the fields the site labels city, state/province and country, or — when it
// labels none of them — the first one it labels a location.
func (p *brassringPosting) location() string {
	var city, state, country, fallback string
	for _, q := range p.Questions {
		v := strings.TrimSpace(html.UnescapeString(q.Value))
		if v == "" || q.Type == "textarea" {
			continue
		}
		switch label := strings.ToLower(strings.TrimRight(brassringLabel(q.Name), ": ")); {
		case label == "city" && city == "":
			city = v
		case (label == "state" || label == "province" || label == "state/province") && state == "":
			state = v
		case label == "country" && country == "":
			country = v
		case strings.Contains(label, "location") && fallback == "":
			fallback = v
		}
	}
	if loc := joinNonEmpty(city, state, country); loc != "" {
		return loc
	}
	return fallback
}
