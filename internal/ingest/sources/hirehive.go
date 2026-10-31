package sources

import (
	"context"
	"errors"
	"fmt"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

const (
	hirehivePageSize = 100
	hirehiveMaxPages = 500
)

var (
	hirehiveBoard    = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9-]{0,61}[a-z0-9])?$`)
	hirehiveCurrency = regexp.MustCompile(`^[A-Z]{3}$`)
)

type hirehiveHTTP interface {
	JSONGetter
}

type hirehive struct {
	http hirehiveHTTP
}

func NewHireHive(c hirehiveHTTP) Source { return hirehive{http: c} }

func (hirehive) Provider() string { return "hirehive" }

func (hirehive) fullBoardListing() {}

func (s hirehive) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	board := strings.ToLower(strings.TrimSpace(e.Board))
	if !hirehiveBoard.MatchString(board) {
		return nil, fmt.Errorf("hirehive: board %q is not a tenant label", e.Board)
	}
	seen := map[string]struct{}{}
	jobs := []Job{}
	var total int
	for page := 1; page <= hirehiveMaxPages; page++ {
		var body hirehivePage
		u := hirehiveListURL(board, page)
		if err := s.http.GetJSON(ctx, u, &body); err != nil {
			var se *StatusError
			if page == 1 && errors.As(err, &se) && (se.Code == http.StatusNotFound || se.Code == http.StatusGone) {
				return nil, fmt.Errorf("hirehive: board %q: %w", board, ErrBoardGone)
			}
			return nil, fmt.Errorf("hirehive: board %s page %d: %w", board, page, err)
		}
		if body.Meta.TotalItems == nil || body.Meta.HasNext == nil || body.Items == nil {
			return nil, fmt.Errorf("hirehive: board %s page %d: listing omitted its count", board, page)
		}
		if page == 1 {
			total = *body.Meta.TotalItems
			if total < 0 {
				return nil, fmt.Errorf("hirehive: board %s: stated total %d", board, total)
			}
		}
		if len(body.Items) == 0 {
			if *body.Meta.HasNext || len(jobs) != total {
				return nil, fmt.Errorf("hirehive: board %s page %d: empty page before the listing ended", board, page)
			}
			break
		}
		for _, item := range body.Items {
			job, err := hirehiveJob(board, e.Company, item)
			if err != nil {
				return nil, fmt.Errorf("hirehive: board %s: %w", board, err)
			}
			if _, ok := seen[job.ExternalID]; ok {
				continue
			}
			seen[job.ExternalID] = struct{}{}
			jobs = append(jobs, job)
		}
		if !*body.Meta.HasNext {
			break
		}
		if page == hirehiveMaxPages {
			return nil, fmt.Errorf("hirehive: board %s: listing runs past %d pages", board, hirehiveMaxPages)
		}
	}
	if len(jobs) != total {
		return nil, fmt.Errorf("hirehive: board %s: read %d postings, source stated %d", board, len(jobs), total)
	}
	return jobs, nil
}

func hirehiveListURL(board string, page int) string {
	q := url.Values{}
	q.Set("page", strconv.Itoa(page))
	q.Set("page_size", strconv.Itoa(hirehivePageSize))
	return (&url.URL{
		Scheme:   "https",
		Host:     board + ".hirehive.com",
		Path:     "/api/v2/jobs",
		RawQuery: q.Encode(),
	}).String()
}

func hirehiveJob(board, company string, item hirehiveItem) (Job, error) {
	id := strings.TrimSpace(item.ID)
	title := strings.TrimSpace(item.Title)
	if id == "" || title == "" {
		return Job{}, fmt.Errorf("posting %q has no id or title", item.ID)
	}
	page, err := url.Parse(strings.TrimSpace(item.HostedURL))
	if err != nil || page.Scheme != "https" || !strings.EqualFold(page.Hostname(), board+".hirehive.com") {
		return Job{}, fmt.Errorf("posting %s url %q is not on this tenant", id, item.HostedURL)
	}
	job := Job{
		ExternalID:     id,
		URL:            page.String(),
		Title:          title,
		Company:        company,
		Location:       joinNonEmpty(item.Location, item.StateCode, item.Country.Name),
		Description:    sanitizeHTML(hirehiveBody(item.Description)),
		EmploymentType: hirehiveEmployment[item.Type.Type],
		Seniority:      hirehiveSeniority[item.Experience.Type],
		PostedAt:       parseRFC3339(item.PublishedDate),
	}
	if code := location.NormalizeCountry(item.Country.Code); code != "" {
		job.Countries = []string{code}
	}
	hirehiveSalary(&job, item.Compensation)
	return job, nil
}

func hirehiveBody(d hirehiveDescriptionField) string {
	if d.HTML != "" {
		return d.HTML
	}
	return plainTextToHTML(d.Text)
}

func hirehiveSalary(job *Job, tiers []hirehiveTier) {
	for _, tier := range tiers {
		period, ok := hirehivePeriods[strings.ToLower(strings.TrimSpace(tier.Interval))]
		currency := strings.ToUpper(strings.TrimSpace(tier.CurrencyCode))
		lo, okLo := hirehiveMoney(tier.MinValue)
		hi, okHi := hirehiveMoney(tier.MaxValue)
		if !ok || !hirehiveCurrency.MatchString(currency) || (!okLo && !okHi) {
			continue
		}
		if okLo && okHi && lo > hi {
			continue
		}
		if okLo {
			v := lo
			job.SalaryMin = &v
		}
		if okHi {
			v := hi
			job.SalaryMax = &v
		}
		job.SalaryCurrency = currency
		job.SalaryPeriod = period
		return
	}
}

func hirehiveMoney(v *float64) (int, bool) {
	if v == nil {
		return 0, false
	}
	n := *v
	whole := n == math.Trunc(n)
	if n <= 0 || math.IsNaN(n) || math.IsInf(n, 0) || !whole || n > float64(math.MaxInt) {
		return 0, false
	}
	return int(n), true
}

var hirehiveEmployment = map[string]string{
	"FullTime":   "full_time",
	"PartTime":   "part_time",
	"Contract":   "contract",
	"Temporary":  "contract",
	"Freelance":  "contract",
	"Internship": "internship",
	"Fellowship": "fellowship",
}

var hirehiveSeniority = map[string]string{
	"EntryLevel": "junior",
	"MidLevel":   "middle",
	"Senior":     "senior",
}

var hirehivePeriods = map[string]string{
	"year":  "year",
	"month": "month",
	"day":   "day",
	"hour":  "hour",
}

type hirehivePage struct {
	Meta struct {
		TotalItems *int  `json:"total_items"`
		HasNext    *bool `json:"has_next_page"`
	} `json:"meta"`
	Items []hirehiveItem `json:"items"`
}

type hirehiveItem struct {
	ID            string                   `json:"id"`
	Title         string                   `json:"title"`
	HostedURL     string                   `json:"hosted_url"`
	Location      string                   `json:"location"`
	StateCode     string                   `json:"state_code"`
	PublishedDate string                   `json:"published_date"`
	Description   hirehiveDescriptionField `json:"description"`
	Type          struct {
		Type string `json:"type"`
	} `json:"type"`
	Experience struct {
		Type string `json:"type"`
	} `json:"experience"`
	Country struct {
		Name string `json:"name"`
		Code string `json:"code"`
	} `json:"country"`
	Compensation []hirehiveTier `json:"compensation_tiers"`
}

type hirehiveDescriptionField struct {
	HTML string `json:"html"`
	Text string `json:"text"`
}

type hirehiveTier struct {
	Interval     string   `json:"interval"`
	CurrencyCode string   `json:"currency_code"`
	MinValue     *float64 `json:"min_value"`
	MaxValue     *float64 `json:"max_value"`
}
