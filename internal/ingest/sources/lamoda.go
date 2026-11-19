package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
)

// lamoda adapts Lamoda's public job API (job.lamoda.ru), a single-company source with no
// per-tenant board id (boardless). The list endpoint paginates by offset and carries only
// a summary, so each posting's body comes from its own detail request, fanned out.
type lamoda struct {
	http JSONGetter
}

const (
	lamodaListURL    = "https://job.lamoda.ru/api/hr/vacancies/compact"
	lamodaDetailURL  = "https://job.lamoda.ru/api/hr/vacancies/%d"
	lamodaVacancyURL = "https://job.lamoda.ru/vacancies/%s"
	lamodaPageSize   = 100
)

// NewLamoda builds the Lamoda adapter over the given HTTP client.
func NewLamoda(c JSONGetter) Source { return lamoda{http: c} }

func (lamoda) Provider() string { return "lamoda" }

// lamoda is single-company, so its config entries carry no board.
func (lamoda) Boardless() {}

// lamodaItem is one vacancy from the list response (summary only; body comes from detail).
type lamodaItem struct {
	ID       int64  `json:"id"`
	Name     string `json:"name"`
	Slug     string `json:"slug"`
	Location struct {
		Name string `json:"name"`
	} `json:"location"`
	ExternalPublicationDate string `json:"externalPublicationDate"`
}

func (l lamoda) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := l.list(ctx)
	if err != nil {
		return nil, err
	}

	return fetchDetails(items, defaultDetailWorkers, func(it lamodaItem) (Job, bool) {
		return l.detail(ctx, e, it)
	}), nil
}

// list pages through every vacancy via offset (pagination[start]) until start reaches the
// reported total.
func (l lamoda) list(ctx context.Context) ([]lamodaItem, error) {
	var all []lamodaItem
	for start := 0; ; {
		q := url.Values{}
		q.Set("pagination[limit]", strconv.Itoa(lamodaPageSize))
		q.Set("pagination[start]", strconv.Itoa(start))

		var resp struct {
			Data []lamodaItem `json:"data"`
			Meta struct {
				Total int `json:"total"`
			} `json:"meta"`
		}
		u := lamodaListURL + "?" + q.Encode()
		if err := l.http.GetJSON(ctx, u, &resp); err != nil {
			return nil, fmt.Errorf("lamoda: list start %d: %w", start, err)
		}
		all = append(all, resp.Data...)
		// Advance by what the page returned; stop on an empty page (guards a total that never reaches
		// zero) or once every reported posting is collected.
		start += len(resp.Data)
		if len(resp.Data) == 0 || start >= resp.Meta.Total {
			break
		}
	}
	return all, nil
}

// detail fetches one vacancy's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller skips just that vacancy. The body is assembled from
// the four HTML attributes (any may be empty).
func (l lamoda) detail(ctx context.Context, e CompanyEntry, it lamodaItem) (Job, bool) {
	var d struct {
		Data struct {
			Attributes struct {
				Introduction string `json:"introduction"`
				Duties       string `json:"duties"`
				Requirements string `json:"requirements"`
				Conditions   string `json:"conditions"`
				// Retail vacancies leave the four fields above empty and carry the whole
				// body here instead.
				Common string `json:"common"`
			} `json:"attributes"`
		} `json:"data"`
	}
	if err := l.http.GetJSON(ctx, fmt.Sprintf(lamodaDetailURL, it.ID), &d); err != nil {
		return Job{}, false
	}

	a := d.Data.Attributes
	body := a.Introduction + a.Duties + a.Requirements + a.Conditions
	if body == "" {
		body = a.Common
	}

	return Job{
		ExternalID:  strconv.FormatInt(it.ID, 10),
		URL:         fmt.Sprintf(lamodaVacancyURL, it.Slug),
		Title:       it.Name,
		Company:     e.Company,
		Location:    it.Location.Name,
		Description: sanitizeHTML(body),
		Remote:      isRemote(it.Location.Name),
		PostedAt:    parseRFC3339(it.ExternalPublicationDate),
	}, true
}
