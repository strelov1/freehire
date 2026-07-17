package sources

import (
	"context"
	"fmt"
	"strings"
)

// alfabank adapts Alfa-Bank's public job API (job.alfabank.ru), a single-company source
// with no per-tenant board id (boardless). The list endpoint paginates by skip/take and
// carries the description inline, so no detail fan-out is needed. cityId is a code resolved
// against a cities dictionary fetched once per Fetch.
type alfabank struct {
	http JSONGetter
}

const (
	alfaListURL   = "https://job.alfabank.ru/api/vacancies?skip=%d&take=%d"
	alfaCitiesURL = "https://job.alfabank.ru/api/vacancies/options?listId=cities"
	alfaBaseURL   = "https://job.alfabank.ru"
	alfaPageSize  = 100
)

// NewAlfaBank builds the Alfa-Bank adapter over the given HTTP client.
func NewAlfaBank(c JSONGetter) Source { return alfabank{http: c} }

func (alfabank) Provider() string { return "alfabank" }

// alfabank is single-company, so its config entries carry no board.
func (alfabank) boardless() {}

// alfaItem is one vacancy from the list response (description inline).
type alfaItem struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	CityID      string `json:"cityId"`
	Slug        string `json:"slug"`
	CreatedAt   string `json:"createdAt"`
	Description string `json:"description"`
}

func (a alfabank) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	cities, err := a.cities(ctx)
	if err != nil {
		return nil, err
	}

	items, err := a.list(ctx)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(items))
	for _, it := range items {
		location := cities[it.CityID] // empty when the cityId is unknown
		jobs = append(jobs, Job{
			ExternalID:  it.ID,
			URL:         alfaBaseURL + it.Slug,
			Title:       it.Name,
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(it.Description),
			Remote:      strings.Contains(it.Slug, "/remote-job/") || isRemote(location),
			PostedAt:    parseRFC3339(it.CreatedAt),
		})
	}
	return jobs, nil
}

// cities fetches the cityId -> city-name dictionary once per Fetch.
func (a alfabank) cities(ctx context.Context) (map[string]string, error) {
	var resp struct {
		OptionLists struct {
			Cities []struct {
				ID   string `json:"id"`
				Text string `json:"text"`
			} `json:"cities"`
		} `json:"optionLists"`
	}
	if err := a.http.GetJSON(ctx, alfaCitiesURL, &resp); err != nil {
		return nil, fmt.Errorf("alfabank: cities: %w", err)
	}
	m := make(map[string]string, len(resp.OptionLists.Cities))
	for _, c := range resp.OptionLists.Cities {
		m[c.ID] = c.Text
	}
	return m, nil
}

// list pages through every vacancy via skip/take until skip reaches the reported total.
func (a alfabank) list(ctx context.Context) ([]alfaItem, error) {
	var all []alfaItem
	for skip := 0; ; skip += alfaPageSize {
		var resp struct {
			Total int        `json:"total"`
			Items []alfaItem `json:"items"`
		}
		url := fmt.Sprintf(alfaListURL, skip, alfaPageSize)
		if err := a.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("alfabank: list skip %d: %w", skip, err)
		}
		all = append(all, resp.Items...)
		// Stop at the reported total, but also on any empty page: a Total larger than the rows
		// actually served (a cap/filter artifact) would otherwise spin forever, since skip never
		// reaches it and every further page comes back empty.
		if len(resp.Items) == 0 || skip+alfaPageSize >= resp.Total {
			break
		}
	}
	return all, nil
}
