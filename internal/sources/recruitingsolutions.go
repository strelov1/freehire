package sources

import (
	"context"
	"fmt"
	"strings"
)

// recruitingSolutions adapts career sites built on the recruiting-solutions.org "Career Site
// Builder" platform (e.g. Bertelsmann's createyourowncareer.com). The career page is a client
// rendered SPA, but it is backed by a keyless Azure Cognitive Search endpoint that returns the
// full posting — title, HTML description, the operating company, location and dates — so the
// adapter speaks to that API directly rather than scraping the SPA.
//
// The board is the platform tenant key (its Azure index, sent as the customerId header — e.g.
// "riv-prod" for the Riverty/BFS customer). One tenant can host several operating companies, so
// the company is taken per posting from the record, not from the board entry. The link field
// carries the canonical apply URL, keeping the adapter independent of the tenant's own host.
//
// The index stores each posting once per UI locale (jobId "<id>-de_DE", "<id>-en_GB", …) with an
// identical description, so the adapter dedups by the numeric base id, preferring the English
// locale for the human-facing title and location text.
const (
	recruitingSolutionsAPI      = "https://production.api.recruiting-solutions.org/search"
	recruitingSolutionsPageSize = 50
)

type recruitingSolutions struct {
	http HeaderJSONPoster
}

// NewRecruitingSolutions builds the recruiting-solutions.org adapter over the given HTTP client.
func NewRecruitingSolutions(c HeaderJSONPoster) Source { return recruitingSolutions{http: c} }

func (recruitingSolutions) Provider() string { return "recruitingsolutions" }

// rsSearchResponse is the Azure Cognitive Search reply: the total match count and one page of
// records.
type rsSearchResponse struct {
	Count int     `json:"@odata.count"`
	Value []rsJob `json:"value"`
}

// rsJob is one posting record (one locale of one posting) as the search index returns it.
type rsJob struct {
	JobID        string `json:"jobId"`
	Title        string `json:"title"`
	Company      string `json:"company"`
	Description  string `json:"description"`
	Link         string `json:"link"`
	DatePosted   string `json:"datePosted"`
	TeleComuteID string `json:"teleComuteId"`
	IsActive     bool   `json:"isActive"`
	Country      string `json:"country"`
	Location     struct {
		City            string `json:"city"`
		CountryProvince string `json:"countryProvince"`
	} `json:"location"`
}

func (s recruitingSolutions) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	headers := map[string]string{"customerId": e.Board}

	best := map[string]rsJob{} // base id → chosen locale's record
	var order []string         // base ids in first-seen order

	for skip := 0; ; skip += recruitingSolutionsPageSize {
		body := map[string]any{"count": true, "facets": []any{}, "filter": "", "search": "*", "skip": skip}
		var resp rsSearchResponse
		if err := s.http.PostJSONWithHeaders(ctx, recruitingSolutionsAPI, headers, body, &resp); err != nil {
			return nil, fmt.Errorf("recruitingsolutions: search %s skip %d: %w", e.Board, skip, err)
		}
		if len(resp.Value) == 0 {
			break
		}
		for _, r := range resp.Value {
			if !r.IsActive {
				continue // closed posting in this locale; an active locale (if any) still keeps it
			}
			base := rsBaseID(r.JobID)
			if base == "" {
				continue // no native id → would collide on the dedup key; skip it
			}
			cur, seen := best[base]
			if !seen {
				order = append(order, base)
				best[base] = r
			} else if rsLocaleRank(r.JobID) > rsLocaleRank(cur.JobID) {
				best[base] = r
			}
		}
		if skip+recruitingSolutionsPageSize >= resp.Count {
			break
		}
	}

	jobs := make([]Job, 0, len(order))
	for _, base := range order {
		jobs = append(jobs, recruitingSolutionsJob(base, best[base]))
	}
	return jobs, nil
}

// recruitingSolutionsJob maps a chosen record to a normalized Job under its base id.
func recruitingSolutionsJob(base string, r rsJob) Job {
	mode := rsWorkMode(r.TeleComuteID)
	return Job{
		ExternalID:  base,
		URL:         rsCanonicalURL(r.Link),
		Title:       r.Title,
		Company:     r.Company,
		Location:    joinNonEmpty(r.Location.City, r.Country),
		Description: sanitizeHTML(r.Description),
		Remote:      mode == "remote",
		WorkMode:    mode,
		PostedAt:    parseRFC3339(r.DatePosted),
	}
}

// rsBaseID strips the trailing UI-locale suffix from a record's jobId ("274342-en_GB" → "274342"),
// so the locale copies of one posting dedup to a single job.
func rsBaseID(jobID string) string {
	base, _, _ := strings.Cut(jobID, "-")
	return base
}

// rsLocaleRank ranks a record's locale so dedup keeps the most English-friendly copy for the
// title and location text (the description is identical across locales). Higher wins.
func rsLocaleRank(jobID string) int {
	switch _, lang, _ := strings.Cut(jobID, "-"); lang {
	case "en_GB":
		return 3
	case "en_US":
		return 2
	default:
		return 1
	}
}

// rsWorkMode maps the platform's telecommute enum to our work-mode vocabulary; an unknown value
// yields "".
func rsWorkMode(teleID string) string {
	switch teleID {
	case "telecommute1":
		return "hybrid"
	case "telecommute2":
		return "remote"
	case "telecommute3":
		return "onsite"
	default:
		return ""
	}
}

// rsCanonicalURL drops the locale query from a posting link, so every locale copy yields one
// canonical apply URL.
func rsCanonicalURL(link string) string {
	url, _, _ := strings.Cut(link, "?")
	return url
}
