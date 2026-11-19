package sources

import (
	"context"
	"fmt"
)

// domclick adapts DomClick's public careers API (career.domclick.ru), a single-company
// source with no per-tenant board id (boardless). The list endpoint carries no
// description, so each vacancy's body comes from its own slug-keyed detail request,
// fanned out like SmartRecruiters.
type domclick struct {
	http JSONGetter
}

const (
	domclickListURL      = "https://career.domclick.ru/api/v1/vacancy/"
	domclickDetailURL    = "https://career.domclick.ru/api/v1/vacancy/detail/%s/"
	domclickVacancyURL   = "https://career.domclick.ru/vacancy/%s"
	domclickRemoteFormat = "REMOTE"
)

// NewDomclick builds the DomClick adapter over the given HTTP client.
func NewDomclick(c JSONGetter) Source { return domclick{http: c} }

func (domclick) Provider() string { return "domclick" }

// domclick is single-company, so its config entries carry no board.
func (domclick) Boardless() {}

// dcItem is one vacancy from the list response. work_format (REMOTE/HYBRID/ON_SITE)
// drives the remote flag without a detail request; the body comes from the detail.
type dcItem struct {
	Slug  string `json:"slug"`
	Title string `json:"title"`
	Area  struct {
		Name string `json:"name"`
	} `json:"area"`
	VacancyContent struct {
		WorkFormat []string `json:"work_format"`
	} `json:"vacancycontent"`
}

func (d domclick) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp struct {
		Result []dcItem `json:"result"`
	}
	if err := d.http.GetJSON(ctx, domclickListURL, &resp); err != nil {
		return nil, fmt.Errorf("domclick: list: %w", err)
	}

	return fetchDetails(resp.Result, defaultDetailWorkers, func(it dcItem) (Job, bool) {
		return d.detail(ctx, e, it)
	}), nil
}

// detail fetches one vacancy's description and maps it to a Job, returning ok=false when
// the fetch fails so the caller skips just that vacancy.
func (d domclick) detail(ctx context.Context, e CompanyEntry, it dcItem) (Job, bool) {
	var resp struct {
		Result struct {
			VacancyContent struct {
				BrandedDescription string `json:"branded_description"`
				Description        string `json:"description"`
			} `json:"vacancycontent"`
		} `json:"result"`
	}
	if err := d.http.GetJSON(ctx, fmt.Sprintf(domclickDetailURL, it.Slug), &resp); err != nil {
		return Job{}, false
	}

	body := firstNonEmpty(resp.Result.VacancyContent.BrandedDescription, resp.Result.VacancyContent.Description)

	return Job{
		ExternalID:  it.Slug,
		URL:         fmt.Sprintf(domclickVacancyURL, it.Slug),
		Title:       it.Title,
		Company:     e.Company,
		Location:    it.Area.Name,
		Description: sanitizeHTML(body),
		Remote:      dcRemote(it.VacancyContent.WorkFormat),
		PostedAt:    nil, // the careers API carries no publish date
	}, true
}

// dcRemote reports whether the list item's work_format marks a remote role. DomClick
// uses an explicit enum (REMOTE/HYBRID/ON_SITE), so this checks for REMOTE rather than
// the text heuristic isRemote.
func dcRemote(formats []string) bool {
	for _, f := range formats {
		if f == domclickRemoteFormat {
			return true
		}
	}
	return false
}
