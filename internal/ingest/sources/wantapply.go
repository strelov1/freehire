package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/dict/location"
)

// wantapply reads the public listing used by WantApply's frontend. Bodies are inline, so a
// crawl needs one request per page rather than one per vacancy. The global and Cyprus sites
// are independent catalogues; neither domain may stand in for the other.
type wantapply struct {
	http     JSONGetter
	host     string
	provider string
}

const (
	// The frontend caps this endpoint at 50, even when a larger limit is requested.
	wantapplyPageSize = 50
	wantapplyMaxPages = 1000
)

func NewWantapply(c JSONGetter) Source {
	return wantapply{http: c, host: "wantapply.com", provider: "wantapply"}
}
func NewWantapplyCy(c JSONGetter) Source {
	return wantapply{http: c, host: "wantapply.cy", provider: "wantapply-cy"}
}
func (s wantapply) Provider() string { return s.provider }
func (wantapply) Boardless()         {}
func (wantapply) Aggregator()        {}

// Every page must be read, and the unique count must match the API's total, before Fetch
// succeeds. A partial listing never licenses the source-wide unseen sweep.
func (wantapply) FullCatalog() {}

func (s wantapply) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	filters := map[string]any{"domain": "tech"}
	if s.provider == "wantapply-cy" {
		filters = map[string]any{"jobLocations": []string{"CYP"}}
	}
	encoded, _ := json.Marshal(filters)
	var jobs []Job
	seen := make(map[string]bool)
	total := -1
	for page := 1; page <= wantapplyMaxPages; page++ {
		q := url.Values{"page": {strconv.Itoa(page)}, "limit": {strconv.Itoa(wantapplyPageSize)}, "filters": {string(encoded)}}
		endpoint := "https://" + s.host + "/api/jobs?" + q.Encode()
		var response struct {
			Data        []wantapplyPosting `json:"data"`
			HasNextPage *bool              `json:"hasNextPage"`
			Total       *int               `json:"total"`
		}
		if err := s.http.GetJSON(ctx, endpoint, &response); err != nil {
			return nil, fmt.Errorf("%s: page %d: %w", s.provider, page, err)
		}
		if response.Data == nil || response.HasNextPage == nil || response.Total == nil || *response.Total < 0 {
			return nil, fmt.Errorf("%s: page %d: listing or pagination evidence absent", s.provider, page)
		}
		if total < 0 {
			total = *response.Total
		} else if *response.Total != total {
			return nil, fmt.Errorf("%s: catalogue total changed during pagination", s.provider)
		}
		for _, p := range response.Data {
			j, ok := p.toJob(s.host)
			if !ok {
				return nil, fmt.Errorf("%s: unusable published vacancy %q", s.provider, p.URL)
			}
			if seen[j.ExternalID] {
				return nil, fmt.Errorf("%s: repeated vacancy %q during pagination", s.provider, j.ExternalID)
			}
			seen[j.ExternalID] = true
			jobs = append(jobs, j)
		}
		if !*response.HasNextPage {
			if len(jobs) != total {
				return nil, fmt.Errorf("%s: incomplete listing: read %d of %d vacancies", s.provider, len(jobs), total)
			}
			return jobs, nil
		}
		if len(response.Data) == 0 || len(jobs) >= total {
			return nil, fmt.Errorf("%s: pagination claims more pages without remaining vacancies", s.provider)
		}
	}
	return nil, fmt.Errorf("%s: page limit reached before catalogue end", s.provider)
}

type wantapplyPosting struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
	CompanyName string `json:"companyName"`
	Company     struct {
		Name string `json:"name"`
	} `json:"company"`
	Remote          bool     `json:"remote"`
	WorkplaceTypes  []string `json:"workplaceTypes"`
	EmploymentTypes []string `json:"employmentTypes"`
	CreatedAt       string   `json:"createdAt"`
	PublishedAt     string   `json:"publishedAt"`
	ExpirationDate  string   `json:"expirationDate"`
	Status          string   `json:"status"`
	JobLocations    []struct {
		ISO2 string `json:"iso2"`
		Name string `json:"name_en"`
	} `json:"jobLocations"`
}

func (p wantapplyPosting) toJob(host string) (Job, bool) {
	rawURL := "https://" + host + "/" + strings.TrimLeft(p.URL, "/")
	u, err := url.Parse(rawURL)
	if err != nil {
		return Job{}, false
	}
	slug := strings.Trim(u.Path, "/")
	company := firstNonEmpty(strings.TrimSpace(p.Company.Name), strings.TrimSpace(p.CompanyName))
	if slug == "" || strings.Contains(slug, "/") || strings.TrimSpace(p.Title) == "" || company == "" || p.Status != "published" {
		return Job{}, false
	}
	j := Job{ExternalID: slug, URL: rawURL, Title: strings.TrimSpace(p.Title), Company: company,
		Description: sanitizeHTML(unescapeEncodedHTML(p.Description)), Remote: p.Remote,
		PostedAt: parseRFC3339(p.PublishedAt)}
	if j.PostedAt == nil {
		j.PostedAt = parseRFC3339(p.CreatedAt)
	}
	if deadline, err := time.Parse(time.RFC3339Nano, p.ExpirationDate); err == nil {
		deadline = deadline.UTC()
		j.ApplicationDeadline = &deadline
	}
	var places []string
	countries := make(map[string]bool)
	for _, c := range p.JobLocations {
		country := location.NormalizeCountry(c.ISO2)
		if country != "" && !countries[country] {
			countries[country] = true
			j.Countries = append(j.Countries, country)
			places = append(places, firstNonEmpty(c.Name, c.ISO2))
		}
	}
	j.Location = strings.Join(places, "; ")
	for _, mode := range p.WorkplaceTypes {
		switch mode {
		case "remote":
			j.Remote = true
		case "hybrid", "onsite":
			if j.WorkMode == "" {
				j.WorkMode = mode
			}
		}
	}
	if j.Remote {
		j.WorkMode = "remote"
	}
	for _, kind := range p.EmploymentTypes {
		switch kind {
		case "fulltime":
			j.EmploymentType = "full_time"
		case "parttime":
			j.EmploymentType = "part_time"
		case "contract", "freelance":
			j.EmploymentType = "contract"
		case "internship":
			j.EmploymentType = "internship"
		}
		if j.EmploymentType != "" {
			break
		}
	}
	return j, true
}
