package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// peoplesoft adapts Oracle PeopleSoft HCM's Fluid Candidate Gateway, the careers site a
// PeopleSoft customer serves from its OWN host — universities, cities, national labs (Sandia,
// Case Western, the University of Missouri System, the DC government). There is no vendor
// host and no JSON: the site is a stateful PeopleTools page whose search results grow by a
// form POSTBACK, so the adapter replays the page's own form the way its "load more" button
// does.
//
// A board is "<host>/<site>/<portal>/<node>", the four parts of the Fluid search URL
// https://<host>/psc/<site>/<portal>/<node>/c/HRS_HRAM_FL.HRS_CG_SEARCH_FL.GBL. All four vary
// per tenant (careers.dc.gov/erecruit/EMPLOYEE/HRMS, erecruit.umsystem.edu/tamext/COLUM/HRMS),
// so none can be assumed.
//
// The listing carries no body, so this is a HydratingSource over the posting page, which a
// plain GET reaches by job opening id.
type peoplesoft struct {
	http peoplesoftHTTP
}

// peoplesoftHTTP is the transport the adapter needs: a GET and a form POST over ONE cookie
// jar, because the postback is only accepted inside the session the first GET opened.
type peoplesoftHTTP interface {
	HTMLGetter
	HeaderFormPoster
}

const (
	peoplesoftComponent = "/c/HRS_HRAM_FL.HRS_CG_SEARCH_FL.GBL"
	// peoplesoftLoadMore is the ICAction the result list's "load more" control submits. It is
	// a PeopleTools field name, not tenant configuration.
	peoplesoftLoadMore = "HRS_AGNT_RSLT_I$hdown$0"
	// peoplesoftMaxPostbacks bounds the walk; reaching it before the stated row count fails the
	// crawl. A postback adds 50 rows, so this covers 10,000 — the largest tenant measured holds
	// 600.
	peoplesoftMaxPostbacks = 200
	// peoplesoftDetailWorkers is the posting-page fan-out. Every request of a board rides one
	// PeopleTools session, so it stays well under the shared default.
	peoplesoftDetailWorkers = 4
)

// NewPeopleSoft builds the PeopleSoft Candidate Gateway adapter over a cookie-persisting client.
func NewPeopleSoft(c peoplesoftHTTP) Source { return peoplesoft{http: c} }

func (peoplesoft) Provider() string { return "peoplesoft" }

// fullBoardListing: the result list states its row count, and the walk keeps posting back until
// the page holds that many rows. A page that fails, a postback that adds no row before the count
// is reached, and the postback cap all fail the crawl.
func (peoplesoft) fullBoardListing() {}

// peoplesoftBoard is a parsed board id.
type peoplesoftBoard struct {
	host, path string // path = "<site>/<portal>/<node>"
}

func parsePeopleSoftBoard(board string) (peoplesoftBoard, error) {
	parts := strings.Split(strings.Trim(board, "/"), "/")
	if len(parts) != 4 {
		return peoplesoftBoard{}, fmt.Errorf("peoplesoft: board %q is not <host>/<site>/<portal>/<node>", board)
	}
	for _, p := range parts {
		if p == "" || strings.ContainsAny(p, "?#:@ ") {
			return peoplesoftBoard{}, fmt.Errorf("peoplesoft: board %q is not <host>/<site>/<portal>/<node>", board)
		}
	}
	return peoplesoftBoard{host: strings.ToLower(parts[0]), path: strings.Join(parts[1:], "/")}, nil
}

func (b peoplesoftBoard) component() string {
	return "https://" + b.host + "/psc/" + b.path + peoplesoftComponent
}

func (b peoplesoftBoard) searchURL() string {
	return b.component() + "?Page=HRS_APP_SCHJOB_FL&Action=U&FOCUS=Applicant&SiteId=1"
}

// postingURL is the posting page, and what a visitor opens: the first request answers a 302 to
// itself that sets the session cookie, which a browser follows like any redirect.
func (b peoplesoftBoard) postingURL(id string) string {
	return b.component() + "?Page=HRS_APP_JBPST_FL&Action=U&FOCUS=Applicant&SiteId=1&JobOpeningId=" +
		url.QueryEscape(id) + "&PostingSeq=1"
}

// peoplesoftRow is one row of the result list.
type peoplesoftRow struct {
	ID, Title, Location, Opened string
}

// peoplesoftPage is one state of the result list.
type peoplesoftPage struct {
	total  int // the row count the list states; -1 when it states none
	rows   []peoplesoftRow
	action string
	fields url.Values
}

func (s peoplesoft) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

func (s peoplesoft) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	b, err := parsePeopleSoftBoard(e.Board)
	if err != nil {
		return nil, err
	}
	rows, err := s.list(ctx, b)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, b, e, rows, seen)
}

// list walks the result list to its stated row count. Every postback answers the WHOLE list so
// far (50 rows more each time), so the last page read holds every row.
func (s peoplesoft) list(ctx context.Context, b peoplesoftBoard) ([]peoplesoftRow, error) {
	root, err := s.http.GetHTML(ctx, b.searchURL())
	if err != nil {
		return nil, fmt.Errorf("peoplesoft: search page: %w", err)
	}
	page, err := parsePeopleSoftPage(root)
	if err != nil {
		return nil, err
	}
	for i := 0; page.total >= 0 && len(page.rows) < page.total; i++ {
		if i == peoplesoftMaxPostbacks {
			return nil, fmt.Errorf("peoplesoft: %d of %d rows after %d postbacks", len(page.rows), page.total, i)
		}
		action, err := url.Parse(page.action)
		if err != nil || action.Scheme != "https" || !strings.EqualFold(action.Host, b.host) {
			return nil, fmt.Errorf("peoplesoft: form posts to %q, not to the board's host", page.action)
		}
		page.fields.Set("ICAction", peoplesoftLoadMore)
		next, err := s.http.PostFormWithHeaders(ctx, action.String(), nil, page.fields)
		if err != nil {
			return nil, fmt.Errorf("peoplesoft: postback %d: %w", i+1, err)
		}
		more, err := parsePeopleSoftPage(next)
		if err != nil {
			return nil, fmt.Errorf("peoplesoft: postback %d: %w", i+1, err)
		}
		if len(more.rows) <= len(page.rows) {
			return nil, fmt.Errorf("peoplesoft: postback %d added no row (%d of %d)", i+1, len(more.rows), page.total)
		}
		page = more
	}
	if page.total < 0 && len(page.rows) > 0 {
		return nil, fmt.Errorf("peoplesoft: %d rows and no stated row count", len(page.rows))
	}
	return page.rows, nil
}

// hydrate reads the posting page of each posting the catalogue lacks. A posting whose page could
// not be read is deferred to a later run rather than stored body-less, since a stored row is seen
// and never hydrated again; a crawl that listed new postings and read none of them fails.
func (s peoplesoft) hydrate(ctx context.Context, b peoplesoftBoard, e CompanyEntry, rows []peoplesoftRow, seen func(string) bool) ([]Job, error) {
	// A posting listed twice (a tenant may show one opening under two postings) is one posting.
	var uniq []peoplesoftRow
	ids := map[string]bool{}
	for _, r := range rows {
		if !ids[r.ID] {
			ids[r.ID] = true
			uniq = append(uniq, r)
		}
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(uniq, peoplesoftDetailWorkers, func(r peoplesoftRow) (Job, bool) {
		link := b.postingURL(r.ID)
		if seen(r.ID) {
			return Job{ExternalID: r.ID, URL: link, Title: r.Title, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, link)
		if err != nil {
			log.Printf("peoplesoft: %s posting %s: %v; deferring", b.host, r.ID, err)
			return Job{}, false
		}
		job, ok := parsePeopleSoftPosting(root, r)
		if !ok {
			log.Printf("peoplesoft: %s posting %s answered no posting; deferring", b.host, r.ID)
			return Job{}, false
		}
		job.URL, job.Company = link, e.Company
		read.Add(1)
		return job, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("peoplesoft: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// peoplesoftIndex maps every element id on a page to its element. A list page holds hundreds of
// rows of five fields each, so a lookup per field would walk the tree thousands of times.
func peoplesoftIndex(root *html.Node) map[string]*html.Node {
	idx := map[string]*html.Node{}
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode {
			if id := Attr(n, "id"); id != "" {
				if _, dup := idx[id]; !dup {
					idx[id] = n
				}
			}
		}
		return true
	})
	return idx
}

func peoplesoftText(idx map[string]*html.Node, id string) string {
	if n := idx[id]; n != nil {
		return strings.Join(strings.Fields(textContent(n)), " ")
	}
	return ""
}

var peoplesoftRowID = regexp.MustCompile(`^HRS_AGNT_RSLT_I\$0_row_(\d+)$`)

// parsePeopleSoftPage reads one state of the result list. A page without the "win0" form is not
// a result list at all — a sign-in page, an error page, a "site name is not valid" answer — and
// is an error, never an empty board.
func parsePeopleSoftPage(root *html.Node) (peoplesoftPage, error) {
	var form *html.Node
	walk(root, func(n *html.Node) bool {
		if form != nil {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "form" && Attr(n, "name") == "win0" {
			form = n
			return false
		}
		return true
	})
	if form == nil {
		return peoplesoftPage{}, fmt.Errorf("peoplesoft: the page is not a PeopleSoft search page (no win0 form)")
	}
	page := peoplesoftPage{total: -1, action: Attr(form, "action"), fields: peoplesoftFormFields(form)}
	idx := peoplesoftIndex(root)
	if n, ok := peoplesoftCount(peoplesoftText(idx, "win0divHRS_AGNT_RSLT_Irowcnt$0")); ok {
		page.total = n
	}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		m := peoplesoftRowID.FindStringSubmatch(Attr(n, "id"))
		if m == nil {
			return true
		}
		i := m[1]
		row := peoplesoftRow{
			ID:       peoplesoftText(idx, "HRS_APP_JBSCH_I_HRS_JOB_OPENING_ID$"+i),
			Title:    peoplesoftText(idx, "SCH_JOB_TITLE$"+i),
			Location: peoplesoftText(idx, "LOCATION$"+i),
			Opened:   peoplesoftText(idx, "SCH_OPENED$"+i),
		}
		if row.ID != "" && row.Title != "" {
			page.rows = append(page.rows, row)
		}
		return false
	})
	return page, nil
}

// peoplesoftFormFields collects the form's state the way a browser submits it: every named
// input, an unchecked checkbox or radio excluded, plus textareas and selects. The postback is
// refused unless the WHOLE state comes back, ICSID and ICStateNum among it.
func peoplesoftFormFields(form *html.Node) url.Values {
	v := url.Values{}
	walk(form, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		name := Attr(n, "name")
		switch n.Data {
		case "input":
			typ := strings.ToLower(Attr(n, "type"))
			if name == "" || ((typ == "checkbox" || typ == "radio") && !peoplesoftHasAttr(n, "checked")) {
				return true
			}
			v.Set(name, Attr(n, "value"))
		case "textarea":
			if name != "" {
				v.Set(name, textContent(n))
			}
			return false
		case "select":
			if name != "" {
				walk(n, func(o *html.Node) bool {
					if o.Type == html.ElementNode && o.Data == "option" && peoplesoftHasAttr(o, "selected") {
						v.Set(name, Attr(o, "value"))
					}
					return true
				})
			}
			return false
		}
		return true
	})
	return v
}

func peoplesoftHasAttr(n *html.Node, key string) bool {
	for _, a := range n.Attr {
		if a.Key == key {
			return true
		}
	}
	return false
}

var (
	peoplesoftGrouping = regexp.MustCompile(`(\d)[\s\x{00a0}.,](\d{3})\b`)
	peoplesoftOf       = regexp.MustCompile(`(?i)\bof\s+(\d+)`)
	peoplesoftDigits   = regexp.MustCompile(`\d+`)
)

// peoplesoftCount reads the list's row count: "68 rows", or a localized "1 of 96" / "Ligne 1 sur
// 96" when the tenant renders a position counter, where the total is the last number.
func peoplesoftCount(s string) (int, bool) {
	s = peoplesoftGrouping.ReplaceAllString(s, "$1$2")
	if m := peoplesoftOf.FindStringSubmatch(s); m != nil {
		n, err := strconv.Atoi(m[1])
		return n, err == nil
	}
	nums := peoplesoftDigits.FindAllString(s, -1)
	if len(nums) == 0 {
		return 0, false
	}
	n, err := strconv.Atoi(nums[len(nums)-1])
	return n, err == nil
}

// parsePeopleSoftPosting maps a posting page. A page whose opening id is not the one asked for is
// not this posting: an opening that no longer exists lands on the search page instead.
func parsePeopleSoftPosting(root *html.Node, r peoplesoftRow) (Job, bool) {
	idx := peoplesoftIndex(root)
	if peoplesoftText(idx, "HRS_SCH_WRK2_HRS_JOB_OPENING_ID") != r.ID {
		return Job{}, false
	}
	var body strings.Builder
	for i := 0; ; i++ {
		n := idx["HRS_SCH_PSTDSC_DESCRLONG$"+strconv.Itoa(i)]
		if n == nil {
			break
		}
		if label := peoplesoftText(idx, "HRS_SCH_WRK_DESCR100$"+strconv.Itoa(i)+"lbl"); label != "" {
			body.WriteString("<h3>" + html.EscapeString(label) + "</h3>")
		}
		body.WriteString(innerHTML(n))
	}
	desc := sanitizeHTML(body.String())
	if strings.TrimSpace(desc) == "" {
		return Job{}, false
	}
	title := peoplesoftText(idx, "HRS_SCH_WRK2_POSTING_TITLE")
	if title == "" {
		title = r.Title
	}
	location := r.Location
	if location == "" {
		location = peoplesoftText(idx, "HRS_SCH_WRK_HRS_DESCRLONG")
	}
	job := Job{
		ExternalID:  r.ID,
		Title:       title,
		Location:    location,
		Description: desc,
		PostedAt:    peoplesoftDate(r.Opened),
	}
	switch strings.ToLower(peoplesoftText(idx, "HRS_SCH_WRK_HRS_FULL_PART_TIME")) {
	case "full-time", "full time":
		job.EmploymentType = "full_time"
	case "part-time", "part time":
		job.EmploymentType = "part_time"
	}
	return job, true
}

// peoplesoftDate reads the list's "Posted" date. Every tenant measured renders M/D/YYYY; any
// other shape is left unread rather than guessed at, since 03/04 reads two ways.
func peoplesoftDate(s string) *time.Time {
	return parseLayout("1/2/2006", strings.TrimSpace(s))
}
