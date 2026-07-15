package sources

import (
	"context"
	"fmt"
	"strings"
	"time"
)

// workdayPageLimit is Workday's max listing page size.
const (
	workdayPageLimit = 20
)

// workday adapts Workday's public "CXS" careers API. The board id is the public board
// host and site path, e.g. "ringcentral.wd1.myworkdayjobs.com/RingCentral_Careers";
// the API tenant is the host's first label (here "ringcentral"). The listing endpoint
// is POST-only and carries no description, so it pages the postings and fetches each
// posting's detail (bounded-concurrency) to assemble the description.
// workdayHTTP is the transport workday needs: a POST-only JSON listing plus JSON detail.
type workdayHTTP interface {
	JSONGetter
	JSONPoster
}

type workday struct {
	http workdayHTTP
}

// NewWorkday builds the Workday adapter over the given HTTP client.
func NewWorkday(c workdayHTTP) Source { return workday{http: c} }

func (workday) Provider() string { return "workday" }

// workdayBoard is a configured board parsed into the parts the CXS endpoints need.
type workdayBoard struct {
	host, tenant, site string
}

// parseWorkdayBoard splits "host/site" (e.g. "acme.wd1.myworkdayjobs.com/Careers") into
// the host, the tenant (the host's first label), and the site.
func parseWorkdayBoard(board string) (workdayBoard, error) {
	host, site, ok := strings.Cut(board, "/")
	if !ok || host == "" || site == "" {
		return workdayBoard{}, fmt.Errorf("workday: board %q must be \"host/site\"", board)
	}
	tenant, _, ok := strings.Cut(host, ".")
	if !ok || tenant == "" {
		return workdayBoard{}, fmt.Errorf("workday: board host %q has no tenant label", host)
	}
	return workdayBoard{host: host, tenant: tenant, site: site}, nil
}

// workdayPosting is one item from the jobs listing (no description here).
type workdayPosting struct {
	Title         string `json:"title"`
	ExternalPath  string `json:"externalPath"`
	LocationsText string `json:"locationsText"`
}

func (s workday) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	b, err := parseWorkdayBoard(e.Board)
	if err != nil {
		return nil, err
	}

	postings, err := s.listPostings(ctx, b)
	if err != nil {
		return nil, err
	}

	// Each posting's description comes from its own detail request, fanned out under a
	// bounded worker pool.
	return fetchDetails(postings, defaultDetailWorkers, func(p workdayPosting) (Job, bool) {
		return s.detail(ctx, e, b, p)
	}), nil
}

// listPostings pages through the board's postings via the POST-only jobs endpoint,
// stopping when a page is empty or all postings reported by total have been collected.
func (s workday) listPostings(ctx context.Context, b workdayBoard) ([]workdayPosting, error) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s/jobs", b.host, b.tenant, b.site)
	var postings []workdayPosting
	// Some boards (e.g. pg.wd5.myworkdayjobs.com) report the real total only on the
	// first page and total:0 thereafter, so latch the first non-zero total and page
	// against it — reading each page's total would break after page one and silently
	// truncate the board, which the 48h unseen sweep then reads as postings removed.
	total := -1
	for offset := 0; ; {
		reqBody := map[string]any{
			"appliedFacets": map[string]any{},
			"limit":         workdayPageLimit,
			"offset":        offset,
			"searchText":    "",
		}
		var page struct {
			Total       int              `json:"total"`
			JobPostings []workdayPosting `json:"jobPostings"`
		}
		if err := s.http.PostJSON(ctx, url, reqBody, &page); err != nil {
			return nil, fmt.Errorf("workday: list board %s: %w", b.site, err)
		}
		if len(page.JobPostings) == 0 {
			break
		}
		postings = append(postings, page.JobPostings...)
		offset += len(page.JobPostings)
		if total < 0 && page.Total > 0 {
			total = page.Total
		}
		if total >= 0 && offset >= total {
			break
		}
	}
	return postings, nil
}

// detail fetches one posting's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that posting.
func (s workday) detail(ctx context.Context, e CompanyEntry, b workdayBoard, p workdayPosting) (Job, bool) {
	url := fmt.Sprintf("https://%s/wday/cxs/%s/%s%s", b.host, b.tenant, b.site, p.ExternalPath)

	var d struct {
		JobPostingInfo struct {
			Title          string `json:"title"`
			JobDescription string `json:"jobDescription"`
			Location       string `json:"location"`
			StartDate      string `json:"startDate"`
			ExternalURL    string `json:"externalUrl"`
			RemoteType     string `json:"remoteType"`
			TimeType       string `json:"timeType"`
		} `json:"jobPostingInfo"`
	}
	if err := s.http.GetJSON(ctx, url, &d); err != nil {
		return Job{}, false
	}
	info := d.JobPostingInfo

	title := firstNonEmpty(strings.TrimSpace(info.Title), strings.TrimSpace(p.Title))
	location := firstNonEmpty(info.Location, p.LocationsText)
	jobURL := info.ExternalURL
	if jobURL == "" {
		jobURL = fmt.Sprintf("https://%s/%s%s", b.host, b.site, p.ExternalPath)
	}
	remote := isRemote(location) || strings.Contains(strings.ToLower(info.RemoteType), "remote")

	return Job{
		ExternalID:  p.ExternalPath,
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(info.JobDescription),
		Remote:      remote,
		PostedAt:    parseWorkdayDate(info.StartDate),
		// timeType is Workday's structured full/part-time enum; the pipeline prefers it
		// over the free-text employment-type parse. Workday's timeType only distinguishes
		// full vs part time (contract/intern live in other, per-tenant fields).
		EmploymentType: workdayEmploymentType(info.TimeType),
	}, true
}

// workdayEmploymentType maps Workday's timeType ("Full time" / "Part time") onto the
// freehire employment-type vocabulary, returning "" for any other/absent value so the
// description parser decides instead — structured signal only, never a guess.
func workdayEmploymentType(timeType string) string {
	switch strings.TrimSpace(strings.ToLower(timeType)) {
	case "full time":
		return "full_time"
	case "part time":
		return "part_time"
	default:
		return ""
	}
}

// parseWorkdayDate reads Workday's startDate, which may be a full RFC3339 timestamp or
// a date-only value, returning nil for anything unparseable (posted_at is nullable).
func parseWorkdayDate(s string) *time.Time {
	if t := parseRFC3339(s); t != nil {
		return t
	}
	return parseDate(s)
}
