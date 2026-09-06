package sources

import (
	"context"
	"fmt"
	"strings"
)

// bambooHR adapts the BambooHR public careers API. Its list endpoint carries no
// description, so it fetches each posting's detail (bounded-concurrency) to assemble the
// body, like the SmartRecruiters and Rippling adapters.
type bambooHR struct {
	http bambooHRHTTP
}

// bambooHRHTTP is the transport surface this adapter needs: the resolved getter for the
// LISTING, whose redirect is the platform's "not serving this board" answer, and the plain
// getter for each posting's detail, where a redirect carries no such meaning.
type bambooHRHTTP interface {
	JSONGetter
	JSONResolvedGetter
}

// NewBambooHR builds the BambooHR adapter over the given HTTP client.
func NewBambooHR(c bambooHRHTTP) Source { return bambooHR{http: c} }

func (bambooHR) Provider() string { return "bamboohr" }

// fullBoardListing: the list request is a single unpaginated call returning the board's whole
// result array — no loop that could stop early. Detail fetches are best-effort per posting.
// See the fullBoardListing interface for the bar.
func (bambooHR) fullBoardListing() {}

// bambooHRBoardGone reports whether a careers-list request was redirected off the board's
// own careers path, and to where. That is how BambooHR says it is not serving a board: the
// request answers 302, the client follows it, and the JSON decode fails on HTML.
//
// The rule is the redirect itself, not a list of destinations, because the destination
// varies with WHY the board is not served and the reasons keep their own vocabulary. All
// four seen in prod on 2026-09-06, across 135 boards, are covered without naming any:
//
//	https://www.bamboohr.com                          no such tenant (an invented board
//	                                                  subdomain lands here too)
//	https://<board>.bamboohr.com/login.php            tenant exists, public board turned off
//	https://<board>.bamboohr.com/settings/account/expired.php   account lapsed
//	https://<board>.bamboohr.com/settings/account/{,temporarily_}suspended
//
// A destination is included in the error rather than folded away: they do not mean the same
// thing to a curator deciding whether to retire, and learning them the first time cost 138
// hand-run probes. "temporarily_suspended" in particular is a board to leave alone.
//
// Staying on the careers path is the success condition, so a redirect BambooHR may add later
// — a host change, a trailing-slash canonicalisation — is not mistaken for a dead board as
// long as it still serves the board. An empty final URL (no response at all) is not a
// verdict: a transport failure must keep reading as one.
func bambooHRBoardGone(board, final string) (string, bool) {
	if final == "" {
		return "", false
	}
	served := fmt.Sprintf("https://%s.bamboohr.com/careers", board)
	if strings.HasPrefix(final, served) {
		return "", false
	}
	return final, true
}

// bambooHRPosting is one item from the careers list (no description here); the list
// carries the work-mode signal, the detail carries the body.
type bambooHRPosting struct {
	ID           string `json:"id"`
	Name         string `json:"jobOpeningName"`
	IsRemote     bool   `json:"isRemote"`
	LocationType string `json:"locationType"`
}

// bambooHRLocationType maps BambooHR's numeric locationType onto our work-mode
// vocabulary. It is the only structured signal the public careers list carries: isRemote
// is null on every posting there, so reading it alone leaves every job mode-less.
func bambooHRLocationType(t string) string {
	switch t {
	case "0":
		return "onsite"
	case "1":
		return "remote"
	case "2":
		return "hybrid"
	default:
		return ""
	}
}

func (b bambooHR) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var list struct {
		Result []bambooHRPosting `json:"result"`
	}
	url := fmt.Sprintf("https://%s.bamboohr.com/careers/list", e.Board)
	final, err := b.http.GetJSONResolved(ctx, url, &list)
	if err != nil {
		// The redirect is checked before the decode error is reported, because the decode
		// error is what the redirect CAUSES: the landing page is HTML, so the failure reads
		// as "invalid character '<'" and says nothing about the board.
		if dest, gone := bambooHRBoardGone(e.Board, final); gone {
			return nil, fmt.Errorf("bamboohr: board %q: %w (redirected to %s)", e.Board, ErrBoardGone, dest)
		}
		return nil, fmt.Errorf("bamboohr: list board %s: %w", e.Board, err)
	}

	// Each posting's description comes from its own detail request, fanned out under a
	// bounded worker pool.
	return fetchDetails(list.Result, defaultDetailWorkers, func(p bambooHRPosting) (Job, bool) {
		return b.detail(ctx, e, p)
	}), nil
}

// detail fetches one posting's detail and maps it to a Job. A posting the platform reports
// gone is dropped (ok=false); one this crawl merely could not read comes back as an
// unreadableDetail marker, since the detail request is this adapter's only source for the
// posting and a dropped one is indistinguishable from a posting taken down.
func (b bambooHR) detail(ctx context.Context, e CompanyEntry, p bambooHRPosting) (Job, bool) {
	url := fmt.Sprintf("https://%s.bamboohr.com/careers/%s/detail", e.Board, p.ID)

	var d struct {
		Result struct {
			JobOpening struct {
				ShareURL    string `json:"jobOpeningShareUrl"`
				Description string `json:"description"`
				DatePosted  string `json:"datePosted"`
				Location    struct {
					City           string `json:"city"`
					State          string `json:"state"`
					AddressCountry string `json:"addressCountry"`
				} `json:"location"`
			} `json:"jobOpening"`
		} `json:"result"`
	}
	if err := b.http.GetJSON(ctx, url, &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(p.ID, url, e.Company), true
		}
		return Job{}, false
	}

	jo := d.Result.JobOpening
	// A 200 is not the same as an answer: an empty body, a `null`, or an interstitial decodes
	// without error and leaves every detail field zero. The LISTING already supplied the id,
	// the name and the location, so such a posting would go on looking read while carrying no
	// URL and no description — counted toward the board's coverage and closed by the sweep all
	// the same. jobOpeningShareUrl is the tell, since every real jobOpening carries one.
	if strings.TrimSpace(jo.ShareURL) == "" {
		return unreadableDetail(p.ID, url, e.Company), true
	}
	location := joinNonEmpty(jo.Location.City, jo.Location.State, jo.Location.AddressCountry)
	mode := firstNonEmpty(bambooHRLocationType(p.LocationType), workModeFromRemote(p.IsRemote))
	return Job{
		ExternalID:  p.ID,
		URL:         jo.ShareURL,
		Title:       p.Name,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(jo.Description),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseDate(jo.DatePosted),
	}, true
}
