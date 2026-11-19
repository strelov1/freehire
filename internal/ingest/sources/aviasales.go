package sources

import (
	"context"
	"fmt"
	"strconv"
)

// aviasales adapts Aviasales' public careers API (vacancies-app.aviasales.ru), a
// single-company source with no per-tenant board id (boardless). The list endpoint is a
// plain JSON array carrying no description, so each vacancy's body comes from its own
// Remix-loader detail request, fanned out like SmartRecruiters.
type aviasales struct {
	http JSONGetter
}

const (
	aviasalesListURL = "https://vacancies-app.aviasales.ru/api/vacancies"
	// The loader's (id) segment is literal text (Remix route id), not the numeric id;
	// the parens are percent-encoded so http.NewRequest sends them unchanged.
	aviasalesDetailURL  = "https://vacancies-app.aviasales.ru/about/vacancies/%d?__loader=about/vacancies/%%28id%%29/page&__ssrDirect=true"
	aviasalesVacancyURL = "https://www.aviasales.ru/about/vacancies/%d"
)

// NewAviasales builds the Aviasales adapter over the given HTTP client.
func NewAviasales(c JSONGetter) Source { return aviasales{http: c} }

func (aviasales) Provider() string { return "aviasales" }

// aviasales is single-company, so its config entries carry no board.
func (aviasales) Boardless() {}

// avItem is one vacancy from the list response (no description here).
type avItem struct {
	ID        int64  `json:"id"`
	Position  string `json:"position"`
	WorkPlace string `json:"workPlace"`
}

func (a aviasales) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var items []avItem
	if err := a.http.GetJSON(ctx, aviasalesListURL, &items); err != nil {
		return nil, fmt.Errorf("aviasales: list: %w", err)
	}

	return fetchDetails(items, defaultDetailWorkers, func(it avItem) (Job, bool) {
		return a.detail(ctx, e, it)
	}), nil
}

// detail fetches one vacancy's loader payload and maps it to a Job, returning ok=false
// when the fetch fails so the caller skips just that vacancy.
func (a aviasales) detail(ctx context.Context, e CompanyEntry, it avItem) (Job, bool) {
	var d struct {
		Vacancy struct {
			Description  string `json:"description"`
			Todo         string `json:"todo"`
			Requirements string `json:"requirements"`
			Conditions   string `json:"conditions"`
			WorkPlace    string `json:"workPlace"`
		} `json:"vacancy"`
	}
	if err := a.http.GetJSON(ctx, fmt.Sprintf(aviasalesDetailURL, it.ID), &d); err != nil {
		return Job{}, false
	}
	v := d.Vacancy

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(aviasalesVacancyURL, it.ID),
		Title:       it.Position,
		Company:     e.Company,
		Location:    it.WorkPlace,
		Description: sanitizeHTML(v.Description + v.Todo + v.Requirements + v.Conditions),
		Remote:      isRemote(it.WorkPlace),
		PostedAt:    nil, // the careers API carries no publish date
	}, true
}
