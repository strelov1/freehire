package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	xhtml "golang.org/x/net/html"
	"golang.org/x/time/rate"
)

// workwise adapts Workwise (www.workwise.io), a German recruiting platform: employers publish
// postings that candidates apply to on Workwise itself, without a cover letter, and the same
// postings are syndicated to job boards through short links (www.workwise.io/s/<code>) that land
// on the posting. It is boardless and an aggregator — thousands of employers, named per posting.
//
// The posting set is one sitemap, enquiryFocusPagesSitemap.xml (4,015 postings on 2026-10-25),
// which names only each posting's URL; every fact is bought with the posting page's schema.org
// JobPosting, so the adapter hydrates only the postings the catalogue lacks. See the "Workwise
// traps" section of AGENTS.md for the measurements behind each rule.
type workwise struct {
	http         workwiseHTTP
	limiter      waiter
	maxNewPerRun int
}

type workwiseHTTP interface {
	XMLGetter
	HTMLGetter
}

const (
	workwiseBaseURL      = "https://www.workwise.io"
	workwiseSitemapIndex = workwiseBaseURL + "/sitemap.xml"
	// workwiseSitemapNeedle selects the posting sitemap; the index's other files list search
	// pages, company profiles, the career guide and the help centre.
	workwiseSitemapNeedle = "/enquiryFocusPagesSitemap.xml"
)

// workwiseRequestInterval paces every request of a run through one limiter. 60 posting pages at
// ~2.5/s drew no refusal; two a second is courtesy, not a limit we measured.
const workwiseRequestInterval = 500 * time.Millisecond

// workwiseDetailWorkers keeps enough pages in flight that the limiter, not latency, sets the pace.
const workwiseDetailWorkers = 3

// workwiseMaxNewPerRun bounds how many unseen postings one run hydrates: 2,500 pages at two a
// second is ~21 minutes. The first run faces the whole catalogue (~4k), so it takes two runs.
const workwiseMaxNewPerRun = 2500

// NewWorkwise builds the adapter over the given client.
func NewWorkwise(c workwiseHTTP) Source {
	return workwise{
		http:         c,
		limiter:      rate.NewLimiter(rate.Every(workwiseRequestInterval), 1),
		maxNewPerRun: workwiseMaxNewPerRun,
	}
}

func (workwise) Provider() string { return "workwise" }

// One platform-wide catalogue, no per-tenant id.
func (workwise) boardless() {}

// Thousands of employers post here, each named per posting.
func (workwise) aggregator() {}

type workwisePosting struct {
	id  int64
	url string
}

func (p workwisePosting) externalID() string { return strconv.FormatInt(p.id, 10) }

// workwisePostingPath matches a posting URL, /job/<id>-<slug>. The slug is cosmetic: /job/<id>
// redirects to it, so the id alone is the identity.
var workwisePostingPath = regexp.MustCompile(`^/job/(\d+)(?:-[^/]*)?/?$`)

// workwisePostingID returns the posting id a URL carries, or 0 when it is not a posting URL.
func workwisePostingID(raw string) int64 {
	u, err := url.Parse(raw)
	if err != nil {
		return 0
	}
	id, _ := strconv.ParseInt(firstSubmatch(workwisePostingPath, u.Path), 10, 64)
	return id
}

// Fetch is the list-only fallback HydratingSource specifies: everything is unseen.
func (s workwise) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew lists every posting the sitemap names, refreshes the ones already stored and hydrates
// up to maxNewPerRun of the rest, newest id first. A posting whose page cannot be read is not
// yielded: nothing is stored for it yet, so a later run tries again.
func (s workwise) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	var unseen []workwisePosting
	var refreshed []Job
	for _, p := range postings {
		if seen(p.externalID()) {
			refreshed = append(refreshed, Job{ExternalID: p.externalID(), URL: p.url, SeenRefresh: true})
			continue
		}
		if len(unseen) < s.maxNewPerRun {
			unseen = append(unseen, p)
		}
	}
	jobs := fetchDetails(unseen, workwiseDetailWorkers, func(p workwisePosting) (Job, bool) {
		return s.detail(ctx, p)
	})
	return append(jobs, refreshed...), nil
}

// list reads the posting sitemap and returns its postings, newest id first. Either request
// failing fails the crawl, as does an index that no longer names the posting sitemap.
func (s workwise) list(ctx context.Context) ([]workwisePosting, error) {
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	loc, err := resolveSubSitemap(ctx, s.http, workwiseSitemapIndex, workwiseSitemapNeedle)
	if err != nil {
		return nil, fmt.Errorf("workwise: %w", err)
	}
	if loc == "" {
		return nil, fmt.Errorf("workwise: sitemap index names no %s", workwiseSitemapNeedle)
	}
	if err := s.limiter.Wait(ctx); err != nil {
		return nil, err
	}
	doc, err := getSitemap(ctx, s.http, loc)
	if err != nil {
		return nil, fmt.Errorf("workwise: sitemap %s: %w", loc, err)
	}
	ids := make(map[int64]bool)
	var out []workwisePosting
	for _, u := range doc.URLs {
		id := workwisePostingID(u.Loc)
		if id == 0 || ids[id] {
			continue
		}
		ids[id] = true
		out = append(out, workwisePosting{id: id, url: u.Loc})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].id > out[j].id })
	return out, nil
}

// detail fetches one posting page. A posting that closed still answers 200, with "Dieser Job ist
// nicht mehr aktiv" and no JobPosting, so a page without one is dropped.
func (s workwise) detail(ctx context.Context, p workwisePosting) (Job, bool) {
	if err := s.limiter.Wait(ctx); err != nil {
		return Job{}, false
	}
	root, err := s.http.GetHTML(ctx, p.url)
	if err != nil {
		return Job{}, false
	}
	return workwiseDetail(root, p)
}

// workwiseLD is the part of the posting page's schema.org JobPosting the adapter reads.
type workwiseLD struct {
	Title              string          `json:"title"`
	Description        string          `json:"description"`
	DatePosted         string          `json:"datePosted"`
	EmploymentType     json.RawMessage `json:"employmentType"`
	JobLocationType    string          `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
	JobLocation                   []workwisePlace `json:"jobLocation"`
	ApplicantLocationRequirements []struct {
		Name string `json:"name"`
	} `json:"applicantLocationRequirements"`
	BaseSalary *struct {
		Currency string `json:"currency"`
		Value    struct {
			Value    string `json:"value"`
			MinValue string `json:"minValue"`
			MaxValue string `json:"maxValue"`
			UnitText string `json:"unitText"`
		} `json:"value"`
	} `json:"baseSalary"`
}

type workwisePlace struct {
	Address struct {
		AddressLocality string `json:"addressLocality"`
		AddressCountry  string `json:"addressCountry"`
	} `json:"address"`
}

// workwiseDetail maps a posting page to a Job. ok is false for a page carrying no posting.
func workwiseDetail(root *xhtml.Node, p workwisePosting) (Job, bool) {
	var ld workwiseLD
	if !ldJobPosting(root, &ld) || strings.TrimSpace(ld.Title) == "" {
		return Job{}, false
	}
	company := strings.TrimSpace(ld.HiringOrganization.Name)
	if company == "" {
		return Job{}, false
	}
	var countries []string
	for _, l := range ld.JobLocation {
		countries = append(countries, l.Address.AddressCountry)
	}
	// A remote posting may name no place at all; the country it hires in is then stated only
	// as applicantLocationRequirements.
	if len(ld.JobLocation) == 0 {
		for _, r := range ld.ApplicantLocationRequirements {
			countries = append(countries, r.Name)
		}
	}
	// addressRegion is deliberately not read: it is a German state abbreviation ("NRW", "BY")
	// that the location dictionary would not resolve, and the cities already place the posting.
	const maxPlaces = 5
	places := ld.JobLocation
	if len(places) > maxPlaces {
		places = places[:maxPlaces]
	}
	j := Job{
		ExternalID:     p.externalID(),
		URL:            p.url,
		Title:          strings.TrimSpace(ld.Title),
		Company:        company,
		Location:       distinctJoin(places, "; ", func(l workwisePlace) string { return l.Address.AddressLocality }),
		Description:    sanitizeHTML(ld.Description),
		PostedAt:       parseRFC3339(strings.TrimSpace(ld.DatePosted)),
		EmploymentType: workwiseEmploymentType(ld.EmploymentType),
		WorkMode:       workwiseWorkMode(ld.JobLocationType, ld.Description),
		Countries:      countriesFromCodes(countries),
	}
	if ld.BaseSalary != nil && strings.EqualFold(ld.BaseSalary.Currency, "EUR") {
		v := ld.BaseSalary.Value
		lo, hi := workwiseAmount(firstNonEmpty(v.MinValue, v.Value)), workwiseAmount(firstNonEmpty(v.MaxValue, v.Value))
		if period := workwiseSalaryPeriod(v.UnitText); period != "" && (lo != nil || hi != nil) {
			j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = lo, hi, "EUR", period
		}
	}
	return j, true
}

// workwiseEmploymentType maps employmentType, a string or a list. A list naming INTERN is an
// internship whatever else it names (the platform pairs it with FULL_TIME); otherwise only a
// single full- or part-time value states a schedule, and anything mixed with OTHER/TEMPORARY
// states none.
func workwiseEmploymentType(raw json.RawMessage) string {
	var types []string
	if err := json.Unmarshal(raw, &types); err != nil {
		var one string
		if json.Unmarshal(raw, &one) != nil {
			return ""
		}
		types = []string{one}
	}
	for _, t := range types {
		if t == "INTERN" {
			return "internship"
		}
	}
	if len(types) != 1 {
		return ""
	}
	switch types[0] {
	case "FULL_TIME":
		return "full_time"
	case "PART_TIME":
		return "part_time"
	}
	return ""
}

// workwiseHomeOfficeDays is the home-office line the platform renders from the posting form's
// own field ("1 - 2 Tage pro Woche Homeoffice", "1 Tag pro Woche Homeoffice").
var workwiseHomeOfficeDays = regexp.MustCompile(`<span>\s*\d+(?:\s*-\s*\d+)?\s+Tage?\s+pro\s+Woche\s+Homeoffice\s*</span>`)

// workwiseWorkMode reads the arrangement the posting form states. jobLocationType TELECOMMUTE
// with the "Remote Job" line is remote; "Remote möglich" (remote possible) and a number of
// home-office days a week are hybrid. "Kein Homeoffice" is NOT read as onsite: it is what 70% of
// sampled postings show, and the form cannot tell an employer who chose it from one who left the
// field alone.
func workwiseWorkMode(locationType, description string) string {
	switch {
	case strings.Contains(description, "<span>Remote Job</span>") && locationType == "TELECOMMUTE":
		return "remote"
	case strings.Contains(description, "<span>Remote möglich</span>"), workwiseHomeOfficeDays.MatchString(description):
		return "hybrid"
	}
	return ""
}

// workwiseAmount parses a salary figure ("54000.00"), or nil when it is absent or not positive.
func workwiseAmount(s string) *int {
	f, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil || f <= 0 {
		return nil
	}
	v := int(math.Round(f))
	return &v
}

func workwiseSalaryPeriod(unit string) string {
	switch unit {
	case "YEAR":
		return "year"
	case "MONTH":
		return "month"
	case "HOUR":
		return "hour"
	}
	return ""
}
