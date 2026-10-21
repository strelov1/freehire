package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// dodo adapts Dodo Brands' public careers API (the backend behind dodoteam.ru), a
// single-company source with no per-tenant board id (boardless). The list endpoint
// groups vacancies by speciality and carries no description, so the items are flattened
// and each body is assembled from its detail page's content blocks, fanned out like
// SmartRecruiters.
type dodo struct {
	http JSONGetter
}

const (
	// The careers site moved to dodoteam.ru in 2026; its Nuxt runtime config names this
	// backend (public.apiURL), which serves the same API the retired career-api.dodoteam.ru did.
	dodoListURL   = "https://job-site-backend.dodo-ai-platform.io/api/v1/vacancies"
	dodoDetailURL = "https://job-site-backend.dodo-ai-platform.io/api/v1/pages/vacancy/%d"
	// The API returns no posting URL; this is the site's own vacancy route.
	dodoVacancyURL = "https://dodoteam.ru/vacancy/%d"
)

// dodoBrandCompanies renames the brands the API labels by team rather than by company:
// "Engineering" is Dodo's tech unit, which its site calls Dodo Engineering.
var dodoBrandCompanies = map[string]string{"Engineering": "Dodo Engineering"}

// dodoBodyTypes are the content-block types whose text forms the description body; other
// blocks (vacancy_main, vacancy_image, …) are skipped.
var dodoBodyTypes = map[string]bool{
	"vacancy_text":        true,
	"vacancy_expectation": true,
	"vacancy_you_will":    true,
	"vacancy_benefits":    true,
}

// NewDodo builds the Dodo adapter over the given HTTP client.
func NewDodo(c JSONGetter) Source { return dodo{http: c} }

func (dodo) Provider() string { return "dodo" }

// dodo is single-company, so its config entries carry no board.
func (dodo) boardless() {}

// dodoItem is one vacancy from the (flattened) list response (no description here).
type dodoItem struct {
	ID         int64    `json:"id"`
	Position   string   `json:"position"`
	Brand      string   `json:"brand"`
	Location   string   `json:"vacancy_location"`
	WorkFormat []string `json:"work_format"`
}

func (d dodo) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var resp struct {
		Data []struct {
			Items []dodoItem `json:"items"`
		} `json:"data"`
	}
	if err := d.http.GetJSON(ctx, dodoListURL, &resp); err != nil {
		return nil, fmt.Errorf("dodo: list: %w", err)
	}

	var items []dodoItem
	for _, g := range resp.Data {
		items = append(items, g.Items...)
	}

	return fetchDetails(items, defaultDetailWorkers, func(it dodoItem) (Job, bool) {
		return d.detail(ctx, e, it)
	}), nil
}

// detail fetches one vacancy's content blocks and assembles its body, returning ok=false
// when the fetch fails so the caller skips just that vacancy.
func (d dodo) detail(ctx context.Context, e CompanyEntry, it dodoItem) (Job, bool) {
	var resp struct {
		Data struct {
			Page struct {
				Content []struct {
					Type string `json:"type"`
					Data struct {
						Text string `json:"text"`
					} `json:"data"`
				} `json:"content"`
			} `json:"page"`
		} `json:"data"`
	}
	if err := d.http.GetJSON(ctx, fmt.Sprintf(dodoDetailURL, it.ID), &resp); err != nil {
		return Job{}, false
	}

	var body strings.Builder
	for _, b := range resp.Data.Page.Content {
		if dodoBodyTypes[b.Type] {
			body.WriteString(b.Data.Text)
		}
	}

	company := firstNonEmpty(it.Brand, e.Company)
	if renamed, ok := dodoBrandCompanies[company]; ok {
		company = renamed
	}

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(dodoVacancyURL, it.ID),
		Title:       it.Position,
		Company:     company,
		Location:    it.Location,
		Description: sanitizeHTML(body.String()),
		Remote:      isRemote(strings.Join(it.WorkFormat, " ")),
		PostedAt:    nil, // the careers API carries no publish date
	}, true
}
