package sources

import (
	"context"
	"fmt"
)

// bambooHR adapts the BambooHR public careers API. Its list endpoint carries no
// description, so it fetches each posting's detail (bounded-concurrency) to assemble the
// body, like the SmartRecruiters and Rippling adapters.
type bambooHR struct {
	http JSONGetter
}

// NewBambooHR builds the BambooHR adapter over the given HTTP client.
func NewBambooHR(c JSONGetter) Source { return bambooHR{http: c} }

func (bambooHR) Provider() string { return "bamboohr" }

// fullBoardListing: the list request is a single unpaginated call returning the board's whole
// result array — no loop that could stop early. Detail fetches are best-effort per posting.
// See the fullBoardListing interface for the bar.
func (bambooHR) fullBoardListing() {}

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
	if err := b.http.GetJSON(ctx, url, &list); err != nil {
		return nil, fmt.Errorf("bamboohr: list board %s: %w", e.Board, err)
	}

	// Each posting's description comes from its own detail request, fanned out under a
	// bounded worker pool.
	return fetchDetails(list.Result, defaultDetailWorkers, func(p bambooHRPosting) (Job, bool) {
		return b.detail(ctx, e, p)
	}), nil
}

// detail fetches one posting's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that posting.
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
		return Job{}, false
	}

	jo := d.Result.JobOpening
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
