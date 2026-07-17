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

// bambooHRPosting is one item from the careers list (no description here); the list
// carries the remote flag, the detail carries the body.
type bambooHRPosting struct {
	ID       string `json:"id"`
	Name     string `json:"jobOpeningName"`
	IsRemote bool   `json:"isRemote"`
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
	return Job{
		ExternalID:  p.ID,
		URL:         jo.ShareURL,
		Title:       p.Name,
		Company:     e.Company,
		Location:    joinNonEmpty(jo.Location.City, jo.Location.State, jo.Location.AddressCountry),
		Description: sanitizeHTML(jo.Description),
		Remote:      p.IsRemote,
		WorkMode:    workModeFromRemote(p.IsRemote),
		PostedAt:    parseDate(jo.DatePosted),
	}, true
}
