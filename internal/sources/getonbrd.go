package sources

import (
	"context"
	"fmt"
	"strings"
	"sync"
)

// getonbrd adapts getonbrd.com (Get on Board), a LatAm tech job board. Boardless (one
// public API, no per-tenant board) and multi-company, so it stays in the source facet and
// takes each posting's company from the API. The public API has no global jobs endpoint
// (it requires auth), but the per-category endpoints are open, so the adapter enumerates
// the fixed category set, de-duplicates jobs across categories, and resolves each posting's
// company name from the company endpoint (cached per run, since companies repeat).
type getonbrd struct {
	http JSONGetter
}

const (
	getonbrdCategoriesURL = "https://www.getonbrd.com/api/v0/categories"
	getonbrdJobsURL       = "https://www.getonbrd.com/api/v0/categories/%s/jobs?per_page=%d&page=%d"
	getonbrdCompanyURL    = "https://www.getonbrd.com/api/v0/companies/%d"
	getonbrdJobURL        = "https://www.getonbrd.com/jobs/%s"
	getonbrdPageSize      = 50
	// getonbrdMaxPages bounds per-category pagination as a runaway guard.
	getonbrdMaxPages = 100
)

// NewGetonbrd builds the Get on Board adapter over the given HTTP client.
func NewGetonbrd(c JSONGetter) Source { return getonbrd{http: c} }

func (getonbrd) Provider() string { return "getonbrd" }

func (getonbrd) boardless() {}

func (getonbrd) aggregator() {}

// getonbrdJob is one job resource from the JSON:API list. The company is a relationship
// reference (an id only); its name comes from a separate company lookup.
type getonbrdJob struct {
	ID         string `json:"id"`
	Attributes struct {
		Title          string   `json:"title"`
		Description    string   `json:"description"`
		Functions      string   `json:"functions"`
		Benefits       string   `json:"benefits"`
		RemoteModality string   `json:"remote_modality"`
		Countries      []string `json:"countries"`
		PublishedAt    int64    `json:"published_at"`
		Company        struct {
			Data struct {
				ID int64 `json:"id"`
			} `json:"data"`
		} `json:"company"`
	} `json:"attributes"`
}

func (s getonbrd) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	cats, err := s.categories(ctx)
	if err != nil {
		return nil, err
	}

	// De-duplicate jobs across categories (a posting can be tagged in several).
	seen := make(map[string]bool)
	var raw []getonbrdJob
	for _, cat := range cats {
		for page := 1; page <= getonbrdMaxPages; page++ {
			var resp struct {
				Data []getonbrdJob `json:"data"`
				Meta struct {
					TotalPages int `json:"total_pages"`
				} `json:"meta"`
			}
			url := fmt.Sprintf(getonbrdJobsURL, cat, getonbrdPageSize, page)
			if err := s.http.GetJSON(ctx, url, &resp); err != nil {
				return nil, fmt.Errorf("getonbrd: category %s page %d: %w", cat, page, err)
			}
			for _, j := range resp.Data {
				if !seen[j.ID] {
					seen[j.ID] = true
					raw = append(raw, j)
				}
			}
			if page >= resp.Meta.TotalPages || len(resp.Data) == 0 {
				break
			}
		}
	}

	companies := s.resolveCompanies(ctx, raw)

	jobs := make([]Job, 0, len(raw))
	for _, j := range raw {
		if job, ok := j.toJob(companies[j.Attributes.Company.Data.ID]); ok {
			jobs = append(jobs, job)
		}
	}
	return jobs, nil
}

// categories returns the open category ids the jobs endpoints are keyed by.
func (s getonbrd) categories(ctx context.Context) ([]string, error) {
	var resp struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := s.http.GetJSON(ctx, getonbrdCategoriesURL, &resp); err != nil {
		return nil, fmt.Errorf("getonbrd: categories: %w", err)
	}
	cats := make([]string, 0, len(resp.Data))
	for _, c := range resp.Data {
		cats = append(cats, c.ID)
	}
	return cats, nil
}

// resolveCompanies fetches the name of every distinct company id referenced by the jobs,
// concurrently under a bounded pool, returning an id→name map. A lookup that fails leaves
// the id absent, so the posting falls back to dropping (no company would break the slug).
func (s getonbrd) resolveCompanies(ctx context.Context, jobs []getonbrdJob) map[int64]string {
	ids := make(map[int64]bool)
	for _, j := range jobs {
		if id := j.Attributes.Company.Data.ID; id != 0 {
			ids[id] = true
		}
	}

	names := make(map[int64]string, len(ids))
	var mu sync.Mutex
	var wg sync.WaitGroup
	sem := make(chan struct{}, defaultDetailWorkers)
	for id := range ids {
		wg.Add(1)
		sem <- struct{}{}
		go func(id int64) {
			defer wg.Done()
			defer func() { <-sem }()
			var resp struct {
				Data struct {
					Attributes struct {
						Name string `json:"name"`
					} `json:"attributes"`
				} `json:"data"`
			}
			if err := s.http.GetJSON(ctx, fmt.Sprintf(getonbrdCompanyURL, id), &resp); err != nil || resp.Data.Attributes.Name == "" {
				return
			}
			mu.Lock()
			names[id] = resp.Data.Attributes.Name
			mu.Unlock()
		}(id)
	}
	wg.Wait()
	return names
}

// toJob maps a job resource to a Job, returning ok=false for an unusable posting (no id, or
// no resolved company which would break the slug). remote_modality is the structured work
// mode; the description assembles the functions/requirements/benefits HTML sections.
func (j getonbrdJob) toJob(company string) (Job, bool) {
	if j.ID == "" || company == "" {
		return Job{}, false
	}
	a := j.Attributes
	var b strings.Builder
	for _, section := range []string{a.Functions, a.Description, a.Benefits} {
		if strings.TrimSpace(section) != "" {
			b.WriteString(section)
		}
	}
	workMode := getonbrdWorkMode(a.RemoteModality)
	return Job{
		ExternalID:  j.ID,
		URL:         fmt.Sprintf(getonbrdJobURL, j.ID),
		Title:       a.Title,
		Company:     company,
		Location:    strings.Join(a.Countries, ", "),
		Description: sanitizeHTML(b.String()),
		Remote:      workMode == "remote" || workMode == "hybrid",
		WorkMode:    workMode,
		PostedAt:    parseEpochSeconds(a.PublishedAt),
	}, true
}

// getonbrdWorkMode maps Get on Board's remote_modality to the controlled work-mode vocab.
func getonbrdWorkMode(modality string) string {
	switch modality {
	case "fully_remote", "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "no_remote", "on_site", "onsite":
		return "onsite"
	default:
		return ""
	}
}
