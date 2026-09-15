package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// recruiterflow adapts RecruiterFlow agency career pages (recruiterflow.com/<board>/jobs).
// The board is the agency's own path segment; a RecruiterFlow board IS a recruiting agency
// (its ld+json hiringOrganization names the agency itself, never a per-posting end-client —
// confirmed live), the same single-tenant-per-board shape huntflow's agency/hub boards
// already have. The listing page is a legacy jQuery SPA with no server-rendered job links
// and no discoverable XHR endpoint — the whole board is instead embedded as a bare
// JavaScript variable assignment, `window.jobsList = {...};`, found only by reading the raw
// page HTML rather than by capturing network traffic. Its "department" key groups every open
// posting; flattening across every group yields the whole board. Each posting's own detail
// page carries a standard schema.org ld+json JobPosting block for the description.
type recruiterflow struct {
	http recruiterflowHTTP
}

// recruiterflowHTTP is the transport recruiterflow needs: the raw-text listing page plus
// the HTML detail page.
type recruiterflowHTTP interface {
	TextGetter
	HTMLGetter
}

// NewRecruiterflow builds the RecruiterFlow adapter over the given text+HTML client.
func NewRecruiterflow(c recruiterflowHTTP) Source { return recruiterflow{http: c} }

func (recruiterflow) Provider() string { return "recruiterflow" }

// fullBoardListing: the board has no pagination mechanism at all — the whole listing is
// embedded in one page load, so there is no further page a truncated read could ever miss.
// A detail-fetch failure for one posting becomes an Unreadable marker rather than a silent
// drop, the same contract a listing-then-detail adapter like recrutei/humanbit already
// gives. A listing fetch or decode failure fails the whole Fetch outright.
func (recruiterflow) fullBoardListing() {}

func (s recruiterflow) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(items, defaultDetailWorkers, func(it recruiterflowItem) (Job, bool) {
		return s.detail(ctx, e, it)
	}), nil
}

// recruiterflowItem is one listing posting, from the embedded window.jobsList.
type recruiterflowItem struct {
	ApplyLink      string `json:"apply_link"`
	Details        string `json:"details"`
	EmploymentType string `json:"employment_type"`
	JobID          int    `json:"job_id"`
	JobName        string `json:"job_name"`
	LastOpened     string `json:"last_opened"`
	RemoteType     string `json:"remote_type"`
}

// recruiterflowDeptGroup is one "[name, items[]]" pair from window.jobsList's "department"
// array — a heterogeneous 2-element tuple, not a plain object, so it needs its own decode.
type recruiterflowDeptGroup struct {
	Items []recruiterflowItem
}

func (g *recruiterflowDeptGroup) UnmarshalJSON(b []byte) error {
	var pair [2]json.RawMessage
	if err := json.Unmarshal(b, &pair); err != nil {
		return err
	}
	return json.Unmarshal(pair[1], &g.Items)
}

type recruiterflowJobsList struct {
	Department []recruiterflowDeptGroup `json:"department"`
}

// list fetches the board's listing page and extracts its embedded window.jobsList,
// flattening every department group into one item list.
func (s recruiterflow) list(ctx context.Context, board string) ([]recruiterflowItem, error) {
	url := fmt.Sprintf("https://recruiterflow.com/%s/jobs", board)
	page, err := s.http.GetText(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("recruiterflow: board %q: %w", board, err)
	}
	raw, ok := bracketSlice(page, "window.jobsList = ", '{', '}')
	if !ok {
		return nil, fmt.Errorf("recruiterflow: board %q: no window.jobsList found", board)
	}
	var jl recruiterflowJobsList
	if err := json.Unmarshal([]byte(raw), &jl); err != nil {
		return nil, fmt.Errorf("recruiterflow: board %q: decode window.jobsList: %w", board, err)
	}
	var items []recruiterflowItem
	for _, dept := range jl.Department {
		items = append(items, dept.Items...)
	}
	return items, nil
}

// detail fetches one posting's page and maps it (with its listing fields) to a Job. A
// transport failure that states nothing about the posting yields an Unreadable marker,
// since the detail page is this adapter's only source for the description; a page that
// answers but carries no JobPosting ld+json block drops the posting instead.
func (s recruiterflow) detail(ctx context.Context, e CompanyEntry, it recruiterflowItem) (Job, bool) {
	id := strconv.Itoa(it.JobID)
	url := "https://recruiterflow.com/" + it.ApplyLink

	root, err := s.http.GetHTML(ctx, url)
	if err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, url, e.Company), true
		}
		return Job{}, false
	}
	var ld struct {
		Description string `json:"description"`
	}
	if !ldJobPosting(root, &ld) {
		return unreadableDetail(id, url, e.Company), true
	}

	return Job{
		ExternalID:     id,
		URL:            url,
		Title:          strings.TrimSpace(it.JobName),
		Company:        e.Company,
		Location:       it.Details,
		Description:    sanitizeHTML(ld.Description),
		Remote:         workplaceTypeMode(it.RemoteType) == "remote" || isRemote(it.JobName+" "+it.Details),
		WorkMode:       workplaceTypeMode(it.RemoteType),
		EmploymentType: recruiterflowEmploymentType(it.EmploymentType),
		PostedAt:       parseRFC3339(it.LastOpened),
	}, true
}

// recruiterflowEmploymentType maps RecruiterFlow's employment_type onto
// vocab.EmploymentTypeValues. "Full time"/"Part time"/"Contract" are confirmed live; the
// defensive "Internship" sibling follows this codebase's usual mapping for a self-evident
// business-vocabulary field (e.g. humanbitEmploymentType), not a guess at an opaque code.
func recruiterflowEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full time", "full-time", "fulltime":
		return "full_time"
	case "part time", "part-time", "parttime":
		return "part_time"
	case "contract", "contractor", "temporary", "freelance":
		return "contract"
	case "internship", "intern":
		return "internship"
	default:
		return ""
	}
}
