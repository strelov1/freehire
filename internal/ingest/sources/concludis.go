package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// concludis adapts concludis (concludis.de), a German ATS whose hosted job boards live at
// https://<board>.concludis.de. Every tenant serves its complete list of open positions at one
// fixed address, /prj/lst/a181a603769c1f98ad927e7367c7aa51/GesamtlisteOffenePositionen.htm —
// the same hash on every tenant — server-rendered, paged under ?page=N at a page size the
// tenant chooses (15 and 25 are both in use), and stating its own extent: "46 Stellen
// gefunden", "1 Stelle gefunden", or "Derzeit haben wir keine offenen Stellen." A page past
// the end repeats the last one. A line carries the title and the posting's address; the body
// is on the posting page, as a schema.org JobPosting with the employer (a group tenant posts
// for its subsidiaries — hhla for METRANS), the workplace address and the employment type. So
// this is a HydratingSource.
//
// A tenant that publishes its jobs on its own site instead REDIRECTS the list address there
// (edeka → verbund.edeka/karriere): leaving <board>.concludis.de is read as ErrBoardGone, the
// destination named in the error, because nothing this adapter can read is served any more.
type concludis struct {
	http concludisGetter
}

// concludisGetter is the transport the adapter needs: the list pages with their final URL (to
// see a tenant redirecting away), and each posting's page.
type concludisGetter interface {
	HTMLResolvedGetter
	HTMLGetter
}

// NewConcludis builds the concludis adapter over the given HTTP client.
func NewConcludis(c concludisGetter) Source { return concludis{http: c} }

func (concludis) Provider() string { return "concludis" }

// fullBoardListing: the walk fails unless it collects exactly the number of positions the list
// states, and its page cap is a ceiling that fails the crawl rather than ending it.
func (concludis) fullBoardListing() {}

// concludisListPath is the complete-list address every tenant serves.
const concludisListPath = "/prj/lst/a181a603769c1f98ad927e7367c7aa51/GesamtlisteOffenePositionen.htm"

// concludisMaxPages is a safety ceiling on the ?page=N walk. The largest tenant measured
// (edeka-rr, 668 positions at 25 a page) needs 27; reaching the ceiling fails the crawl.
const concludisMaxPages = 200

// concludisBoard is the shape of a tenant label.
var concludisBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

var (
	// concludisCount reads the list's stated extent ("46 Stellen gefunden").
	concludisCount = regexp.MustCompile(`([0-9]+)\s+Stellen?\s+gefunden`)
	// concludisOpenJob captures the posting address from a line's onclick handler.
	concludisOpenJob = regexp.MustCompile(`openJob\('([^']+)'`)
	// concludisPostingID captures the position id from a posting path
	// (/prj/shw/<hash>_0/6635/<title>.htm).
	concludisPostingID = regexp.MustCompile(`^/prj/shw/[0-9a-f]+_[0-9]+/([0-9]+)/`)
)

// concludisNoOpenings is the list's statement that the tenant has no open position.
const concludisNoOpenings = "keine offenen Stellen"

// concludisLine is one line of the list.
type concludisLine struct {
	ID, URL, Title string
}

// Fetch walks the list and reads every posting's page.
func (s concludis) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	lines, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(lines, defaultDetailWorkers, func(l concludisLine) (Job, bool) {
		return s.detail(ctx, e, l)
	}), nil
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold; a
// seen posting is re-listed as a SeenRefresh, the list itself proving it is still open.
func (s concludis) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	lines, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(lines, defaultDetailWorkers, func(l concludisLine) (Job, bool) {
		if seen(l.ID) {
			return Job{ExternalID: l.ID, URL: l.URL, Title: l.Title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, l)
	}), nil
}

// list walks the list pages. ANY page failing fails the board, and so does a walk that ends
// without collecting the stated number of positions: the list states its own extent, so a
// short walk is a truncation the unseen sweep must never read as closures.
func (s concludis) list(ctx context.Context, board string) ([]concludisLine, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !concludisBoard.MatchString(board) {
		return nil, fmt.Errorf("concludis: board %q is not a tenant label", board)
	}
	host := board + ".concludis.de"
	var lines []concludisLine
	seen := map[string]bool{}
	want := -1
	for page := 1; page <= concludisMaxPages; page++ {
		root, final, err := s.http.GetHTMLResolved(ctx, fmt.Sprintf("https://%s%s?page=%d", host, concludisListPath, page))
		if err != nil {
			return nil, fmt.Errorf("concludis: board %s page %d: %w", board, page, err)
		}
		if u, perr := url.Parse(final); perr == nil && final != "" && !strings.EqualFold(u.Hostname(), host) {
			return nil, fmt.Errorf("concludis: board %s now publishes at %s: %w", board, final, ErrBoardGone)
		}
		if want < 0 {
			if want, err = concludisStated(root); err != nil {
				return nil, fmt.Errorf("concludis: board %s: %w", board, err)
			}
			if want == 0 {
				return nil, nil
			}
		}
		added := 0
		for _, l := range concludisLines(root, host) {
			if !seen[l.ID] {
				seen[l.ID] = true
				lines = append(lines, l)
				added++
			}
		}
		if len(lines) >= want {
			break
		}
		if added == 0 {
			return nil, fmt.Errorf("concludis: board %s: list ended at %d of the %d positions it states", board, len(lines), want)
		}
		if page == concludisMaxPages {
			return nil, fmt.Errorf("concludis: board %s: %d pages without reaching the %d positions it states", board, page, want)
		}
	}
	if len(lines) != want {
		return nil, fmt.Errorf("concludis: board %s: list holds %d positions, states %d", board, len(lines), want)
	}
	return lines, nil
}

// concludisStated reads the number of positions the list states. A page stating neither a
// count nor the no-openings notice is a reshaped page, not an empty board.
func concludisStated(root *html.Node) (int, error) {
	sum := firstByClass(root, "stellensum")
	if sum == nil {
		return 0, fmt.Errorf("list page carries no position count")
	}
	text := strings.Join(strings.Fields(textContent(sum)), " ")
	if m := firstSubmatch(concludisCount, text); m != "" {
		return strconv.Atoi(m)
	}
	if strings.Contains(text, concludisNoOpenings) {
		return 0, nil
	}
	return 0, fmt.Errorf("list page states %q, not a position count", text)
}

// concludisLines reads every line of a list page: the posting address from its onclick handler
// and the title from its stellenlink span. A line whose address is not a posting on the
// tenant's own host is skipped.
func concludisLines(root *html.Node, host string) []concludisLine {
	var lines []concludisLine
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || !strings.HasPrefix(Attr(n, "id"), "line_") {
			return true
		}
		u, err := url.Parse(firstSubmatch(concludisOpenJob, Attr(n, "onclick")))
		if err != nil || !strings.EqualFold(u.Hostname(), host) {
			return false
		}
		id := firstSubmatch(concludisPostingID, u.Path)
		if id == "" {
			return false
		}
		l := concludisLine{ID: id, URL: u.String()}
		if t := firstByClass(n, "stellenlink"); t != nil {
			l.Title = strings.TrimSpace(textContent(t))
		}
		lines = append(lines, l)
		return false
	})
	return lines
}

// concludisJobPosting is the posting page's schema.org JobPosting.
type concludisJobPosting struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation schemaPlaces `json:"jobLocation"`
}

// detail fetches one posting page and maps its JobPosting, with the list line, to a Job. A 404
// is the platform's own "no such posting" and drops it; any other failure, or a page carrying
// no JobPosting body, is an unreadableDetail marker — the page is the only source of the body.
func (s concludis) detail(ctx context.Context, e CompanyEntry, l concludisLine) (Job, bool) {
	root, err := s.http.GetHTML(ctx, l.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(l.ID, l.URL, e.Company), true
		}
		return Job{}, false
	}
	var jp concludisJobPosting
	if !ldJobPosting(root, &jp) || strings.TrimSpace(jp.Description) == "" {
		return unreadableDetail(l.ID, l.URL, e.Company), true
	}
	// No sampled posting (90, across every live tenant) states jobLocationType; TELECOMMUTE is
	// still read as the remote statement schema.org defines it to be, and its absence says
	// nothing — the description parser decides.
	remote := strings.EqualFold(strings.TrimSpace(jp.JobLocationType), "TELECOMMUTE")
	loc, countries := concludisPlaces(jp.JobLocation)
	return Job{
		ExternalID:     l.ID,
		URL:            l.URL,
		Title:          firstNonEmpty(strings.TrimSpace(jp.Title), l.Title),
		Company:        firstNonEmpty(strings.TrimSpace(jp.HiringOrganization.Name), e.Company),
		Location:       loc,
		Description:    sanitizeHTML(jp.Description),
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: concludisEmploymentType(jp.EmploymentType),
		PostedAt:       parseRFC3339OrDate(strings.TrimSpace(jp.DatePosted)),
		Countries:      countries,
	}, true
}

// concludisPlaces renders the workplace addresses as "City, Region" (several joined by "; ")
// and collects their countries, which concludis writes as ISO codes ("DE").
func concludisPlaces(places schemaPlaces) (string, []string) {
	var parts, countries []string
	for _, pl := range places {
		if p := joinNonEmpty(pl.Address.AddressLocality, pl.Address.AddressRegion); p != "" && !slices.Contains(parts, p) {
			parts = append(parts, p)
		}
		if c := location.NormalizeCountry(pl.Address.AddressCountry); c != "" && !slices.Contains(countries, c) {
			countries = append(countries, c)
		}
	}
	return strings.Join(parts, "; "), countries
}

// concludisEmploymentType maps employmentType, which concludis writes as one enum or a list
// (["FULL_TIME","TEMPORARY"]). A list names several arrangements at once, so only a single
// value is mapped; a list, and "OTHER", are left to the description parser.
func concludisEmploymentType(raw json.RawMessage) string {
	var one string
	if json.Unmarshal(raw, &one) == nil {
		return schemaEmploymentType(one)
	}
	var many []string
	if json.Unmarshal(raw, &many) == nil && len(many) == 1 {
		return schemaEmploymentType(many[0])
	}
	return ""
}
