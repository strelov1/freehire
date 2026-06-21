package sources

import (
	"context"
	"fmt"
	"strings"
)

// senior adapts Senior's "Portal de Talentos" recruiting platform (the careersmanager
// candidate bridge API on platform.senior.com.br). The board id is the tenant subdomain
// (e.g. "intelbras"). The API is keyless and POST-only in three steps: resolve the tenant
// subdomain to a profile id, page the vacancy search filtered by that profile, then fetch
// each vacancy's detail for its description (the search listing carries no description).
const (
	seniorBase     = "https://platform.senior.com.br/t/senior.com.br/bridge/1.0/anonymous/rest/hcm/careersmanagercandidate"
	seniorPageSize = 50
)

// seniorHTTPClient is the transport senior needs: the whole flow is POST-only JSON.
type seniorHTTPClient interface {
	JSONPoster
}

type senior struct {
	http seniorHTTPClient
}

// NewSenior builds the Senior Portal de Talentos adapter over the given HTTP client.
func NewSenior(c seniorHTTPClient) Source { return senior{http: c} }

func (senior) Provider() string { return "senior" }

// seniorContent is one item from the vacancy search listing (no description here).
type seniorContent struct {
	Vacancy struct {
		ID           string `json:"id"`
		Title        string `json:"title"`
		Localization struct {
			City     string `json:"city"`
			Province string `json:"province"`
			Country  string `json:"country"`
		} `json:"localization"`
		JobModel    []string `json:"jobModel"`
		Publication struct {
			StartDate string `json:"startDate"`
		} `json:"publication"`
	} `json:"vacancy"`
}

func (s senior) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	profileID, err := s.resolveProfileID(ctx, e.Board)
	if err != nil {
		return nil, err
	}

	contents, err := s.searchVacancies(ctx, profileID)
	if err != nil {
		return nil, err
	}

	// Each vacancy's description comes from its own detail request, fanned out under a
	// bounded worker pool; a failed detail skips just that vacancy.
	return fetchDetails(contents, defaultDetailWorkers, func(c seniorContent) (Job, bool) {
		return s.detail(ctx, e, c)
	}), nil
}

// resolveProfileID maps the tenant subdomain to its profile id; a non-tenant subdomain
// fails (a bad board, not an empty one), aborting the board.
func (s senior) resolveProfileID(ctx context.Context, board string) (string, error) {
	url := seniorBase + "/actions/getProfileIdBySubdomain"
	var res struct {
		ProfileID string `json:"profileId"`
	}
	if err := s.http.PostJSON(ctx, url, map[string]any{"subdomain": board}, &res); err != nil {
		return "", fmt.Errorf("senior: resolve subdomain %q: %w", board, err)
	}
	if res.ProfileID == "" {
		return "", fmt.Errorf("senior: subdomain %q has no profile id", board)
	}
	return res.ProfileID, nil
}

// searchVacancies pages through the profile's vacancies via the POST-only search endpoint,
// stopping once every reported page has been collected (page is 0-based).
func (s senior) searchVacancies(ctx context.Context, profileID string) ([]seniorContent, error) {
	url := seniorBase + "/queries/searchVacancies"
	var contents []seniorContent
	for page := 0; ; page++ {
		reqBody := map[string]any{
			"page":   page,
			"size":   seniorPageSize,
			"filter": "",
			"match": map[string]any{
				"companies":     []map[string]any{{"id": profileID}},
				"localizations": []any{},
			},
		}
		var res struct {
			TotalPages int             `json:"totalPages"`
			Contents   []seniorContent `json:"contents"`
		}
		if err := s.http.PostJSON(ctx, url, reqBody, &res); err != nil {
			return nil, fmt.Errorf("senior: search profile %s: %w", profileID, err)
		}
		contents = append(contents, res.Contents...)
		if page+1 >= res.TotalPages {
			break
		}
	}
	return contents, nil
}

// detail fetches one vacancy's detail and maps it to a Job, returning ok=false when the
// detail request fails so the caller can skip just that vacancy.
func (s senior) detail(ctx context.Context, e CompanyEntry, c seniorContent) (Job, bool) {
	url := seniorBase + "/queries/findVacancyById"
	var d struct {
		Vacancy struct {
			About struct {
				Description string `json:"description"`
			} `json:"about"`
		} `json:"vacancy"`
	}
	if err := s.http.PostJSON(ctx, url, map[string]any{"id": c.Vacancy.ID}, &d); err != nil {
		return Job{}, false
	}

	loc := c.Vacancy.Localization
	location := joinNonEmpty(loc.City, loc.Province, loc.Country)
	remote := seniorHasModel(c.Vacancy.JobModel, "REMOTE")

	return Job{
		ExternalID:  c.Vacancy.ID,
		URL:         fmt.Sprintf("https://%s.portaldetalentos.senior.com.br/vacancy/%s", e.Board, c.Vacancy.ID),
		Title:       c.Vacancy.Title,
		Company:     e.Company,
		Location:    location,
		Description: sanitizeHTML(d.Vacancy.About.Description),
		Remote:      remote,
		PostedAt:    parseDate(c.Vacancy.Publication.StartDate),
		WorkMode:    seniorWorkMode(c.Vacancy.JobModel),
	}, true
}

// seniorWorkMode maps Senior's jobModel enum to our work-mode vocabulary, preferring the
// most-remote signal when a vacancy lists several models; an empty or unknown set yields "".
func seniorWorkMode(models []string) string {
	switch {
	case seniorHasModel(models, "REMOTE"):
		return "remote"
	case seniorHasModel(models, "HYBRID"):
		return "hybrid"
	case seniorHasModel(models, "IN_PERSON"):
		return "onsite"
	default:
		return ""
	}
}

// seniorHasModel reports whether models contains the given jobModel enum (case-insensitive).
func seniorHasModel(models []string, want string) bool {
	for _, m := range models {
		if strings.EqualFold(strings.TrimSpace(m), want) {
			return true
		}
	}
	return false
}
