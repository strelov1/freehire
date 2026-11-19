package sources

import (
	"context"
	"fmt"
	"strconv"
)

// mtslink adapts MTS Link's public careers API (mts-link.ru/api/huntflow), a
// single-company source with no per-tenant board id (boardless). The list endpoint
// returns every vacancy with empty body fields, so each open vacancy's body comes from
// its own detail request, fanned out like SmartRecruiters. Only OPEN vacancies are kept.
type mtslink struct {
	http JSONGetter
}

const (
	mtslinkListURL    = "https://mts-link.ru/api/huntflow/vacancies"
	mtslinkDetailURL  = "https://mts-link.ru/api/huntflow/vacancy/%d"
	mtslinkVacancyURL = "https://mts-link.ru/vacancies/%d/"
	mtslinkOpenState  = "OPEN"
	mtslinkTimeLayout = "2006-01-02 15:04:05.000000"
)

// NewMtslink builds the MTS Link adapter over the given HTTP client.
func NewMtslink(c JSONGetter) Source { return mtslink{http: c} }

func (mtslink) Provider() string { return "mtslink" }

// mtslink is single-company, so its config entries carry no board.
func (mtslink) Boardless() {}

// mtsItem is one vacancy from the list response (body fields are empty here). Only items
// whose State is OPEN are kept.
type mtsItem struct {
	ID       int64  `json:"id"`
	Position string `json:"position"`
	State    string `json:"state"`
}

func (m mtslink) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var items []mtsItem
	if err := m.http.GetJSON(ctx, mtslinkListURL, &items); err != nil {
		return nil, fmt.Errorf("mtslink: list: %w", err)
	}

	open := make([]mtsItem, 0, len(items))
	for _, it := range items {
		if it.State == mtslinkOpenState {
			open = append(open, it)
		}
	}

	return fetchDetails(open, defaultDetailWorkers, func(it mtsItem) (Job, bool) {
		return m.detail(ctx, e, it)
	}), nil
}

// detail fetches one vacancy's body and maps it to a Job, returning ok=false when the
// fetch fails so the caller skips just that vacancy.
func (m mtslink) detail(ctx context.Context, e CompanyEntry, it mtsItem) (Job, bool) {
	var d struct {
		Body         string `json:"body"`
		Requirements string `json:"requirements"`
		Conditions   string `json:"conditions"`
		WorkFormat   string `json:"workFormat"`
		Created      struct {
			Date string `json:"date"`
		} `json:"created"`
	}
	if err := m.http.GetJSON(ctx, fmt.Sprintf(mtslinkDetailURL, it.ID), &d); err != nil {
		return Job{}, false
	}

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(mtslinkVacancyURL, it.ID),
		Title:       it.Position,
		Company:     e.Company,
		Location:    "", // the careers API carries no city field
		Description: sanitizeHTML(d.Body + d.Requirements + d.Conditions),
		Remote:      isRemote(d.WorkFormat),
		PostedAt:    parseLayout(mtslinkTimeLayout, d.Created.Date),
	}, true
}
