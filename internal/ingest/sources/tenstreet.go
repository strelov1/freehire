package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strings"
	"sync/atomic"
	"time"

	"golang.org/x/net/html"
)

// tenstreet adapts Tenstreet's "Pulse" share pages, the public job pages of a recruiting
// platform used almost entirely by US trucking and logistics carriers. A board is the carrier's
// numeric COMPANY ID, the company_id every posting link carries
// (pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=…&company_id=774&portal_user_id=1).
//
// The listing is the carrier's Pulse profile, share_app_index.php?company_id=<board>: one
// unpaginated, server-rendered page whose "Open Jobs" section links every open posting (a
// card per posting, sometimes repeated). It carries a title and a truncated teaser only, so
// this is a HydratingSource over the posting page, which embeds the posting as a JSON array
// (`var active_job_array = [...]`) — one row per hiring location, each repeating the full
// title, description and requirements.
type tenstreet struct {
	http HTMLGetter
}

// NewTenstreet builds the Tenstreet adapter over the given client.
func NewTenstreet(c HTMLGetter) Source { return tenstreet{http: c} }

func (tenstreet) Provider() string { return "tenstreet" }

// Tenstreet's posting pages are slow and uneven: a few stall for tens of seconds, and the page
// repeats the whole posting once per hiring location, so a nationwide one is ~1.8 MB. Hence the
// long-timeout client, a narrow detail pool, and a pacer held at a modest rate across every board
// of a run.
const (
	tenstreetDetailWorkers   = 4
	tenstreetRequestInterval = 500 * time.Millisecond // ~2 req/s
	tenstreetRequestBurst    = 4
)

// tenstreetBoard is the shape of a board id: a carrier's numeric company id.
var tenstreetBoard = regexp.MustCompile(`^[0-9]+$`)

// tenstreetPostingLink finds a posting link inside a card's onclick handler
// (window.location='https://pulse.tenstreet.com/share_app_show_active_jobs.php?…').
var tenstreetPostingLink = regexp.MustCompile(`share_app_show_active_jobs\.php\?([^'"\s]+)`)

// tenstreetStateCode is a US state's postal code, the geography every location row names.
var tenstreetStateCode = regexp.MustCompile(`^[A-Z]{2}$`)

// TenstreetListing is a carrier's Pulse profile: its name as Tenstreet states it and the ids
// of its open postings, deduplicated in page order.
type TenstreetListing struct {
	Company    string
	PostingIDs []string
}

// ReadTenstreetListing reads a carrier's Pulse profile. An unknown company id is answered with
// HTTP 200 and an empty profile — no carrier name and no jobs — which is reported as
// ErrBoardGone. It is exported for cmd/harvest-boards, whose prober validates a board with
// this one request rather than a crawl.
func ReadTenstreetListing(ctx context.Context, c HTMLGetter, companyID string) (TenstreetListing, error) {
	if !tenstreetBoard.MatchString(companyID) {
		return TenstreetListing{}, fmt.Errorf("tenstreet: board %q is not a numeric company id", companyID)
	}
	root, err := c.GetHTML(ctx, "https://pulse.tenstreet.com/share_app_index.php?company_id="+companyID)
	if err != nil {
		return TenstreetListing{}, fmt.Errorf("tenstreet: listing: %w", err)
	}
	l := parseTenstreetListing(root, companyID)
	if l.Company == "" {
		return TenstreetListing{}, fmt.Errorf("tenstreet: company %s: %w", companyID, ErrBoardGone)
	}
	return l, nil
}

// parseTenstreetListing reads the carrier name and the posting cards. A card's link is
// accepted only when it names this carrier's company id: the posting page is company-scoped
// (another carrier's posting id answers an empty page), so a link to another carrier is not a
// posting of this board.
func parseTenstreetListing(root *html.Node, companyID string) TenstreetListing {
	var l TenstreetListing
	seen := map[string]bool{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode {
			return true
		}
		if Attr(n, "id") == "carrier_detail_company_name" && l.Company == "" {
			l.Company = strings.Join(strings.Fields(textContent(n)), " ")
		}
		m := tenstreetPostingLink.FindStringSubmatch(Attr(n, "onclick"))
		if m == nil {
			return true
		}
		q, err := url.ParseQuery(m[1])
		if err != nil || q.Get("company_id") != companyID {
			return true
		}
		if id := q.Get("active_job_id"); tenstreetBoard.MatchString(id) && !seen[id] {
			seen[id] = true
			l.PostingIDs = append(l.PostingIDs, id)
		}
		return true
	})
	return l
}

// tenstreetPostingURL is a posting's public page. portal_user_id=1 is load-bearing: without
// it the page answers an empty posting (`active_job_array = null`).
func tenstreetPostingURL(companyID, postingID string) string {
	return "https://pulse.tenstreet.com/share_app_show_active_jobs.php?active_job_id=" + postingID +
		"&company_id=" + companyID + "&portal_user_id=1"
}

func (s tenstreet) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew reads the carrier's listing and the posting page of each posting the catalogue
// lacks. A page that could not be read, or that answers an empty posting (the platform's
// answer for a posting it has closed, under HTTP 200), is deferred to a later run rather than
// stored body-less. A crawl that listed new postings and read none of them fails.
func (s tenstreet) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board := strings.TrimSpace(e.Board)
	l, err := ReadTenstreetListing(ctx, s.http, board)
	if err != nil {
		return nil, err
	}
	var wanted, read atomic.Int64
	jobs := fetchDetails(l.PostingIDs, tenstreetDetailWorkers, func(id string) (Job, bool) {
		u := tenstreetPostingURL(board, id)
		if seen(id) {
			return Job{ExternalID: id, URL: u, Company: e.Company, SeenRefresh: true}, true
		}
		wanted.Add(1)
		root, err := s.http.GetHTML(ctx, u)
		if err != nil {
			log.Printf("tenstreet: %s: %v; deferring", u, err)
			return Job{}, false
		}
		job, ok := parseTenstreetPosting(root, id)
		if !ok {
			log.Printf("tenstreet: %s carries no posting; deferring", u)
			return Job{}, false
		}
		job.URL = u
		job.Company = e.Company
		read.Add(1)
		return job, true
	})
	if wanted.Load() > 0 && read.Load() == 0 {
		return nil, fmt.Errorf("tenstreet: listed %d new postings and read none of their pages", wanted.Load())
	}
	return jobs, nil
}

// tenstreetRow is one row of a posting page's active_job_array: the posting repeated once per
// hiring location. A location is either a whole state (location_type "select_states", the
// state's code in value) or a radius around a ZIP code ("zip_radius", the state in state and
// a ZIP-prefix in value).
type tenstreetRow struct {
	ActiveJobID     string `json:"active_job_id"`
	JobTitle        string `json:"job_title"`
	JobDescription  string `json:"job_description"`
	JobRequirements string `json:"job_requirements"`
	JobPay          string `json:"job_pay"`
	LocationType    string `json:"location_type"`
	Value           string `json:"value"`
	State           string `json:"state"`
}

// tenstreetArrayMarker opens the posting's JSON in the page's inline script.
const tenstreetArrayMarker = "var active_job_array = "

// parseTenstreetPosting maps a posting page. It reports false for a page that carries no
// posting — `active_job_array = null` (a closed posting, or one asked for under another
// carrier's company id) — rows naming another posting, or a posting with no title.
func parseTenstreetPosting(root *html.Node, postingID string) (Job, bool) {
	var rows []tenstreetRow
	found := false
	walk(root, func(n *html.Node) bool {
		if found {
			return false
		}
		if n.Type != html.ElementNode || n.Data != "script" {
			return true
		}
		text := textContent(n)
		_, rest, ok := strings.Cut(text, tenstreetArrayMarker)
		if !ok {
			return false
		}
		found = true
		// The decoder reads exactly one JSON value and ignores the script that follows it.
		if err := json.NewDecoder(strings.NewReader(rest)).Decode(&rows); err != nil {
			rows = nil
		}
		return false
	})
	if len(rows) == 0 || rows[0].ActiveJobID != postingID {
		return Job{}, false
	}
	first := rows[0]
	title := strings.Join(strings.Fields(first.JobTitle), " ")
	if title == "" {
		return Job{}, false
	}
	// A posting with no description and no requirements at all is the platform's own answer, not a
	// failed read, so it is yielded body-less rather than deferred forever; the pipeline re-offers
	// a body-less row for its hydration-retry window anyway.
	job := Job{ExternalID: postingID, Title: title, Description: tenstreetDescription(first)}
	if states := tenstreetStates(rows); len(states) > 0 {
		job.Location = strings.Join(states, ", ") + ", US"
		// The platform's own location model is US states — its page plots every row on a
		// US map as "US-<state>" — so the country is stated, not inferred. Spelling the codes
		// out in Location alone would not do: several collide with country codes ("IN",
		// "GA", "DE") and the location dictionary would read them as countries.
		job.Countries = []string{"us"}
	}
	return job, true
}

// tenstreetDescription joins the posting's description, its requirements and its pay note
// (free text, usually empty) into one sanitized body. Requirements are often an empty
// placeholder (a bare `<strong>&nbsp;</strong>`), which is left out.
func tenstreetDescription(r tenstreetRow) string {
	var b strings.Builder
	b.WriteString(r.JobDescription)
	if tenstreetHasText(r.JobRequirements) {
		b.WriteString("<h3>Requirements</h3>")
		b.WriteString(r.JobRequirements)
	}
	if pay := strings.TrimSpace(r.JobPay); pay != "" {
		b.WriteString("<h3>Pay</h3><p>")
		b.WriteString(html.EscapeString(pay))
		b.WriteString("</p>")
	}
	out := sanitizeHTML(b.String())
	if !tenstreetHasText(out) {
		return ""
	}
	return out
}

// tenstreetHasText reports whether an HTML fragment renders any visible text. textContent
// trims with unicode.IsSpace, which covers the U+00A0 an empty placeholder holds.
func tenstreetHasText(fragment string) bool {
	if strings.TrimSpace(fragment) == "" {
		return false
	}
	root, err := html.Parse(strings.NewReader(fragment))
	if err != nil {
		return false
	}
	return textContent(root) != ""
}

// tenstreetStates lists the distinct state codes the posting's location rows name, in page
// order.
func tenstreetStates(rows []tenstreetRow) []string {
	var out []string
	seen := map[string]bool{}
	for _, r := range rows {
		code := r.State
		if r.LocationType == "select_states" {
			code = r.Value
		}
		code = strings.ToUpper(strings.TrimSpace(code))
		if !tenstreetStateCode.MatchString(code) || seen[code] {
			continue
		}
		seen[code] = true
		out = append(out, code)
	}
	return out
}
