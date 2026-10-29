package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// recruitify adapts Recruitify (recruitify.ai), a Polish ATS used mostly by IT recruitment
// agencies and software houses, whose career portal is an Angular app at
// https://<board>.recruitify.ai. Every portal reads one JSON API, api.recruitify.ai, and the API
// tells portals apart by the request's Origin header alone — the same path answers each tenant
// with its own postings:
//
//	/apply/config               the portal: company name, its locations and tags
//	/apply/project?page=N       one page of open postings (25 a page, no total; an empty page ends)
//	/apply/project/<guid>       one posting with its body
//
// The listing carries no body, so this is a HydratingSource. An unknown portal answers 404
// ("entity not found or not accessible") on the listing; so does a posting that is gone, or one
// that belongs to another portal. The platform serves every label (wildcard DNS), so DNS
// proves nothing about a tenant. A posting's public page is https://<board>.recruitify.ai/job/<guid>.
//
// The listing includes postings whose dateTo has passed: the portal still lists them and still
// takes applications (allowApply), so they are read as open — what the portal shows is the
// platform's own statement of what is open.
type recruitify struct {
	http HeaderJSONGetter
}

// NewRecruitify builds the Recruitify adapter over the given HTTP client.
func NewRecruitify(c HeaderJSONGetter) Source { return recruitify{http: c} }

func (recruitify) Provider() string { return "recruitify" }

// fullBoardListing: the listing is paginated to its natural end (an empty page), and a page that
// fails, or a walk that reaches recruitifyMaxPages first, fails the crawl.
func (recruitify) fullBoardListing() {}

const (
	recruitifyAPI = "https://api.recruitify.ai/apply"
	// recruitifyMaxPages bounds the walk; the largest portal measured on 2026-10-28 (Link Group,
	// 257 postings) needs 11 pages. Needing more than this fails the crawl.
	recruitifyMaxPages = 200
	// recruitifyPoland is the platform's country id for Poland, the only one measured: every
	// posting it marks with it is in a Polish city. Other ids (188 rides on "Remote" postings)
	// are not mapped — a guess at an id would file a posting in the wrong country.
	recruitifyPoland = 139
)

// recruitifyRequestInterval paces every request to api.recruitify.ai, the single host behind
// every portal. Unmeasured, so it takes the conservative side like every unmeasured pace here.
const (
	recruitifyRequestInterval = 500 * time.Millisecond // ~2 req/s
	recruitifyRequestBurst    = 1
)

// pacedRecruitifyGetter wraps the client in the provider-wide limiter.
func pacedRecruitifyGetter(c HeaderJSONGetter) HeaderJSONGetter {
	return rateLimitedHeaderJSONGetter{
		inner:   c,
		limiter: rate.NewLimiter(rate.Every(recruitifyRequestInterval), recruitifyRequestBurst),
	}
}

// recruitifyBoard is the shape of a portal label.
var recruitifyBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// recruitifyPosting is a posting as the listing and the detail both carry it; only the detail
// fills Description.
type recruitifyPosting struct {
	GUID        string `json:"guid"`
	Name        string `json:"name"`
	Published   string `json:"published"`
	Description string `json:"description"`
	Location    []struct {
		Name string `json:"name"`
	} `json:"location"`
	Country  []int `json:"country"`
	IsRemote bool  `json:"isRemote"`
}

// recruitifyHeaders names the portal a request is for; the API answers by Origin alone.
func recruitifyHeaders(board string) map[string]string {
	origin := "https://" + board + ".recruitify.ai"
	return map[string]string{"Origin": origin, "Referer": origin + "/"}
}

func recruitifyPageURL(board, guid string) string {
	return "https://" + board + ".recruitify.ai/job/" + url.PathEscape(guid)
}

// Fetch reads the listing and every posting's detail.
func (s recruitify) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board, postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p recruitifyPosting) (Job, bool) {
		return s.detail(ctx, e, board, p)
	}), nil
}

// FetchNew is Fetch that spends a detail request only on postings the catalogue does not hold;
// a seen posting is re-listed as a SeenRefresh, the listing proving it is still open.
func (s recruitify) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	board, postings, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p recruitifyPosting) (Job, bool) {
		if seen(p.GUID) {
			return Job{ExternalID: p.GUID, URL: recruitifyPageURL(board, p.GUID), Title: strings.TrimSpace(p.Name),
				Company: e.Company, SeenRefresh: true}, true
		}
		return s.detail(ctx, e, board, p)
	}), nil
}

// list walks the portal's listing to its first empty page, dropping postings without the guid
// that keys them. A 404 on the first page is the platform's "no such portal".
func (s recruitify) list(ctx context.Context, rawBoard string) (string, []recruitifyPosting, error) {
	board := strings.ToLower(strings.TrimSpace(rawBoard))
	if !recruitifyBoard.MatchString(board) {
		return "", nil, fmt.Errorf("recruitify: board %q is not a portal label", rawBoard)
	}
	headers := recruitifyHeaders(board)
	seen := map[string]bool{}
	var out []recruitifyPosting
	for page := 1; ; page++ {
		if page > recruitifyMaxPages {
			return "", nil, fmt.Errorf("recruitify: board %s: listing runs past %d pages", board, recruitifyMaxPages)
		}
		var items []recruitifyPosting
		u := recruitifyAPI + "/project?page=" + strconv.Itoa(page)
		if err := s.http.GetJSONWithHeaders(ctx, u, headers, &items); err != nil {
			var se *StatusError
			if page == 1 && errors.As(err, &se) && se.Code == http.StatusNotFound {
				return "", nil, fmt.Errorf("recruitify: board %q: %w", board, ErrBoardGone)
			}
			// Any page failing fails the crawl: a truncated listing would read as closures.
			return "", nil, fmt.Errorf("recruitify: board %s page %d: %w", board, page, err)
		}
		if len(items) == 0 {
			return board, out, nil
		}
		added := 0
		for _, p := range items {
			id := strings.TrimSpace(p.GUID)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			p.GUID = id
			out = append(out, p)
			added++
		}
		if added == 0 {
			// A page that repeats only postings already read is a listing that ignores the
			// page parameter; walking on would never reach the empty page.
			return "", nil, fmt.Errorf("recruitify: board %s page %d repeats earlier postings", board, page)
		}
	}
}

// detail fetches one posting and maps it to a Job. A 404 is the platform's "not found" and
// drops it; any other failure, or an answer that is not this posting, is an unreadableDetail
// marker — the detail is the only source of the body.
func (s recruitify) detail(ctx context.Context, e CompanyEntry, board string, p recruitifyPosting) (Job, bool) {
	pageURL := recruitifyPageURL(board, p.GUID)
	var d recruitifyPosting
	if err := s.http.GetJSONWithHeaders(ctx, recruitifyAPI+"/project/"+url.PathEscape(p.GUID), recruitifyHeaders(board), &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.GUID, pageURL, e.Company), true
		}
		return Job{}, false
	}
	if !strings.EqualFold(strings.TrimSpace(d.GUID), p.GUID) {
		return unreadableDetail(p.GUID, pageURL, e.Company), true
	}
	places := recruitifyPlaces(p)
	j := Job{
		ExternalID:  p.GUID,
		URL:         pageURL,
		Title:       firstNonEmpty(strings.TrimSpace(d.Name), strings.TrimSpace(p.Name)),
		Company:     e.Company,
		Location:    strings.Join(places, "; "),
		Description: sanitizeHTML(d.Description),
		// The listing's timestamp carries its zone; the detail's drops it.
		PostedAt: parseRFC3339(strings.TrimSpace(firstNonEmpty(p.Published, d.Published))),
	}
	// isRemote is a structured flag, but beside a named city it is "remote possible", which
	// says nothing about hybrid versus remote; only a posting naming no place is read as remote.
	if (p.IsRemote || d.IsRemote) && len(places) == 0 {
		j.Remote = true
		j.WorkMode = "remote"
	}
	for _, c := range p.Country {
		if c == recruitifyPoland {
			j.Countries = []string{"pl"}
			break
		}
	}
	return j, true
}

// recruitifyPlaces returns the posting's named places, dropping the platform's "Remote"
// pseudo-location and a French ", Pologne" some portals append to a Polish city.
func recruitifyPlaces(p recruitifyPosting) []string {
	var out []string
	seen := map[string]bool{}
	for _, l := range p.Location {
		n := strings.TrimSpace(strings.TrimSuffix(strings.TrimSpace(l.Name), ", Pologne"))
		if n == "" || strings.EqualFold(n, "remote") || seen[strings.ToLower(n)] {
			continue
		}
		seen[strings.ToLower(n)] = true
		out = append(out, n)
	}
	return out
}
