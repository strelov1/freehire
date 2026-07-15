package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

// alignerr adapts Alignerr's careers site (www.alignerr.com), a single-company source with no
// per-tenant board id (boardless). Each page is a Next.js app embedding its data in the
// __NEXT_DATA__ script: the /jobs listing carries every active posting's id and title (its
// description truncated to a preview), so the full body, dates, and structured fields come
// from each posting's own detail page, fanned out like the other detail-fetching adapters.
type alignerr struct {
	http TextGetter
}

// NewAlignerr builds the Alignerr adapter over the given HTTP client.
func NewAlignerr(c TextGetter) Source { return alignerr{http: c} }

func (alignerr) Provider() string { return "alignerr" }

// alignerr is single-company, so its config entry carries no board.
func (alignerr) boardless() {}

const (
	alignerrListURL = "https://www.alignerr.com/jobs"
	alignerrJobURL  = "https://www.alignerr.com/jobs/"
)

// alignerrNextData is the slice of the __NEXT_DATA__ payload we read: the listing exposes
// initialJobs, a detail page exposes a single job.
type alignerrNextData struct {
	Props struct {
		PageProps struct {
			InitialJobs []alignerrListItem `json:"initialJobs"`
			Job         *alignerrJob       `json:"job"`
		} `json:"pageProps"`
	} `json:"props"`
}

// alignerrListItem is one posting from the listing's initialJobs array; it carries the id used
// to build the detail URL and a location/title fallback for when the detail fetch fails.
type alignerrListItem struct {
	ID       string `json:"id"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

// alignerrJob is the detail page's job record. htmlLongDescription is the full posting body;
// isActive gates closed postings; jobType is the structured employment enum; firstPostDate is
// the publish date (createdAt is the fallback).
type alignerrJob struct {
	ID                  string `json:"id"`
	Name                string `json:"name"`
	HTMLLongDescription string `json:"htmlLongDescription"`
	ShortDescription    string `json:"shortDescription"`
	IsActive            bool   `json:"isActive"`
	FirstPostDate       string `json:"firstPostDate"`
	CreatedAt           string `json:"createdAt"`
	JobType             string `json:"jobType"`
	Location            string `json:"location"`
}

func (a alignerr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	body, err := a.http.GetText(ctx, alignerrListURL)
	if err != nil {
		return nil, fmt.Errorf("alignerr: listing: %w", err)
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return nil, fmt.Errorf("alignerr: no __NEXT_DATA__ in listing")
	}
	var data alignerrNextData
	if err := json.Unmarshal([]byte(raw), &data); err != nil {
		return nil, fmt.Errorf("alignerr: decode listing: %w", err)
	}

	// Each posting's body and structured fields come from its own detail page, fanned out
	// under a bounded pool.
	return fetchDetails(data.Props.PageProps.InitialJobs, defaultDetailWorkers, func(it alignerrListItem) (Job, bool) {
		return a.detail(ctx, e, it)
	}), nil
}

// detail fetches one posting's detail page and maps its job record to a Job, returning ok=false
// when the id is missing, the fetch fails, the page carries no job, or the posting is inactive —
// so the caller skips just that posting.
func (a alignerr) detail(ctx context.Context, e CompanyEntry, it alignerrListItem) (Job, bool) {
	id := strings.TrimSpace(it.ID)
	if id == "" {
		return Job{}, false // no native id → would collide on the dedup key; skip it
	}
	jobURL := alignerrJobURL + id
	body, err := a.http.GetText(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	raw, ok := bracketSlice(body, "__NEXT_DATA__", '{', '}')
	if !ok {
		return Job{}, false
	}
	var data alignerrNextData
	if json.Unmarshal([]byte(raw), &data) != nil {
		return Job{}, false
	}
	j := data.Props.PageProps.Job
	if j == nil || !j.IsActive {
		return Job{}, false
	}

	description := sanitizeHTML(j.HTMLLongDescription)
	if description == "" {
		description = strings.TrimSpace(j.ShortDescription)
	}
	// The listing presents every posting's location as "Remote"; the detail's own location is
	// the fallback when the listing item is absent.
	location := firstNonEmpty(it.Location, j.Location)
	return Job{
		ExternalID:     id,
		URL:            jobURL,
		Title:          firstNonEmpty(strings.TrimSpace(j.Name), strings.TrimSpace(it.Title)),
		Company:        e.Company,
		Location:       location,
		Description:    description,
		Remote:         isRemote(location),
		EmploymentType: alignerrEmploymentType(j.JobType),
		PostedAt:       parseRFC3339(firstNonEmpty(j.FirstPostDate, j.CreatedAt)),
	}, true
}

// alignerrEmploymentType maps Alignerr's jobType enum onto the freehire vocabulary, returning
// "" for an unknown/absent value so the description parser decides.
func alignerrEmploymentType(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "FULL_TIME", "FULLTIME":
		return "full_time"
	case "PART_TIME", "PARTTIME":
		return "part_time"
	case "CONTRACT", "CONTRACTOR", "TEMPORARY":
		return "contract"
	case "INTERN", "INTERNSHIP":
		return "internship"
	default:
		return ""
	}
}
