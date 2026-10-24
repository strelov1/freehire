package sources

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

// polymer adapts Polymer (polymer.co), an ATS for small startups whose public job board lives
// at https://jobs.polymer.co/<board>. That page sits behind a Cloudflare challenge, but the
// board is rendered from an unauthenticated JSON API on api.polymer.co which is not:
//
//	/v1/hire/organizations/<board>/jobs          the board's open postings, paged
//	/v1/hire/organizations/<board>/jobs/<id>     one posting, with its HTML body
//
// The listing carries every structured field (title, place, a remoteness enum, the countries a
// remote posting is restricted to) but no body, so each posting's body is one detail request.
//
// Both endpoints answer "not found" with HTTP 422 rather than 404: an unknown or disabled board
// as {"errors":{"general":["no careers_page found"]}}, a posting that is gone as "no job found".
type polymer struct {
	http JSONGetter
}

// NewPolymer builds the Polymer adapter over the given HTTP client.
func NewPolymer(c JSONGetter) Source { return polymer{http: c} }

func (polymer) Provider() string { return "polymer" }

// fullBoardListing: the walk ends only on a page the API marks last (or the page count it
// states), and fails unless the distinct postings read reach the stated count. Any page
// failing, a page that adds nothing before the end, and the page cap all fail the crawl.
func (polymer) fullBoardListing() {}

const (
	polymerAPI = "https://api.polymer.co/v1/hire/organizations/"
	// polymerMaxPages bounds the listing walk. The largest board measured (2026-10-24, 61 live
	// boards) holds 11 postings on a single page; the cap only stops a pager that never ends.
	polymerMaxPages = 50
)

// polymerBoard is the shape of a board slug: the organisation's URL name, sometimes with a
// UUID suffix the platform adds to disambiguate a repeated name.
var polymerBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)

// polymerPage is one page of the listing. Count is the board's total number of postings and
// Total the number of PAGES; IsLast is reliable only on a real page — a page past the end
// answers an empty list with is_last false. Count and Total are pointers because the walk's
// completeness check rests on them: decoded as zero when absent, they would let page one
// pass for the whole board.
type polymerPage struct {
	Items *[]polymerPosting `json:"items"`
	Meta  struct {
		Count  *int `json:"count"`
		Total  *int `json:"total"`
		IsLast bool `json:"is_last"`
	} `json:"meta"`
}

// polymerPosting is one listing entry.
type polymerPosting struct {
	ID                  int64    `json:"id"`
	Title               string   `json:"title"`
	Country             string   `json:"country"`
	StateRegion         string   `json:"state_region"`
	City                string   `json:"city"`
	OrganizationName    string   `json:"organization_name"`
	PublishedAt         string   `json:"published_at"`
	Remoteness          string   `json:"remoteness_pretty"`
	ResidencyIsRequired bool     `json:"remote_restriction_country_residency_is_required"`
	ResidencyCountries  []string `json:"remote_restriction_country_list"`
}

// polymerDetail is the slice of a posting's detail we read: the body and the employment enum,
// which the listing carries only as display text.
type polymerDetail struct {
	Description string `json:"description"`
	Kind        string `json:"kind"`
}

func (s polymer) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !polymerBoard.MatchString(board) {
		return nil, fmt.Errorf("polymer: board %q is not an organisation slug", e.Board)
	}
	postings, err := s.list(ctx, board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p polymerPosting) (Job, bool) {
		return s.detail(ctx, board, e, p)
	}), nil
}

func (s polymer) list(ctx context.Context, board string) ([]polymerPosting, error) {
	var out []polymerPosting
	ids := map[int64]bool{}
	for page := 1; ; page++ {
		if page > polymerMaxPages {
			return nil, fmt.Errorf("polymer: no end of the listing after %d pages", polymerMaxPages)
		}
		var resp polymerPage
		if err := s.http.GetJSON(ctx, polymerAPI+board+"/jobs?page="+strconv.Itoa(page), &resp); err != nil {
			if page == 1 && polymerNotFound(err) {
				return nil, fmt.Errorf("polymer: board %q: %w", board, ErrBoardGone)
			}
			return nil, fmt.Errorf("polymer: page %d: %w", page, err)
		}
		// A page without the key is a reshaped answer, not an empty board: read as empty it
		// would let the board-scoped sweep close every posting the board still lists.
		if resp.Items == nil || resp.Meta.Count == nil || resp.Meta.Total == nil {
			return nil, fmt.Errorf("polymer: page %d carries no items/count/total", page)
		}
		count, pages := *resp.Meta.Count, *resp.Meta.Total
		for _, p := range *resp.Items {
			if p.ID != 0 && !ids[p.ID] {
				ids[p.ID] = true
				out = append(out, p)
			}
		}
		if resp.Meta.IsLast || page >= pages {
			// More than the count is fine — a posting published mid-walk shifts the pages.
			// Fewer is a walk that missed some.
			if len(out) < count {
				return nil, fmt.Errorf("polymer: read %d postings, the board states %d", len(out), count)
			}
			return out, nil
		}
		if len(*resp.Items) == 0 {
			return nil, fmt.Errorf("polymer: page %d is empty before the last page (%d of %d)", page, len(out), count)
		}
	}
}

// detail reads one posting's body and maps it onto a Job. A posting the platform says is gone
// is dropped; any other failure to read its body is an unreadableDetail marker, because the
// detail is the only source of the body (see detailUnreadable).
func (s polymer) detail(ctx context.Context, board string, e CompanyEntry, p polymerPosting) (Job, bool) {
	id := strconv.FormatInt(p.ID, 10)
	jobURL := "https://jobs.polymer.co/" + board + "/" + id
	var d polymerDetail
	if err := s.http.GetJSON(ctx, polymerAPI+board+"/jobs/"+id, &d); err != nil {
		if polymerNotFound(err) || !detailUnreadable(err) {
			return Job{}, false
		}
		return unreadableDetail(id, jobURL, e.Company), true
	}
	description := sanitizeHTML(d.Description)
	if description == "" {
		return unreadableDetail(id, jobURL, e.Company), true
	}

	workMode := polymerWorkMode(p.Remoteness)
	return Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          strings.TrimSpace(p.Title),
		Company:        firstNonEmpty(strings.TrimSpace(p.OrganizationName), e.Company),
		Location:       joinNonEmpty(strings.TrimSpace(p.City), strings.TrimSpace(p.StateRegion), strings.TrimSpace(p.Country)),
		Description:    description,
		Remote:         workMode == "remote",
		WorkMode:       workMode,
		EmploymentType: polymerEmploymentType(d.Kind),
		Countries:      polymerCountries(p, workMode),
		PostedAt:       parseRFC3339(p.PublishedAt),
	}, true
}

// polymerNotFound reports the platform's "not found" answer: HTTP 422 (its own spelling of it,
// for a board and a posting alike) or a plain 404.
func polymerNotFound(err error) bool {
	var se *StatusError
	return errors.As(err, &se) && (se.Code == http.StatusUnprocessableEntity || se.Code == http.StatusNotFound)
}

// polymerWorkMode maps the remoteness enum. "Remote friendly" says remote work is possible,
// not which arrangement the posting is, so it stays "" for the description parser to decide.
func polymerWorkMode(remoteness string) string {
	switch strings.ToLower(strings.TrimSpace(remoteness)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "no remote":
		return "onsite"
	}
	return ""
}

// polymerCountries is where the work may be done. A remote posting that requires residency
// names its countries in a list; any other posting's country is its office's. A remote posting
// with no residency rule states no country at all, and its office's would be a guess.
func polymerCountries(p polymerPosting, workMode string) []string {
	if workMode == "remote" {
		if p.ResidencyIsRequired {
			return countriesFromCodes(p.ResidencyCountries)
		}
		return nil
	}
	return countryFromCode(p.Country)
}

// polymerEmploymentType maps the detail's kind enum; "other", "temporary" and the combined
// part-or-full-time value stay "" so the description parser decides.
func polymerEmploymentType(kind string) string {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case "full_time":
		return "full_time"
	case "part_time":
		return "part_time"
	case "contract":
		return "contract"
	case "internship":
		return "internship"
	}
	return ""
}
