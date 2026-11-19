package sources

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"strings"
)

// jobspage adapts Jobspage (jobspage.co), a free careers-page builder and ATS for small
// companies. Each tenant's careers site is a Next.js app at https://<board>.jobspage.co whose
// __NEXT_DATA__ carries every open posting with its structured fields (title, city, ISO country,
// a remote enum, an employment enum). The body there is Draft.js JSON, not HTML, so it comes
// from each posting's own page instead, which server-renders a schema.org JobPosting holding
// the HTML body and the publish date.
//
// The platform answers "no such board" and "no such posting" the same way: a 308 to /404,
// which itself 308s to /404 forever, so the request fails on the redirect limit rather than
// on a status code (jobspageNotFound).
type jobspage struct {
	http jobspageGetter
}

// jobspageGetter is the transport the adapter needs: the listing is read as text for its
// __NEXT_DATA__ script, a posting page as an HTML tree for its JobPosting block.
type jobspageGetter interface {
	TextGetter
	HTMLGetter
}

// NewJobspage builds the Jobspage adapter over the given HTTP client.
func NewJobspage(c jobspageGetter) Source { return jobspage{http: c} }

func (jobspage) Provider() string { return "jobspage" }

// fullBoardListing: the listing is one unpaginated page carrying the board's whole jobs array
// — no loop that could stop early. See the fullBoardListing interface for the bar.
func (jobspage) FullBoardListing() {}

// jobspageNextData is the slice of a listing page's __NEXT_DATA__ we read. JobsData is a
// pointer so a page that lost the key is told apart from a board with no openings, which the
// platform serves as an empty array: read as empty, a reshaped page would close every posting.
type jobspageNextData struct {
	Props struct {
		PageProps struct {
			JobsData *[]jobspagePosting `json:"jobsData"`
		} `json:"pageProps"`
	} `json:"props"`
}

// jobspagePosting is one listing entry. ID is the posting's path segment on the tenant host
// (an 8-character key plus the title slug); a shorter form of it does not resolve.
type jobspagePosting struct {
	ID             string `json:"id"`
	Title          string `json:"jobTitle"`
	City           string `json:"city"`
	Country        string `json:"country"`
	RemoteJob      string `json:"remoteJob"`
	EmploymentType string `json:"employmentType"`
	IsExpired      bool   `json:"isExpired"`
}

// jobspageLDPosting is the slice of a posting page's JobPosting block we read.
type jobspageLDPosting struct {
	Description string `json:"description"`
	DatePosted  string `json:"datePosted"`
}

func (s jobspage) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	body, err := s.http.GetText(ctx, jobspageBoardURL(e.Board))
	if err != nil {
		if jobspageNotFound(err) {
			return nil, fmt.Errorf("jobspage: board %q: %w", e.Board, ErrBoardGone)
		}
		return nil, fmt.Errorf("jobspage: listing %s: %w", e.Board, err)
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return nil, fmt.Errorf("jobspage: no __NEXT_DATA__ in listing %s", e.Board)
	}
	var data jobspageNextData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("jobspage: decode listing %s: %w", e.Board, err)
	}
	if data.Props.PageProps.JobsData == nil {
		return nil, fmt.Errorf("jobspage: no jobsData in listing %s", e.Board)
	}

	var open []jobspagePosting
	for _, p := range *data.Props.PageProps.JobsData {
		if !p.IsExpired {
			open = append(open, p)
		}
	}
	return fetchDetails(open, defaultDetailWorkers, func(p jobspagePosting) (Job, bool) {
		return s.detail(ctx, e, p)
	}), nil
}

// detail reads one posting's page for its body and date and maps it onto a Job. A posting the
// platform says is gone is dropped; any other failure to read its body is an unreadableDetail
// marker, because the page is the only source of the body (see detailUnreadable).
func (s jobspage) detail(ctx context.Context, e CompanyEntry, p jobspagePosting) (Job, bool) {
	id := strings.TrimSpace(p.ID)
	if id == "" {
		return Job{}, false // no id → no address and nothing a close could reach
	}
	jobURL := jobspageBoardURL(e.Board) + "/" + id
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		if jobspageNotFound(err) || !detailUnreadable(err) {
			return Job{}, false
		}
		return unreadableDetail(id, jobURL, e.Company), true
	}
	var ld jobspageLDPosting
	description := ""
	if LDJobPosting(root, &ld) {
		description = sanitizeHTML(ld.Description)
	}
	if description == "" {
		return unreadableDetail(id, jobURL, e.Company), true
	}

	location := jobspageLocation(p.City, p.Country)
	workMode := jobspageWorkMode(p.RemoteJob)
	return Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          strings.TrimSpace(p.Title),
		Company:        e.Company,
		Location:       location,
		Description:    description,
		Remote:         workMode == "remote",
		WorkMode:       workMode,
		EmploymentType: jobspageEmploymentType(p.EmploymentType),
		PostedAt:       parseRFC3339(ld.DatePosted),
	}, true
}

func jobspageBoardURL(board string) string { return "https://" + board + ".jobspage.co" }

// jobspageNotFound reports the platform's "not found" answer: the redirect chain that ends
// looping on /404. Go's client gives up on it with a *url.Error naming the last URL it was
// sent to, so the /404 path is the platform's own verdict rather than a guess from the
// error text.
func jobspageNotFound(err error) bool {
	var ue *url.Error
	if !errors.As(err, &ue) {
		return false
	}
	u, perr := url.Parse(ue.URL)
	return perr == nil && u.Path == "/404"
}

// jobspageLocation joins the city and ISO country the tenant entered, either of which may be
// blank.
func jobspageLocation(city, country string) string {
	var parts []string
	for _, p := range []string{city, country} {
		if p = strings.TrimSpace(p); p != "" {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

// jobspageWorkMode maps the remoteJob enum. "Remote friendly" states that remote work is
// possible, not which arrangement the posting is, so it stays "" for the description parser
// to decide rather than being guessed into hybrid.
func jobspageWorkMode(remoteJob string) string {
	switch strings.ToLower(strings.TrimSpace(remoteJob)) {
	case "remote only":
		return "remote"
	case "no remote":
		return "onsite"
	}
	return ""
}

// jobspageEmploymentType maps the employmentType enum; "Other" and anything unknown stay ""
// so the description parser decides.
func jobspageEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full-time":
		return "full_time"
	case "part-time":
		return "part_time"
	case "contract":
		return "contract"
	case "internship":
		return "internship"
	}
	return ""
}
