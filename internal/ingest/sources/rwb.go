package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// rwb adapts Wildberries' public career API (career.rwb.ru), a single-company source with
// no per-tenant board id (boardless). The list endpoint paginates by offset and carries the
// city and employment types but no body, so each vacancy's description comes from its own
// detail request, fanned out like SmartRecruiters.
type rwb struct {
	http JSONGetter
}

const (
	rwbListURL    = "https://career.rwb.ru/crm-api/api/v1/pub/vacancies?limit=%d&offset=%d"
	rwbDetailURL  = "https://career.rwb.ru/crm-api/api/v1/pub/vacancies/%d"
	rwbVacancyURL = "https://career.rwb.ru/vacancies/%d"
	rwbPageSize   = 50 // The API returns at most 50 even when range.limit echoes a larger request.
)

// NewRWB builds the Wildberries adapter over the given HTTP client.
func NewRWB(c JSONGetter) Source { return rwb{http: c} }

func (rwb) Provider() string { return "rwb" }

// rwb is single-company, so its config entries carry no board.
func (rwb) Boardless() {}

// rwbItem is one vacancy from the list response (no description here). The list carries the
// city and employment types, which drive location and the remote flag.
type rwbItem struct {
	ID              int64  `json:"id"`
	Name            string `json:"name"`
	CityTitle       string `json:"city_title"`
	EmploymentTypes []struct {
		Title string `json:"title"`
	} `json:"employment_types"`
}

func (r rwb) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := r.list(ctx)
	if err != nil {
		return nil, err
	}

	return fetchDetails(items, defaultDetailWorkers, func(it rwbItem) (Job, bool) {
		return r.detail(ctx, e, it)
	}), nil
}

// list pages through every vacancy by offset (offset += rwbPageSize until offset >= count),
// where count comes from the response's range. Each page carries list-level items only.
func (r rwb) list(ctx context.Context) ([]rwbItem, error) {
	var all []rwbItem
	for offset := 0; ; offset += rwbPageSize {
		var resp struct {
			Data struct {
				Items []rwbItem `json:"items"`
				Range struct {
					Count int `json:"count"`
				} `json:"range"`
			} `json:"data"`
		}
		url := fmt.Sprintf(rwbListURL, rwbPageSize, offset)
		if err := r.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("rwb: list offset %d: %w", offset, err)
		}
		all = append(all, resp.Data.Items...)
		if offset+rwbPageSize >= resp.Data.Range.Count {
			break
		}
	}
	return all, nil
}

// detail fetches one vacancy's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller skips just that vacancy. Location and the remote flag
// come from the list item (the detail body keys city differently); the body is assembled
// from the detail's description plus its three bullet arrays in order.
func (r rwb) detail(ctx context.Context, e CompanyEntry, it rwbItem) (Job, bool) {
	var d struct {
		Data struct {
			Description     string   `json:"description"`
			DutiesArr       []string `json:"duties_arr"`
			RequirementsArr []string `json:"requirements_arr"`
			ConditionsArr   []string `json:"conditions_arr"`
		} `json:"data"`
	}
	if err := r.http.GetJSON(ctx, fmt.Sprintf(rwbDetailURL, it.ID), &d); err != nil {
		return Job{}, false
	}

	var body strings.Builder
	body.WriteString(d.Data.Description)
	for _, arr := range [][]string{d.Data.DutiesArr, d.Data.RequirementsArr, d.Data.ConditionsArr} {
		for _, p := range arr {
			body.WriteString("<p>" + p + "</p>")
		}
	}

	employment := make([]string, 0, len(it.EmploymentTypes))
	for _, et := range it.EmploymentTypes {
		employment = append(employment, et.Title)
	}

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(rwbVacancyURL, it.ID),
		Title:       it.Name,
		Company:     e.Company,
		Location:    it.CityTitle,
		Description: sanitizeHTML(body.String()),
		Remote:      isRemote(strings.Join(employment, " ")),
		PostedAt:    nil, // the career feed carries no publish date
	}, true
}
