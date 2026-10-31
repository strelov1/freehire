package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

const (
	seamlesshiringPageSize = 100
	seamlesshiringMaxJobs  = 50000
)

var seamlesshiringBoardPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)

type seamlesshiringHTTP interface {
	JSONGetter
}

type seamlesshiring struct {
	http seamlesshiringHTTP
}

func NewSeamlessHiring(c seamlesshiringHTTP) Source { return seamlesshiring{http: c} }

func (seamlesshiring) Provider() string { return "seamlesshiring" }

func (seamlesshiring) fullBoardListing() {}

func (seamlesshiring) emptyBoardListing() {}

type seamlesshiringJob struct {
	ID        int    `json:"id"`
	Title     string `json:"title"`
	Summary   string `json:"summary"`
	Details   string `json:"details"`
	JobType   string `json:"job_type"`
	WorkStyle string `json:"work_style"`
	City      string `json:"city"`
	Location  string `json:"location"`
	CreatedAt string `json:"created_at"`
	PostDate  string `json:"post_date"`
	Company   struct {
		Name string `json:"name"`
	} `json:"company"`
	LocationDetails *struct {
		Country struct {
			Code string `json:"country_code"`
			Name string `json:"name"`
		} `json:"country"`
	} `json:"location_details"`
}

type seamlesshiringPage struct {
	StatusCode int    `json:"status_code"`
	Status     string `json:"status"`
	Data       *struct {
		Jobs *struct {
			Data        *[]seamlesshiringJob `json:"data"`
			CurrentPage int                  `json:"current_page"`
			LastPage    int                  `json:"last_page"`
			PerPage     int                  `json:"per_page"`
			Total       int                  `json:"total"`
		} `json:"jobs"`
	} `json:"data"`
}

func (s seamlesshiring) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !seamlesshiringBoardPattern.MatchString(board) {
		return nil, fmt.Errorf("seamlesshiring: board %q is not a tenant label", e.Board)
	}
	var (
		total int
		last  int
		out   []Job
		seen  = map[string]struct{}{}
	)
	for page := 1; ; page++ {
		body, err := s.page(ctx, board, page)
		if err != nil {
			return nil, err
		}
		jobs := body.Data.Jobs
		if page == 1 {
			total = jobs.Total
			last = jobs.LastPage
			if total < 0 || total > seamlesshiringMaxJobs {
				return nil, fmt.Errorf("seamlesshiring: board %s: total %d is outside 0..%d", board, total, seamlesshiringMaxJobs)
			}
			if last != seamlesshiringLastPage(total) {
				return nil, fmt.Errorf("seamlesshiring: board %s: last_page %d, want %d", board, last, seamlesshiringLastPage(total))
			}
		} else if jobs.Total != total || jobs.LastPage != last {
			return nil, fmt.Errorf("seamlesshiring: board %s: total changed while paging", board)
		}
		if jobs.CurrentPage != page || jobs.PerPage != seamlesshiringPageSize {
			return nil, fmt.Errorf("seamlesshiring: board %s: page %d answered current_page %d per_page %d", board, page, jobs.CurrentPage, jobs.PerPage)
		}
		rows := *jobs.Data
		want := seamlesshiringPageLen(total, page)
		if len(rows) != want {
			return nil, fmt.Errorf("seamlesshiring: board %s: page %d has %d rows, want %d", board, page, len(rows), want)
		}
		for _, row := range rows {
			job, err := seamlesshiringMap(board, e.Company, row)
			if err != nil {
				return nil, err
			}
			if _, ok := seen[job.ExternalID]; ok {
				return nil, fmt.Errorf("seamlesshiring: board %s: duplicate id %s", board, job.ExternalID)
			}
			seen[job.ExternalID] = struct{}{}
			out = append(out, job)
		}
		if page >= last {
			break
		}
	}
	if len(out) != total {
		return nil, fmt.Errorf("seamlesshiring: board %s: listed %d postings, total %d", board, len(out), total)
	}
	return out, nil
}

func (s seamlesshiring) page(ctx context.Context, board string, page int) (seamlesshiringPage, error) {
	u := url.URL{
		Scheme: "https",
		Host:   board + ".seamlesshiring.com",
		Path:   "/v2/jobs/job-list",
		RawQuery: url.Values{
			"limit": {strconv.Itoa(seamlesshiringPageSize)},
			"page":  {strconv.Itoa(page)},
		}.Encode(),
	}
	var body seamlesshiringPage
	if err := s.http.GetJSON(ctx, u.String(), &body); err != nil {
		return seamlesshiringPage{}, err
	}
	if body.Status != "success" || body.StatusCode != 200 {
		return seamlesshiringPage{}, fmt.Errorf("seamlesshiring: board %s: listing status %q (%d)", board, body.Status, body.StatusCode)
	}
	if body.Data == nil || body.Data.Jobs == nil || body.Data.Jobs.Data == nil {
		return seamlesshiringPage{}, errListingAbsent("seamlesshiring", board, "data.jobs.data")
	}
	return body, nil
}

func seamlesshiringMap(board, catalogue string, row seamlesshiringJob) (Job, error) {
	if row.ID <= 0 || strings.TrimSpace(row.Title) == "" {
		return Job{}, fmt.Errorf("seamlesshiring: board %s: a posting has no id or title", board)
	}
	id := strconv.Itoa(row.ID)
	company := strings.TrimSpace(row.Company.Name)
	if company == "" {
		company = catalogue
	}
	posted := parseRFC3339OrDate(row.CreatedAt)
	if posted == nil {
		posted = parseRFC3339OrDate(row.PostDate)
	}
	return Job{
		ExternalID:     id,
		URL:            "https://" + board + ".seamlesshiring.com/job/view/" + id,
		Title:          strings.TrimSpace(row.Title),
		Company:        company,
		Location:       seamlesshiringLocation(row.City, row.Location),
		Description:    seamlesshiringDescription(row.Summary, row.Details),
		PostedAt:       posted,
		WorkMode:       seamlesshiringWorkMode(row.WorkStyle),
		EmploymentType: seamlesshiringEmployment(row.JobType),
		Countries:      seamlesshiringCountry(row),
	}, nil
}

func seamlesshiringLocation(city, place string) string {
	city = strings.TrimSpace(city)
	place = strings.TrimSpace(place)
	if strings.EqualFold(city, place) {
		return city
	}
	return joinNonEmpty(city, place)
}

func seamlesshiringDescription(summary, details string) string {
	var parts []string
	for _, part := range []string{summary, details} {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if strings.Contains(part, "<") {
			parts = append(parts, sanitizeHTML(part))
			continue
		}
		parts = append(parts, plainTextToHTML(part))
	}
	return strings.Join(parts, "\n")
}

func seamlesshiringEmployment(raw string) string {
	raw = strings.ToUpper(strings.TrimSpace(raw))
	raw = strings.NewReplacer("-", "_", " ", "_").Replace(raw)
	return schemaEmploymentType(raw)
}

func seamlesshiringWorkMode(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "remote":
		return "remote"
	case "hybrid":
		return "hybrid"
	case "onsite", "on-site", "on_site":
		return "onsite"
	default:
		return ""
	}
}

func seamlesshiringCountry(row seamlesshiringJob) []string {
	if row.LocationDetails == nil {
		return nil
	}
	code := location.NormalizeCountry(row.LocationDetails.Country.Code)
	if code == "" {
		code = location.NormalizeCountry(row.LocationDetails.Country.Name)
	}
	if code == "" {
		return nil
	}
	return []string{code}
}

func seamlesshiringLastPage(total int) int {
	if total <= 0 {
		return 1
	}
	return (total + seamlesshiringPageSize - 1) / seamlesshiringPageSize
}

func seamlesshiringPageLen(total, page int) int {
	start := (page - 1) * seamlesshiringPageSize
	if start >= total {
		return 0
	}
	n := total - start
	if n > seamlesshiringPageSize {
		return seamlesshiringPageSize
	}
	return n
}
