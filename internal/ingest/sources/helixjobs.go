package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// helixjobs adapts helixjobs.com, the job portal Perbility runs for the employers on its
// concludis ATS. The portal names an employer by its concludis instance id (iid), and a
// posting by "<iid>P<position id>" — the position id is the one the employer's own
// <tenant>.concludis.de list shows (Vitanas: iid 147, concludis position 130755, portal
// posting 147P130755). The portal reaches the employers the concludis adapter cannot: it lists
// 590 iids and 25,021 postings (2026-10-25), against 23 tenants whose concludis label is known,
// because a tenant's label is on no page the portal serves. So the board is the iid.
//
// The list is https://helixjobs.com/_/joblist?iid=<board>&page=N, server-rendered, 25 lines a
// page, stating its extent ("Suchergebnis: 62 Treffer"); a page past the end carries no line,
// and an iid with no posting — or none at all, the portal answers both alike — states
// "Keine Ergebnisse gefunden". A line carries the title and the posting address
// /_/jobad?prj=<id>; the posting page carries a schema.org JobPosting in the concludis shape
// (concludisJobPosting), with the employer, the workplace and the employment type. So this is a
// HydratingSource.
//
// A posting the portal relays to another platform redirects there (iid 500000's postings land
// on a P&I LOGA site); off the portal there is no JobPosting this adapter reads, so it is
// dropped, the same rule the concludis adapter keeps for postings redirected off its tenant.
type helixjobs struct {
	http HTMLResolvedGetter
}

// NewHelixjobs builds the helixjobs adapter over the given HTTP client.
func NewHelixjobs(c HTMLResolvedGetter) Source { return helixjobs{http: c} }

func (helixjobs) Provider() string { return "helixjobs" }

// fullBoardListing: the walk fails unless it collects exactly the number of postings the list
// states, and its page cap is a ceiling that fails the crawl rather than ending it.
func (helixjobs) fullBoardListing() {}

const helixjobsHost = "helixjobs.com"

// helixjobsMaxPages is a safety ceiling on the ?page=N walk. The largest iid measured (95, 2,899
// postings at 25 a page) needs 116; reaching the ceiling fails the crawl.
const helixjobsMaxPages = 400

var (
	// helixjobsBoard is the shape of an iid.
	helixjobsBoard = regexp.MustCompile(`^[1-9][0-9]*$`)
	// helixjobsPosting captures a posting id from a line's address. The portal spells the
	// separator either case (harvested links carry 100007p36), so it is folded to "P".
	helixjobsPosting = regexp.MustCompile(`^([1-9][0-9]*)[Pp]([0-9]+)$`)
	// helixjobsStated reads the list's stated extent from its result line.
	helixjobsStated = regexp.MustCompile(`([0-9][0-9.]*)\s*Treffer`)
)

// helixjobsNoResults is the list's statement that the iid has no posting.
const helixjobsNoResults = "Keine Ergebnisse gefunden"

// helixjobsLine is one line of the list.
type helixjobsLine struct {
	ID, URL, Title string
}

func helixjobsPostingURL(id string) string {
	return "https://" + helixjobsHost + "/_/jobad?prj=" + id
}

// Fetch walks the list and reads every posting's page.
func (s helixjobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	lines, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(lines, defaultDetailWorkers, func(l helixjobsLine) (Job, bool) {
		return s.detail(ctx, e, l)
	}), nil
}

// FetchNew is Fetch that spends a page request only on postings the catalogue does not hold; a
// seen posting is re-listed as a SeenRefresh, the list itself proving it is still open.
func (s helixjobs) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	lines, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(lines, defaultDetailWorkers, func(l helixjobsLine) (Job, bool) {
		if seen(l.ID) {
			return Job{ExternalID: l.ID, URL: l.URL, Title: l.Title, Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, l)
	}), nil
}

// list walks the list pages. ANY page failing fails the board, and so does a walk that ends
// without collecting the stated number of postings: a short walk is a truncation the unseen
// sweep must never read as closures.
func (s helixjobs) list(ctx context.Context, rawBoard string) ([]helixjobsLine, error) {
	board := strings.TrimSpace(rawBoard)
	if !helixjobsBoard.MatchString(board) {
		return nil, fmt.Errorf("helixjobs: board %q is not an instance id", rawBoard)
	}
	var lines []helixjobsLine
	seen := map[string]bool{}
	want := -1
	for page := 1; page <= helixjobsMaxPages; page++ {
		u := fmt.Sprintf("https://%s/_/joblist?iid=%s&page=%d", helixjobsHost, board, page)
		root, final, err := s.http.GetHTMLResolved(ctx, u)
		if err != nil {
			return nil, fmt.Errorf("helixjobs: board %s page %d: %w", board, page, err)
		}
		if concludisLeftTenant(final, u) {
			return nil, fmt.Errorf("helixjobs: board %s list redirected to %s", board, final)
		}
		if want < 0 {
			if want, err = helixjobsStatedCount(root); err != nil {
				return nil, fmt.Errorf("helixjobs: board %s: %w", board, err)
			}
			if want == 0 {
				return nil, nil
			}
		}
		added := 0
		for _, l := range helixjobsLines(root, board) {
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
			return nil, fmt.Errorf("helixjobs: board %s: list ended at %d of the %d postings it states", board, len(lines), want)
		}
		if page == helixjobsMaxPages {
			return nil, fmt.Errorf("helixjobs: board %s: %d pages without reaching the %d postings it states", board, page, want)
		}
	}
	if len(lines) != want {
		return nil, fmt.Errorf("helixjobs: board %s: list holds %d postings, states %d", board, len(lines), want)
	}
	return lines, nil
}

// helixjobsStatedCount reads the number of postings the list states. A page stating neither a
// count nor the no-results notice is a reshaped page, not an empty board.
func helixjobsStatedCount(root *html.Node) (int, error) {
	if res := firstByClass(root, "result-text"); res != nil {
		text := strings.Join(strings.Fields(textContent(res)), " ")
		m := firstSubmatch(helixjobsStated, text)
		if m == "" {
			return 0, fmt.Errorf("list page states %q, not a posting count", text)
		}
		return strconv.Atoi(strings.ReplaceAll(m, ".", ""))
	}
	if strings.Contains(textContent(root), helixjobsNoResults) {
		return 0, nil
	}
	return 0, fmt.Errorf("list page carries no posting count")
}

// helixjobsLines reads every line of a list page: the posting id from the line's address and
// the title from its heading. A line whose posting belongs to another iid is skipped — the
// board's own list never carries one, so it is not this board's.
func helixjobsLines(root *html.Node, board string) []helixjobsLine {
	var lines []helixjobsLine
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "li" || !HasClass(n, "joblist-item") {
			return true
		}
		walk(n, func(a *html.Node) bool {
			if a.Type != html.ElementNode || a.Data != "a" {
				return true
			}
			id := helixjobsPostingID(Attr(a, "href"))
			if id == "" || !strings.HasPrefix(id, board+"P") {
				return true
			}
			l := helixjobsLine{ID: id, URL: helixjobsPostingURL(id)}
			if h := firstByTag(a, "h3"); h != nil {
				l.Title = strings.TrimSpace(textContent(h))
			}
			lines = append(lines, l)
			return false
		})
		return false
	})
	return lines
}

// helixjobsPostingID reads the canonical posting id ("146P13181") from a /_/jobad address, or
// "" when the address names no posting.
func helixjobsPostingID(href string) string {
	u, err := url.Parse(href)
	if err != nil || u.Path != "/_/jobad" {
		return ""
	}
	m := helixjobsPosting.FindStringSubmatch(u.Query().Get("prj"))
	if m == nil {
		return ""
	}
	return m[1] + "P" + m[2]
}

// detail fetches one posting page and maps its JobPosting, with the list line, to a Job. A 404
// is the portal's own "no such posting" and drops it; any other failure, or a portal page
// carrying no JobPosting body, is an unreadableDetail marker — the page is the only source of
// the body.
func (s helixjobs) detail(ctx context.Context, e CompanyEntry, l helixjobsLine) (Job, bool) {
	root, final, err := s.http.GetHTMLResolved(ctx, l.URL)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(l.ID, l.URL, e.Company), true
		}
		return Job{}, false
	}
	var jp concludisJobPosting
	if !ldJobPosting(root, &jp) || strings.TrimSpace(jp.Description) == "" {
		// A posting relayed to another platform is not on any page this adapter reads, and
		// never will be — a marker would recur every run and hold the board's sweep shut.
		if concludisLeftTenant(final, l.URL) {
			return Job{}, false
		}
		return unreadableDetail(l.ID, l.URL, e.Company), true
	}
	// No sampled posting (119, 2026-10-25) states jobLocationType; TELECOMMUTE is still read as
	// the remote statement schema.org defines it to be, and its absence says nothing.
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
