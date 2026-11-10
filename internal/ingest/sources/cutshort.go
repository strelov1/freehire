package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// cutshort adapts Cutshort (cutshort.io), an Indian hiring platform. Boardless (one public
// syndication XML, no per-tenant board) and multi-company, so it stays in the source facet and
// takes each posting's company from the feed. The feed carries every posting's body and
// structured facets inline (no detail call). See providers/cutshort.md.
type cutshort struct {
	http XMLGetter
}

// cutshortFeedURL is the public syndication document Cutshort publishes for freehire. It is a
// full snapshot of the intended inventory (refreshed daily), not a recency window.
const cutshortFeedURL = "https://cutshort-job-feed-data.s3.ap-southeast-1.amazonaws.com/jobs_data_for_freehire.xml"

// NewCutshort builds the Cutshort adapter over the given HTTP client.
func NewCutshort(c XMLGetter) Source { return cutshort{http: c} }

func (cutshort) Provider() string { return "cutshort" }

func (cutshort) boardless() {}

func (cutshort) aggregator() {}

// fullBoardListing: Fetch is one GET of an unpaginated XML document that is the whole inventory
// Cutshort elects to syndicate. Reaching the document is the proof; there is no page loop that
// could stop early. See the fullBoardListing interface for the bar.
func (cutshort) fullBoardListing() {}

// cutshortFeed is the root <source> document: a flat list of <job> elements.
type cutshortFeed struct {
	Jobs []cutshortItem `xml:"job"`
}

// cutshortItem is one feed row. Cutshort expands multi-city roles into one row per city under
// the same partnerJobId; Fetch collapses those before yielding.
type cutshortItem struct {
	JobType         string           `xml:"jobtype"`
	Country         string           `xml:"country"`
	Title           string           `xml:"title"`
	Date            string           `xml:"date"`
	PartnerJobID    string           `xml:"partnerJobId"`
	URL             string           `xml:"url"`
	ApplyURL        string           `xml:"applyUrl"`
	Company         string           `xml:"company"`
	City            string           `xml:"city"`
	Location        string           `xml:"location"`
	WorkplaceTypes  string           `xml:"workplaceTypes"`
	IsRemote        string           `xml:"isRemote"`
	Description     string           `xml:"description"`
	ExperienceLevel string           `xml:"experienceLevel"`
	Experience      string           `xml:"experience"`
	Salaries        []cutshortSalary `xml:"salaries>salary"`
}

// cutshortSalary is one structured compensation band under <salaries>.
type cutshortSalary struct {
	LowEnd struct {
		Amount       string `xml:"amount"`
		CurrencyCode string `xml:"currencyCode"`
	} `xml:"lowEnd"`
	HighEnd struct {
		Amount       string `xml:"amount"`
		CurrencyCode string `xml:"currencyCode"`
	} `xml:"highEnd"`
	Period string `xml:"period"`
}

// Fetch reads the syndication XML and maps every distinct partnerJobId, joining city-expansion
// rows into one Location.
func (s cutshort) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var feed cutshortFeed
	if err := s.http.GetXML(ctx, cutshortFeedURL, &feed); err != nil {
		return nil, fmt.Errorf("cutshort: feed: %w", err)
	}
	order := make([]string, 0)
	byID := map[string]*Job{}
	cities := map[string][]string{}
	for _, it := range feed.Jobs {
		job, ok := it.toJob()
		if !ok {
			continue
		}
		id := job.ExternalID
		place := cutshortPlace(it)
		if prev, seen := byID[id]; seen {
			if place != "" {
				cities[id] = append(cities[id], place)
			}
			// Any remote city-row makes the posting remote. The live feed (2026-11-10) never mixes
			// arrangements within one partnerJobId and carries no hybrid, so this is a guard only.
			if job.Remote && !prev.Remote {
				prev.Remote = true
				prev.WorkMode = job.WorkMode
			}
			continue
		}
		order = append(order, id)
		j := job
		byID[id] = &j
		if place != "" {
			cities[id] = []string{place}
		}
	}
	out := make([]Job, 0, len(order))
	for _, id := range order {
		j := *byID[id]
		if locs := cities[id]; len(locs) > 0 {
			j.Location = distinctJoin(locs, "; ", func(s string) string { return s })
		}
		out = append(out, j)
	}
	return out, nil
}

// cutshortPlace prefers the city field (the city-expansion key) over the free-text location.
func cutshortPlace(it cutshortItem) string {
	if c := strings.TrimSpace(it.City); c != "" {
		return c
	}
	return strings.TrimSpace(it.Location)
}

// toJob maps one feed row. ok=false when the row lacks the partnerJobId identity or a company
// name (both are required for the catalogue slug).
func (it cutshortItem) toJob() (Job, bool) {
	id := strings.TrimSpace(it.PartnerJobID)
	company := strings.TrimSpace(it.Company)
	title := strings.TrimSpace(it.Title)
	if id == "" || company == "" || title == "" {
		return Job{}, false
	}
	url := strings.TrimSpace(it.ApplyURL)
	if url == "" {
		url = strings.TrimSpace(it.URL)
	}
	if url == "" {
		return Job{}, false
	}
	mode := workplaceTypeMode(it.WorkplaceTypes)
	remote := mode == "remote" || strings.EqualFold(strings.TrimSpace(it.IsRemote), "yes")
	if remote && mode == "" {
		mode = "remote"
	}
	job := Job{
		ExternalID:  id,
		URL:         url,
		Title:       title,
		Company:     company,
		Location:    cutshortPlace(it),
		Description: sanitizeHTML(it.Description),
		Remote:      remote,
		WorkMode:    mode,
		PostedAt:    parseRFC3339(strings.TrimSpace(it.Date)),
		Seniority:   cutshortSeniority(it.ExperienceLevel),
	}
	if et := cutshortEmploymentType(it.JobType); et != "" {
		job.EmploymentType = et
	}
	if code := location.NormalizeCountry(strings.TrimSpace(it.Country)); code != "" {
		job.Countries = []string{code}
	} else {
		// The feed is India-centric; when a row omits <country>, treat India as a weak hint
		// rather than a stated workplace country.
		job.FallbackCountries = []string{"in"}
	}
	if lo, hi, cur, period := cutshortSalaryBounds(it.Salaries); period != "" {
		job.SalaryMin, job.SalaryMax = lo, hi
		job.SalaryCurrency, job.SalaryPeriod = cur, period
	}
	if years := cutshortExperienceYears(it.Experience); years != nil {
		job.ExperienceYearsMin = years
	}
	return job, true
}

// cutshortEmploymentType maps the feed's jobtype onto the employment-type vocabulary.
func cutshortEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "fulltime", "full-time", "full_time", "full time":
		return "full_time"
	case "parttime", "part-time", "part_time", "part time":
		return "part_time"
	case "contract", "contractor", "temporary":
		return "contract"
	case "internship", "intern":
		return "internship"
	default:
		return ""
	}
}

// cutshortSeniority maps LinkedIn-style experienceLevel enums. DIRECTOR/EXECUTIVE are left
// empty so the title dictionary decides rather than forcing a people-manager into an IC bucket.
func cutshortSeniority(level string) string {
	switch strings.ToUpper(strings.TrimSpace(level)) {
	case "INTERNSHIP":
		return "intern"
	case "ENTRY_LEVEL", "ASSOCIATE":
		return "junior"
	case "MID_SENIOR_LEVEL":
		return "middle"
	default:
		return ""
	}
}

// cutshortExperienceYears reads the feed's "4-7" / "5+" experience label as a minimum years
// floor. Zero or unparseable stays unset.
func cutshortExperienceYears(s string) *int {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	// Take the leading integer ("4-7", "5+", "3").
	n := 0
	for _, r := range s {
		if r < '0' || r > '9' {
			break
		}
		n = n*10 + int(r-'0')
	}
	if n <= 0 {
		return nil
	}
	return &n
}

// cutshortSalaryBounds reads the first usable BASE-style band. Period values are the feed's
// "yearly"/"monthly"/…; currency is ISO 4217 from either bound.
func cutshortSalaryBounds(bands []cutshortSalary) (lo, hi *int, currency, period string) {
	for _, b := range bands {
		period = cutshortSalaryPeriod(b.Period)
		if period == "" {
			continue
		}
		lo = cutshortAmount(b.LowEnd.Amount)
		hi = cutshortAmount(b.HighEnd.Amount)
		if lo == nil && hi == nil {
			continue
		}
		currency = strings.TrimSpace(b.LowEnd.CurrencyCode)
		if currency == "" {
			currency = strings.TrimSpace(b.HighEnd.CurrencyCode)
		}
		if currency == "" {
			currency = "INR"
		}
		return lo, hi, strings.ToUpper(currency), period
	}
	return nil, nil, "", ""
}

func cutshortSalaryPeriod(p string) string {
	switch strings.ToLower(strings.TrimSpace(p)) {
	case "yearly", "year", "annual", "annum", "per annum":
		return "year"
	case "monthly", "month", "per month":
		return "month"
	case "daily", "day", "per day":
		return "day"
	case "hourly", "hour", "per hour":
		return "hour"
	default:
		return ""
	}
}

func cutshortAmount(s string) *int {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return nil
	}
	return roundSalaryPart(f)
}
