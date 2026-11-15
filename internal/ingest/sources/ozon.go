package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// ozon adapts Ozon's public job API (job-api.ozon.ru), a boardless single-company source. The
// list endpoint carries no description, so each kept vacancy is fetched in detail. Only
// external_vacancy items are real Ozon postings; the rest are dropped.
type ozon struct {
	http JSONGetter
}

const (
	ozonListURL      = "https://job-api.ozon.ru/v2/vacancy?page=%d&limit=50"
	ozonDetailURL    = "https://job-api.ozon.ru/vacancy/%d"
	ozonVacancyURL   = "https://career.ozon.ru/vacancy/%s/"
	ozonExternalType = "external_vacancy"
	ozonTimeLayout   = "2006-01-02 15:04:05"
)

// NewOzon builds the Ozon adapter over the given HTTP client.
func NewOzon(c JSONGetter) Source { return ozon{http: c} }

func (ozon) Provider() string { return "ozon" }

// ozon is single-company, so its config entries carry no board.
func (ozon) boardless() {}

// ozonItem is one vacancy from the list response (no description here). Only items whose
// VacancyType is external_vacancy are kept.
type ozonItem struct {
	HHID        int64    `json:"hhId"`
	Title       string   `json:"title"`
	WorkFormat  []string `json:"workFormat"`
	City        string   `json:"city"`
	VacancyType string   `json:"vacancyType"`
}

func (o ozon) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := o.list(ctx)
	if err != nil {
		return nil, err
	}

	return fetchDetails(items, defaultDetailWorkers, func(it ozonItem) (Job, bool) {
		return o.detail(ctx, e, it)
	}), nil
}

// list pages through 1..meta.totalPages, keeping only external_vacancy items. The server fixes
// the page size regardless of limit, so totalPages drives the loop.
func (o ozon) list(ctx context.Context) ([]ozonItem, error) {
	var kept []ozonItem
	for page := 1; ; page++ {
		var resp struct {
			Items []ozonItem `json:"items"`
			Meta  struct {
				TotalPages int `json:"totalPages"`
			} `json:"meta"`
		}
		url := fmt.Sprintf(ozonListURL, page)
		if err := o.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("ozon: list page %d: %w", page, err)
		}
		for _, it := range resp.Items {
			if it.VacancyType == ozonExternalType {
				kept = append(kept, it)
			}
		}
		if page >= resp.Meta.TotalPages {
			break
		}
	}
	return kept, nil
}

// detail fetches one vacancy's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller skips just that vacancy.
func (o ozon) detail(ctx context.Context, e CompanyEntry, it ozonItem) (Job, bool) {
	var d struct {
		Name        string   `json:"name"`
		City        string   `json:"city"`
		HHID        int64    `json:"hhId"`
		Descr       string   `json:"descr"`
		Slug        string   `json:"slug"`
		PublishedAt string   `json:"publishedAt"`
		WorkFormat  []string `json:"workFormat"`
	}
	if err := o.http.GetJSON(ctx, fmt.Sprintf(ozonDetailURL, it.HHID), &d); err != nil {
		return Job{}, false
	}

	title := firstNonEmpty(d.Name, it.Title)

	return Job{
		ExternalID:  strconv.FormatInt(it.HHID, 10),
		URL:         fmt.Sprintf(ozonVacancyURL, d.Slug),
		Title:       title,
		Company:     e.Company,
		Location:    d.City,
		Description: sanitizeHTML(d.Descr),
		Remote:      isRemote(strings.Join(d.WorkFormat, " ")),
		PostedAt:    parseLayout(ozonTimeLayout, d.PublishedAt),
	}, true
}
