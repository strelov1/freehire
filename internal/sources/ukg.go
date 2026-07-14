package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"golang.org/x/net/html"
)

// ukg adapts UKG Pro Recruiting career sites (formerly UltiPro). A board is the JobBoard's
// full path without scheme — "<host>/<tenant>/<guid>" (e.g.
// "recruiting.ultipro.com/van5000vcscu/a46cbdaa-ca2c-49b6-8d2b-e0ceaafa0e25"); the host
// carries the data-residency TLD (.com / .ca / …) so the board id is self-describing, the
// same shape as the Workday adapter. The listing is a keyless POST JSON API
// (LoadSearchResults, Top/Skip pagination); it carries only a brief description, so the full
// body comes from a per-posting detail page that bootstraps the opportunity as an embedded
// JSON literal.
type ukg struct {
	http ukgHTTP
}

// ukgHTTP is the transport ukg needs: a POST-JSON listing plus an HTML detail page whose
// embedded JSON literal carries the full description.
type ukgHTTP interface {
	JSONPoster
	HTMLGetter
}

// NewUKG builds the UKG Pro Recruiting adapter over the given HTTP client.
func NewUKG(c ukgHTTP) Source { return ukg{http: c} }

func (ukg) Provider() string { return "ukg" }

// ukgPageSize is the listing page size. LoadSearchResults reports totalCount, so pagination
// stops once every posting is collected.
const ukgPageSize = 50

// ukgOpportunity is one posting in a LoadSearchResults page. The listing carries a brief
// description only; the full body comes from the detail page.
type ukgOpportunity struct {
	ID               string        `json:"Id"`
	Title            string        `json:"Title"`
	PostedDate       string        `json:"PostedDate"`
	BriefDescription string        `json:"BriefDescription"`
	Locations        []ukgLocation `json:"Locations"`
}

// ukgLocation is one posting location; Address is null for a location named only by its
// localized label.
type ukgLocation struct {
	LocalizedName string `json:"LocalizedName"`
	Address       *struct {
		City  string `json:"City"`
		State *struct {
			Name string `json:"Name"`
		} `json:"State"`
		Country *struct {
			Name string `json:"Name"`
		} `json:"Country"`
	} `json:"Address"`
}

// location renders the posting's first location as "City, State, Country", falling back to
// the localized label when there is no structured address.
func (o ukgOpportunity) location() string {
	if len(o.Locations) == 0 {
		return ""
	}
	loc := o.Locations[0]
	if loc.Address == nil {
		return loc.LocalizedName
	}
	var state, country string
	if loc.Address.State != nil {
		state = loc.Address.State.Name
	}
	if loc.Address.Country != nil {
		country = loc.Address.Country.Name
	}
	if s := joinNonEmpty(loc.Address.City, state, country); s != "" {
		return s
	}
	return loc.LocalizedName
}

func (u ukg) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host, tenant, guid, ok := splitUKGBoard(e.Board)
	if !ok {
		return nil, fmt.Errorf("ukg: board %q must be <host>/<tenant>/<guid>", e.Board)
	}

	opps, err := u.list(ctx, host, tenant, guid)
	if err != nil {
		return nil, err
	}

	// The listing already carries title/location/brief description, so every posting yields
	// a job; the detail fetch only upgrades the brief body to the full one (best-effort).
	return fetchDetails(opps, defaultDetailWorkers, func(o ukgOpportunity) (Job, bool) {
		return u.toJob(ctx, host, tenant, guid, o), true
	}), nil
}

// list pages through LoadSearchResults until every posting is collected. totalCount is
// authoritative; a page that returns no opportunities also stops the loop.
func (u ukg) list(ctx context.Context, host, tenant, guid string) ([]ukgOpportunity, error) {
	url := fmt.Sprintf("https://%s/%s/JobBoard/%s/JobBoardView/LoadSearchResults", host, tenant, guid)
	var opps []ukgOpportunity
	for skip := 0; ; {
		body := map[string]any{"opportunitySearch": map[string]any{"Top": ukgPageSize, "Skip": skip}}
		var page struct {
			Opportunities []ukgOpportunity `json:"opportunities"`
			TotalCount    int              `json:"totalCount"`
		}
		if err := u.http.PostJSON(ctx, url, body, &page); err != nil {
			return nil, fmt.Errorf("ukg: list board %s/%s: %w", tenant, guid, err)
		}
		if len(page.Opportunities) == 0 {
			break
		}
		opps = append(opps, page.Opportunities...)
		skip += len(page.Opportunities)
		if skip >= page.TotalCount {
			break
		}
	}
	return opps, nil
}

// toJob maps a listing opportunity to a Job, upgrading the brief description to the full
// body from the detail page when that fetch succeeds.
func (u ukg) toJob(ctx context.Context, host, tenant, guid string, o ukgOpportunity) Job {
	url := fmt.Sprintf("https://%s/%s/JobBoard/%s/OpportunityDetail?opportunityId=%s", host, tenant, guid, o.ID)
	body := o.BriefDescription
	var employmentType string
	if detail, ok := u.fetchDetail(ctx, url); ok {
		body = detail.Description
		employmentType = ukgEmploymentType(detail.FullTime)
	}
	location := o.location()
	return Job{
		ExternalID:  o.ID,
		URL:         url,
		Title:       o.Title,
		Location:    location,
		Description: sanitizeHTML(html.UnescapeString(body)),
		Remote:      isRemote(location),
		PostedAt:    parseRFC3339(o.PostedDate),
		// FullTime is UKG's structured full/part-time flag; preferred over the free-text
		// employment-type parse.
		EmploymentType: employmentType,
	}
}

// ukgDetail is the parsed subset of the CandidateOpportunityDetail JSON the detail page
// bootstraps: the full HTML description plus the structured full-time flag (a pointer so an
// absent field is distinguishable from false).
type ukgDetail struct {
	Description string `json:"Description"`
	FullTime    *bool  `json:"FullTime"`
}

// fetchDetail fetches a detail page and returns the opportunity's parsed detail (full HTML
// description + FullTime flag), which the page bootstraps as the JSON argument of a
// CandidateOpportunityDetail(...) call. It returns ok=false (so the caller keeps the brief
// body) when the page fetch fails or carries no parseable opportunity.
func (u ukg) fetchDetail(ctx context.Context, url string) (ukgDetail, bool) {
	root, err := u.http.GetHTML(ctx, url)
	if err != nil {
		return ukgDetail{}, false
	}
	script := scriptContaining(root, ukgDetailMarker)
	if script == "" {
		return ukgDetail{}, false
	}
	raw, ok := bracketSlice(script, ukgDetailMarker, '{', '}')
	if !ok {
		return ukgDetail{}, false
	}
	var detail ukgDetail
	if err := json.Unmarshal([]byte(raw), &detail); err != nil || detail.Description == "" {
		return ukgDetail{}, false
	}
	return detail, true
}

// ukgEmploymentType maps UKG's FullTime boolean onto the freehire vocabulary: true →
// full_time, false → part_time. A nil flag (the field is absent) yields "" so the
// description parser decides — structured signal only.
func ukgEmploymentType(fullTime *bool) string {
	switch {
	case fullTime == nil:
		return ""
	case *fullTime:
		return "full_time"
	default:
		return "part_time"
	}
}

// ukgDetailMarker is the bootstrap call whose JSON argument carries the full opportunity
// (including the Description) on a detail page.
const ukgDetailMarker = "CandidateOpportunityDetail("

// splitUKGBoard splits a "<host>/<tenant>/<guid>" board id into its three parts, requiring
// all three to be non-empty.
func splitUKGBoard(board string) (host, tenant, guid string, ok bool) {
	parts := strings.Split(board, "/")
	if len(parts) != 3 || parts[0] == "" || parts[1] == "" || parts[2] == "" {
		return "", "", "", false
	}
	return parts[0], parts[1], parts[2], true
}

// scriptContaining returns the text of the first <script> whose content contains marker, or
// "" when none does. UKG bootstraps the opportunity model inside an inline script.
func scriptContaining(root *html.Node, marker string) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "script" {
			text := textContent(n)
			if strings.Contains(text, marker) {
				found = text
				return false
			}
		}
		return true
	})
	return found
}
