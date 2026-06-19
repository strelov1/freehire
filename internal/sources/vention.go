package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"golang.org/x/net/html"
)

// vention adapts Vention's careers site (join.ventionteams.com), the iTechArt rebrand. Its
// pages render no JobPosting ld+json, but the Wagtail CMS backing the SPA exposes a keyless
// JSON API (/api/v2/pages/) whose vacancy entries carry the full posting inline — so one
// paged list sweep is the whole catalogue, no per-job detail fetch. The board is the host.
type vention struct {
	http JSONGetter
}

// NewVention builds the Vention adapter over the given HTTP client.
func NewVention(c JSONGetter) Source { return vention{http: c} }

func (vention) Provider() string { return "vention" }

const (
	ventionPageSize = 20
	ventionMaxPages = 100
)

// ventionVacancy is one Wagtail page of type join_vacancy.VacancyPage. The description is
// assembled from the HTML body blocks; location from the nested city/country.
type ventionVacancy struct {
	ID    int64  `json:"id"`
	Title string `json:"title"`
	Meta  struct {
		Slug             string `json:"slug"`
		HTMLURL          string `json:"html_url"`
		FirstPublishedAt string `json:"first_published_at"`
	} `json:"meta"`
	City struct {
		Name    string `json:"name"`
		Country struct {
			Name string `json:"name"`
		} `json:"country"`
	} `json:"city"`
	WorkFormat       string `json:"work_format"`
	Responsibilities string `json:"responsibilities"`
	RequiredSkills   string `json:"required_skills"`
	Benefits         string `json:"benefits"`
	About            string `json:"about"`
}

func (v vention) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	base := fmt.Sprintf("https://%s/api/v2/pages/?type=join_vacancy.VacancyPage&fields=*&limit=%d", e.Board, ventionPageSize)

	var items []ventionVacancy
	for page := 0; page < ventionMaxPages; page++ {
		var resp struct {
			Meta struct {
				TotalCount int `json:"total_count"`
			} `json:"meta"`
			Items []ventionVacancy `json:"items"`
		}
		url := fmt.Sprintf("%s&offset=%d", base, page*ventionPageSize)
		if err := v.http.GetJSON(ctx, url, &resp); err != nil {
			if page == 0 {
				return nil, fmt.Errorf("vention: list %s: %w", e.Board, err)
			}
			break
		}
		if len(resp.Items) == 0 {
			break
		}
		items = append(items, resp.Items...)
		if (page+1)*ventionPageSize >= resp.Meta.TotalCount {
			break
		}
	}

	jobs := make([]Job, 0, len(items))
	for _, it := range items {
		if it.ID == 0 || it.Title == "" {
			continue
		}
		location := joinNonEmpty(it.City.Name, it.City.Country.Name)
		jobs = append(jobs, Job{
			ExternalID:  strconv.FormatInt(it.ID, 10),
			URL:         ventionURL(e.Board, it),
			Title:       it.Title,
			Company:     e.Company,
			Location:    location,
			Description: sanitizeHTML(html.UnescapeString(it.Responsibilities + it.RequiredSkills + it.About + it.Benefits)),
			Remote:      it.WorkFormat == "remote" || isRemote(location),
			WorkMode:    ventionWorkMode(it.WorkFormat),
			PostedAt:    parseRFC3339(it.Meta.FirstPublishedAt),
		})
	}
	return jobs, nil
}

func ventionURL(board string, it ventionVacancy) string {
	if it.Meta.HTMLURL != "" {
		return it.Meta.HTMLURL
	}
	return fmt.Sprintf("https://%s/job-openings/%s", board, it.Meta.Slug)
}

// ventionWorkMode maps Vention's work_format ("remote"/"*_hybrid"/"*_onsite") to the
// structured work-mode signal, leaving "" for an unrecognized value.
func ventionWorkMode(format string) string {
	switch {
	case format == "remote":
		return "remote"
	case strings.Contains(format, "hybrid"):
		return "hybrid"
	case strings.Contains(format, "onsite") || strings.Contains(format, "office"):
		return "onsite"
	default:
		return ""
	}
}
