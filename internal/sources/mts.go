package sources

import (
	"context"
	"errors"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// mts adapts MTS's public careers API (api.job.mts.ru), a single-company source with no
// per-tenant board id (boardless). The API is gated behind a non-secret x-api-key header
// that the public SPA bakes into its Nuxt runtime config, so each run harvests the key from
// job.mts.ru first, then pages the list and fans out per-vacancy detail fetches like the
// other detail-fetching adapters.
// mtsHTTP is the transport mts needs: HTML (to harvest the x-api-key from the SPA) plus
// header-bearing JSON GET/POST for the gated list and detail endpoints.
type mtsHTTP interface {
	HTMLGetter
	HeaderJSONGetter
	HeaderJSONPoster
}

type mts struct {
	http mtsHTTP
}

const (
	mtsSiteURL    = "https://job.mts.ru/"
	mtsListURL    = "https://api.job.mts.ru/v1/vacancies/filtered/career"
	mtsDetailURL  = "https://api.job.mts.ru/v1/vacancy/%s"
	mtsVacancyURL = "https://job.mts.ru/vacancy/%s"
	mtsPageLimit  = 200
)

// NewMTS builds the MTS adapter over the given HTTP client.
func NewMTS(c mtsHTTP) Source { return mts{http: c} }

func (mts) Provider() string { return "mts" }

// mts is single-company, so its config entries carry no board.
func (mts) boardless() {}

// mtsListRequest is the filtered/career POST body; offset paginates to pageInfo.total.
type mtsListRequest struct {
	Offset int `json:"offset"`
	Limit  int `json:"limit"`
}

// mtsVacancyItem is one vacancy from the list response (no body here). id is a string.
type mtsVacancyItem struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	Info struct {
		Brand    string `json:"brand"`
		City     string `json:"city"`
		Worktype string `json:"worktype"`
	} `json:"info"`
}

func (m mts) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	key, err := m.apiKey(ctx)
	if err != nil {
		return nil, err
	}

	items, err := m.list(ctx, key)
	if err != nil {
		return nil, err
	}

	return fetchDetails(items, defaultDetailWorkers, func(it mtsVacancyItem) (Job, bool) {
		return m.detail(ctx, e, key, it)
	}), nil
}

// apiKey harvests the public x-api-key from job.mts.ru's embedded Nuxt runtime config. A
// missing key is an error so the board is isolated (its failure does not abort other boards).
func (m mts) apiKey(ctx context.Context) (string, error) {
	root, err := m.http.GetHTML(ctx, mtsSiteURL)
	if err != nil {
		return "", fmt.Errorf("mts: fetch site for apiKey: %w", err)
	}
	key := mtsExtractAPIKey(root)
	if key == "" {
		return "", errors.New("mts: apiKey not found in site config")
	}
	return key, nil
}

// mtsAPIKeyPattern captures the apiKey value from the SPA's inline runtime config
// (apiKey:"<JWT>"). The value is a JWT (base64url, so dots and -_), held to non-quote chars.
var mtsAPIKeyPattern = regexp.MustCompile(`apiKey:"([^"]+)"`)

// mtsExtractAPIKey returns the apiKey embedded in the page's script text, or "" if absent.
func mtsExtractAPIKey(root *html.Node) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.TextNode {
			if m := mtsAPIKeyPattern.FindStringSubmatch(n.Data); m != nil {
				found = m[1]
				return false
			}
		}
		return true
	})
	return found
}

// list pages through every vacancy (offset += limit until pageInfo.total), sending the
// harvested key on each request.
func (m mts) list(ctx context.Context, key string) ([]mtsVacancyItem, error) {
	headers := map[string]string{"x-api-key": key}
	var items []mtsVacancyItem
	for offset := 0; ; offset += mtsPageLimit {
		var resp struct {
			Data struct {
				Vacancies []mtsVacancyItem `json:"vacancies"`
				PageInfo  struct {
					Total int `json:"total"`
				} `json:"pageInfo"`
			} `json:"data"`
		}
		req := mtsListRequest{Offset: offset, Limit: mtsPageLimit}
		if err := m.http.PostJSONWithHeaders(ctx, mtsListURL, headers, req, &resp); err != nil {
			return nil, fmt.Errorf("mts: list offset %d: %w", offset, err)
		}
		items = append(items, resp.Data.Vacancies...)
		if offset+mtsPageLimit >= resp.Data.PageInfo.Total {
			break
		}
	}
	return items, nil
}

// detail fetches one vacancy's body and maps it to a Job, returning ok=false when the fetch
// fails so the caller skips just that vacancy.
func (m mts) detail(ctx context.Context, e CompanyEntry, key string, it mtsVacancyItem) (Job, bool) {
	headers := map[string]string{"x-api-key": key}
	var resp struct {
		Data struct {
			Vacancy struct {
				DetailText struct {
					DescriptionOfProject string `json:"descriptionOfProject"`
					Description          string `json:"description"`
					Requirements         string `json:"requirements"`
					Conditions           string `json:"conditions"`
				} `json:"detailText"`
			} `json:"vacancy"`
		} `json:"data"`
	}
	if err := m.http.GetJSONWithHeaders(ctx, fmt.Sprintf(mtsDetailURL, it.ID), headers, &resp); err != nil {
		return Job{}, false
	}

	dt := resp.Data.Vacancy.DetailText
	company := firstNonEmpty(it.Info.Brand, e.Company)

	return Job{
		ExternalID:  it.ID,
		URL:         fmt.Sprintf(mtsVacancyURL, it.ID),
		Title:       it.Name,
		Company:     company,
		Location:    it.Info.City,
		Description: sanitizeHTML(dt.DescriptionOfProject + dt.Description + dt.Requirements + dt.Conditions),
		Remote:      isRemote(it.Info.Worktype),
		PostedAt:    nil,
	}, true
}
