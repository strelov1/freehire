package sources

import (
	"context"
	"errors"
	"fmt"
	"html"
	"net/url"
	"strings"
)

// higherme adapts HigherMe (app.higherme.com), an hourly-workforce ATS for restaurants, hotels
// and retail. The board is a BRAND id — the 13-hex reference in the brand's public career page,
// app.higherme.com/careers/<brand>. A posting's own URL (app.higherme.com/jobs/<id>) names no
// brand at all; the brand is only reachable through the posting's API record, which is why
// URL recognition (internal/ingest/atsboard) accepts the careers URL and nothing else.
//
// The career page is a SPA over a keyless JSON API on api.higherme.com: /classic/jobs filtered
// by brand lists the brand's postings with title, body, requirements, salary and location
// inline, so a whole board is one paged call with no per-posting detail fetch.
//
// The employer is the configured company, not the posting's: the per-posting company is the
// platform's internal ACCOUNT label ("Huey Magoo's | Orlando Restaurant Concepts LLC"), one per
// franchisee group, so a brand board would scatter across pipe-suffixed company names. The brand
// is what the career page presents as the employer.
type higherme struct {
	http JSONGetter
}

// NewHigherMe builds the HigherMe adapter over the given JSON client.
func NewHigherMe(c JSONGetter) Source { return higherme{http: c} }

func (higherme) Provider() string { return "higherme" }

// fullBoardListing: Fetch walks the listing to its declared last page and fails unless the
// postings it read match the listing's own total. See the fullBoardListing interface.
func (higherme) fullBoardListing() {}

const (
	highermeAPI = "https://api.higherme.com/classic"
	// highermePageSize is the listing's own ceiling: a larger limit is silently clamped to 100.
	highermePageSize = 100
	// highermeMaxPages bounds the walk against a listing that never reports its last page; it is
	// far above any real board.
	highermeMaxPages = 50
)

func (s higherme) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var (
		jobs  []Job
		read  int
		total int
	)
	for page := 1; ; page++ {
		if page > highermeMaxPages {
			return nil, fmt.Errorf("higherme: board %q: listing exceeded %d pages", e.Board, highermeMaxPages)
		}
		var resp highermeListing
		if err := s.http.GetJSON(ctx, highermeListingURL(e.Board, page), &resp); err != nil {
			return nil, fmt.Errorf("higherme: board %q: page %d: %w", e.Board, page, err)
		}
		total = resp.Meta.Total
		for _, p := range resp.Data {
			// The brand filter is a query parameter the API could stop honouring, and an ignored
			// filter answers with the WHOLE platform (an unknown filter key does exactly that) —
			// tens of thousands of postings filed under one employer. Every posting must belong
			// to the board asked for.
			if b := p.Relations.Location.Relations.Brand.ID; b != e.Board {
				return nil, fmt.Errorf("higherme: board %q: listing returned a posting of brand %q — brand filter not applied", e.Board, b)
			}
			read++
			if job, ok := p.toJob(e.Company); ok {
				jobs = append(jobs, job)
			}
		}
		if page >= resp.Meta.LastPage || len(resp.Data) == 0 {
			break
		}
	}
	if read != total {
		return nil, fmt.Errorf("higherme: board %q: read %d postings, listing reports %d", e.Board, read, total)
	}
	if total == 0 {
		// An empty listing is also what an unknown brand id answers. The brand resource tells the
		// two apart: a brand the platform no longer has is a 404 there.
		if err := s.http.GetJSON(ctx, highermeAPI+"/brands/"+url.PathEscape(e.Board), &struct{}{}); err != nil {
			var se *StatusError
			if errors.As(err, &se) && se.Code == 404 {
				return nil, fmt.Errorf("higherme: board %q: %w", e.Board, ErrBoardGone)
			}
			return nil, fmt.Errorf("higherme: board %q: brand: %w", e.Board, err)
		}
	}
	return jobs, nil
}

// highermeListingURL is one page of a brand's postings, with the location (and its brand, for
// the filter check) inlined.
func highermeListingURL(brand string, page int) string {
	q := url.Values{
		"page":              {fmt.Sprint(page)},
		"limit":             {fmt.Sprint(highermePageSize)},
		"includes":          {"location,location.brand"},
		"filters[brand.id]": {brand},
	}
	return highermeAPI + "/jobs?" + q.Encode()
}

// highermeListing is one page of /classic/jobs.
type highermeListing struct {
	Data []highermePosting `json:"data"`
	Meta struct {
		LastPage int `json:"last_page"`
		Total    int `json:"total"`
	} `json:"meta"`
}

type highermePosting struct {
	ID         string `json:"id"`
	Attributes struct {
		Title        string   `json:"title"`
		FullTime     bool     `json:"full_time"`
		PartTime     bool     `json:"part_time"`
		About        string   `json:"about"`
		Summary      string   `json:"summary"`
		Requirements []string `json:"requirements"`
		Salary       struct {
			Min  *float64 `json:"min"`
			Max  *float64 `json:"max"`
			Type string   `json:"type"`
		} `json:"salary"`
		Status string `json:"status"`
	} `json:"attributes"`
	Relations struct {
		Location struct {
			Attributes struct {
				City  string `json:"city"`
				State struct {
					Name string `json:"name"`
				} `json:"state"`
				Country struct {
					Name  string `json:"name"`
					Short string `json:"short"`
				} `json:"country"`
			} `json:"attributes"`
			Relations struct {
				Brand struct {
					ID string `json:"id"`
				} `json:"brand"`
			} `json:"relations"`
		} `json:"location"`
	} `json:"relations"`
}

// toJob maps one listed posting; ok=false drops a posting that is not published.
func (p highermePosting) toJob(company string) (Job, bool) {
	a := p.Attributes
	if p.ID == "" || a.Status != "published" {
		return Job{}, false
	}
	loc := p.Relations.Location.Attributes
	location := joinNonEmpty(loc.City, loc.State.Name, loc.Country.Name)
	job := Job{
		ExternalID:     p.ID,
		URL:            "https://app.higherme.com/jobs/" + url.PathEscape(p.ID),
		Title:          strings.TrimSpace(a.Title),
		Company:        company,
		Location:       location,
		Description:    sanitizeHTML(a.Summary + highermeRequirements(a.Requirements) + headedSection("About", a.About)),
		Remote:         isRemote(location),
		EmploymentType: highermeEmploymentType(a.FullTime, a.PartTime),
		Countries:      countryFromCode(loc.Country.Short),
		// No PostedAt: the listing carries only updated_at, and the posting page's "date_posted"
		// is the same re-stamped value — a posting created in 2021 reads as posted two days ago.
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = highermeSalary(a.Salary.Min, a.Salary.Max, a.Salary.Type, loc.Country.Short)
	return job, true
}

// highermeRequirements renders the posting's requirement bullets as a headed list.
func highermeRequirements(reqs []string) string {
	var b strings.Builder
	for _, r := range reqs {
		if r = strings.TrimSpace(r); r != "" {
			b.WriteString("<li>" + html.EscapeString(r) + "</li>")
		}
	}
	if b.Len() == 0 {
		return ""
	}
	return headedSection("Requirements", "<ul>"+b.String()+"</ul>")
}

// highermeEmploymentType maps the two schedule flags. A posting open to both schedules states
// neither, so it is left to the description parser.
func highermeEmploymentType(fullTime, partTime bool) string {
	switch {
	case fullTime && !partTime:
		return "full_time"
	case partTime && !fullTime:
		return "part_time"
	default:
		return ""
	}
}

// highermeSalary reads the posting's pay range. The API states the amount and its unit but no
// currency; the platform's own posting page derives both the same way this does — USD for a US
// location, CAD otherwise (it operates in the two countries only), and "hourly" as HOUR with
// anything else as YEAR. A location in neither country gets no salary rather than a guessed
// currency.
func highermeSalary(lo, hi *float64, typ, country string) (minPay, maxPay *int, currency, period string) {
	if lo == nil && hi == nil {
		return nil, nil, "", ""
	}
	switch strings.ToUpper(country) {
	case "US":
		currency = "USD"
	case "CA":
		currency = "CAD"
	default:
		return nil, nil, "", ""
	}
	period = "year"
	if typ == "hourly" {
		period = "hour"
	}
	return highermeAmount(lo), highermeAmount(hi), currency, period
}

func highermeAmount(v *float64) *int {
	if v == nil || *v <= 0 {
		return nil
	}
	n := int(*v + 0.5)
	return &n
}
