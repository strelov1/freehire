package sources

import (
	"context"
	"fmt"
)

// sber adapts Sber's public candidate API (rabota.sber.ru), a single-company source with no
// per-tenant board id (boardless). Unlike the list→detail adapters, the publications endpoint
// returns each vacancy's body inline, so there is no per-vacancy detail request. The feed
// paginates by skip/take. Sber publishes for many employer entities (СберТех, etc.), so each
// vacancy's own company is the employer, with the configured entry as a fallback.
type sber struct {
	http JSONGetter
}

const (
	sberListURL  = "https://rabota.sber.ru/public/app-candidate-public-api-gateway/api/v1/publications?skip=%d&take=%d"
	sberVacURL   = "https://rabota.sber.ru/search/%d"
	sberPageSize = 200
)

// NewSber builds the Sber adapter over the given HTTP client.
func NewSber(c JSONGetter) Source { return sber{http: c} }

func (sber) Provider() string { return "sber" }

// sber is single-company (one configured entry), so its config carries no board.
func (sber) boardless() {}

// sberVac is one publication with its body inline (no detail call).
type sberVac struct {
	RequisitionID   string `json:"requisitionId"`
	InternalID      int64  `json:"internalId"`
	Title           string `json:"title"`
	Company         string `json:"company"`
	City            string `json:"city"`
	PublicationDate string `json:"publicationDate"`
	Introduction    string `json:"introduction"`
	Duties          string `json:"duties"`
	Requirements    string `json:"requirements"`
	Conditions      string `json:"conditions"`
}

func (s sber) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	vacancies, err := s.list(ctx)
	if err != nil {
		return nil, err
	}

	jobs := make([]Job, 0, len(vacancies))
	for _, v := range vacancies {
		jobs = append(jobs, s.toJob(e, v))
	}
	return jobs, nil
}

// list pages through every publication by skip/take (skip += sberPageSize until skip >= total),
// where total comes from the response. Each page carries vacancies with their body inline.
func (s sber) list(ctx context.Context) ([]sberVac, error) {
	var all []sberVac
	for skip := 0; ; skip += sberPageSize {
		var resp struct {
			Data struct {
				Vacancies []sberVac `json:"vacancies"`
				Total     int       `json:"total"`
			} `json:"data"`
		}
		url := fmt.Sprintf(sberListURL, skip, sberPageSize)
		if err := s.http.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("sber: list skip %d: %w", skip, err)
		}
		all = append(all, resp.Data.Vacancies...)
		if skip+sberPageSize >= resp.Data.Total {
			break
		}
	}
	return all, nil
}

// toJob maps an inline vacancy to a Job. The body is assembled from the four text fields
// (any may be empty); the employer company falls back to the configured entry when blank.
// The two ids play distinct roles: the public vacancy page resolves the numeric internalId
// (the requisitionId GUID route 404s), so the URL uses internalId, while the stable
// requisitionId stays the dedup ExternalID.
func (s sber) toJob(e CompanyEntry, v sberVac) Job {
	company := firstNonEmpty(v.Company, e.Company)
	body := v.Introduction + v.Duties + v.Requirements + v.Conditions

	return Job{
		ExternalID:  v.RequisitionID,
		URL:         fmt.Sprintf(sberVacURL, v.InternalID),
		Title:       v.Title,
		Company:     company,
		Location:    v.City,
		Description: sanitizeHTML(body),
		Remote:      isRemote(v.City),
		PostedAt:    parseRFC3339(v.PublicationDate),
	}
}
