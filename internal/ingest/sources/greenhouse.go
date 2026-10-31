package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"regexp"
	"slices"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// greenhouseBaseURL is the Greenhouse public boards API root. Unlike Lever, Greenhouse
// serves EU-hosted boards (job-boards.eu.greenhouse.io front-ends) from this same single
// API host, so no per-region host selection is needed.
const greenhouseBaseURL = "https://boards-api.greenhouse.io/v1/boards"

// greenhouse adapts the Greenhouse public boards API. The list endpoint carries the
// description inline when asked with content=true, so no per-posting detail request
// is needed.
type greenhouse struct {
	http JSONGetter
}

// NewGreenhouse builds the Greenhouse adapter over the given HTTP client.
func NewGreenhouse(c JSONGetter) Source { return greenhouse{http: c} }

func (greenhouse) Provider() string { return "greenhouse" }

// fullBoardListing: Fetch is a single unpaginated request returning the board's whole jobs
// array — no loop that could stop early. See the fullBoardListing interface for the bar.
func (greenhouse) fullBoardListing() {}

// emptyBoardListing: an unknown board answers 404, and an emptied one
// answers 200 with "jobs": [] (skedda, 2026-10-29); an envelope without the array fails Fetch. See the emptyBoardListing interface for the bar.
func (greenhouse) emptyBoardListing() {}

func (g greenhouse) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	// pay_transparency=true is what unlocks pay_input_ranges; without it the API omits
	// the field entirely rather than returning it empty (confirmed live 2026-10-31).
	url := fmt.Sprintf("%s/%s/jobs?content=true&pay_transparency=true", greenhouseBaseURL, e.Board)

	var resp struct {
		Jobs []GreenhousePosting `json:"jobs"`
	}
	if err := g.http.GetJSON(ctx, url, &resp); err != nil {
		return nil, fmt.Errorf("greenhouse: fetch board %s: %w", e.Board, err)
	}
	if resp.Jobs == nil {
		return nil, errListingAbsent("greenhouse", e.Board, "jobs")
	}

	jobs := make([]Job, 0, len(resp.Jobs))
	for _, j := range resp.Jobs {
		job := MapGreenhousePosting(j)
		job.ExternalID = strconv.FormatInt(j.ID, 10)
		job.Company = e.Company
		jobs = append(jobs, job)
	}
	return jobs, nil
}

// CompanyDescription fetches the board's own "about us" text from the board-metadata
// endpoint — distinct from the /jobs listing used by Fetch, so this costs one extra
// request per board crawl, not per posting. Unlike the listing's content=true field,
// this one is raw (not entity-encoded) HTML, confirmed against live boards. An
// employer who has not filled the field in yields ("", nil), not an error.
func (g greenhouse) CompanyDescription(ctx context.Context, e CompanyEntry) (string, error) {
	url := fmt.Sprintf("%s/%s", greenhouseBaseURL, e.Board)

	var resp struct {
		Content string `json:"content"`
	}
	if err := g.http.GetJSON(ctx, url, &resp); err != nil {
		return "", fmt.Errorf("greenhouse: fetch board metadata %s: %w", e.Board, err)
	}
	if resp.Content == "" {
		return "", nil
	}
	return sanitizeHTML(resp.Content), nil
}

// GreenhousePosting is one job from the Greenhouse public boards API (the list endpoint
// with content=true and the per-job endpoint share the shape), exported so the
// link-following adapter (internal/ingest/linksource) decodes the same payload.
type GreenhousePosting struct {
	ID          int64  `json:"id"`
	Title       string `json:"title"`
	AbsoluteURL string `json:"absolute_url"`
	UpdatedAt   string `json:"updated_at"`
	Content     string `json:"content"`
	// CompanyName is served by the per-job endpoint only; the board list leaves it empty.
	CompanyName string `json:"company_name"`
	Location    struct {
		Name string `json:"name"`
	} `json:"location"`
	// Metadata carries the employer's own custom application-question fields — most
	// boards set none (null), some carry unrelated ones ("Headcount #"), and a minority
	// configure an "Employment Type" field read by greenhouseMetadataEmploymentType.
	Metadata []GreenhouseMetadataField `json:"metadata"`
	// PayInputRanges is the employer's own pay transparency data, present only when the
	// request carries pay_transparency=true. A posting may list several (one per state
	// or zone); like Ashby's tiers, only the first is read.
	PayInputRanges []GreenhousePayRange `json:"pay_input_ranges"`
	// Offices are the employer's offices the posting is filed under. Location is
	// Greenhouse's own geocoded "City, Region, Country" string (null on many offices);
	// Name is the employer's label ("Remote US", "Headquarters"). Read only by
	// greenhouseOfficeCountry.
	Offices []GreenhouseOffice `json:"offices"`
}

// GreenhouseOffice is one office a Greenhouse posting is filed under.
// GreenhousePayRange is one pay_input_ranges entry. The amounts are in CENTS. The API
// carries no period field: the period is stated, when it is, only in the range's Title
// ("Hourly Pay Range") or its HTML Blurb ("the annual base salary range"; matched as
// markup, since a word boundary holds beside a tag), so
// greenhousePayPeriod reads those and a range that states neither is dropped.
type GreenhousePayRange struct {
	MinCents     *int64 `json:"min_cents"`
	MaxCents     *int64 `json:"max_cents"`
	CurrencyType string `json:"currency_type"`
	Title        string `json:"title"`
	Blurb        string `json:"blurb"`
}

type GreenhouseOffice struct {
	Name     string `json:"name"`
	Location string `json:"location"`
}

// GreenhouseMetadataField is one employer-configured custom field on a posting.
// Value is decoded lazily (raw JSON) because it varies by field: a plain string for
// a single_select/short_text field, null when unset, or an empty array for an unset
// multi-select — greenhouseMetadataEmploymentType treats every shape but "a string"
// as absent rather than guessing.
type GreenhouseMetadataField struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// MapGreenhousePosting maps a Greenhouse boards-API posting into a Job, so the board
// adapter and the link-following adapter produce identical facets for the same posting.
// ExternalID and Company are left to the caller: the board adapter sets the raw id (the
// pipeline namespaces it by board) and the configured company; the link resolver
// namespaces the id itself and takes the company from the API's company_name (a TG link
// can point at a board the crawl config does not list).
func MapGreenhousePosting(j GreenhousePosting) Job {
	job := Job{
		URL:               j.AbsoluteURL,
		Title:             j.Title,
		Location:          j.Location.Name,
		Description:       sanitizeHTML(html.UnescapeString(j.Content)),
		Remote:            isRemote(j.Location.Name),
		PostedAt:          parseRFC3339(j.UpdatedAt),
		EmploymentType:    greenhouseMetadataEmploymentType(j.Metadata),
		FallbackCountries: greenhouseOfficeCountry(j),
	}
	job.SalaryMin, job.SalaryMax, job.SalaryCurrency, job.SalaryPeriod = greenhouseSalary(j.PayInputRanges)
	return job
}

// greenhouseSalary reads the first pay range whose period the employer states. Measured
// on the 25 largest boards (2026-10-31) about half state it; the rest ("US Salary
// Range" at $190,000, "Pay Range" at $18) are dropped rather than given a period by
// magnitude, because a wrong period publishes an hourly rate as a yearly salary.
func greenhouseSalary(ranges []GreenhousePayRange) (salaryMin, salaryMax *int, currency, period string) {
	// The first range that is not a named bonus/commission/equity component. On the
	// largest boards every multi-range posting lists one range per state or zone, all
	// of them base pay, so this skips nothing today — it keeps a board that does list a
	// bonus first from publishing the bonus as the salary.
	i := slices.IndexFunc(ranges, func(r GreenhousePayRange) bool { return !greenhouseNonBase.MatchString(r.Title) })
	if i < 0 {
		return nil, nil, "", ""
	}
	r := ranges[i]
	period = greenhousePayPeriod(r.Title)
	if period == "" {
		period = greenhousePayPeriod(r.Blurb)
	}
	if period == "" || r.CurrencyType == "" {
		return nil, nil, "", ""
	}
	scale := currencyMinorUnits(r.CurrencyType)
	if r.MinCents != nil {
		salaryMin = roundSalaryPart(float64(*r.MinCents) / scale)
	}
	if r.MaxCents != nil {
		salaryMax = roundSalaryPart(float64(*r.MaxCents) / scale)
	}
	if salaryMin == nil && salaryMax == nil {
		return nil, nil, "", ""
	}
	// A boilerplate blurb can say "annual" over an hourly range ("$18 - $25", measured
	// on a dozen Colorado/Washington ranges): no currency these boards publish in has a
	// yearly salary under 1,000, so such a range is dropped, not re-labelled.
	if period == "year" {
		top := 0
		for _, b := range []*int{salaryMin, salaryMax} {
			if b != nil && *b > top {
				top = *b
			}
		}
		if top < 1000 {
			return nil, nil, "", ""
		}
	}
	return salaryMin, salaryMax, r.CurrencyType, period
}

// greenhouseNonBase names a pay range that is not the base wage.
var greenhouseNonBase = regexp.MustCompile(`(?i)\bbonus|\bcommission|\bequity\b|\bstock\b|\bincentive`)

// currencyMinorUnits is how many of a currency's smallest unit make one: Greenhouse's
// "cents" are the currency's minor unit, so a JPY range is already in yen — Anduril's
// Tokyo range reads 20,454,000, ¥20M a year, not ¥204,540. ISO 4217 exponents 0 and 3;
// every other currency has two.
func currencyMinorUnits(code string) float64 {
	switch strings.ToUpper(code) {
	case "BIF", "CLP", "DJF", "GNF", "ISK", "JPY", "KMF", "KRW", "PYG", "RWF", "UGX", "UYI", "VND", "VUV", "XAF", "XOF", "XPF":
		return 1
	case "BHD", "IQD", "JOD", "KWD", "LYD", "OMR", "TND":
		return 1000
	default:
		return 100
	}
}

// greenhousePeriodWords are the phrases that state a pay period, in the languages the
// catalogue's Greenhouse boards write their ranges in. Hour is tried first: "hourly
// rate, paid annually in bonuses" is a wage per hour.
var greenhousePeriodWords = []struct {
	period string
	re     *regexp.Regexp
}{
	{"hour", regexp.MustCompile(`(?i)\bhour(ly)?\b|\bper hr\b|/\s?hr\b|\bhoraire\b|\bstündlich\b|\bstundenlohn\b|\bpor hora\b`)},
	{"year", regexp.MustCompile(`(?i)\bannual(ly)?\b|\byearly\b|\bper year\b|\ba year\b|/\s?yr\b|\bannuel(le)?\b|\bjährlich\b|\bjahresgehalt\b|\banual\b`)},
	{"month", regexp.MustCompile(`(?i)\bmonth(ly)?\b|\bper month\b|\bmensuel(le)?\b|\bmonatlich\b|\bmensual\b`)},
}

func greenhousePayPeriod(text string) string {
	for _, w := range greenhousePeriodWords {
		if w.re.MatchString(text) {
			return w.period
		}
	}
	return ""
}

// greenhouseOfficeCountry reads the country of a bare-"Remote" posting from the offices
// it is filed under — the only place Greenhouse states it (location.name is the
// employer's free text, and "Remote" names no place). It is a FallbackCountries signal,
// not Countries, because an office is sometimes the employer's headquarters rather than
// the hiring scope; jobderive therefore lets the title and description outrank it.
//
// Measured 2026-10-23 over the live listings of the 1,612 boards holding an open
// bare-remote posting with no geography (62,172 postings, 8,737 of them bare-remote):
// this rule pins 1,570 postings on 451 boards, 92% of them to us. Where enrichment had
// pinned a country it agreed on 179 and disagreed on 7 — an HQ office far from the team
// (a US team under a Greek HQ, a Tunisia role under a Barcelona office) and one office
// label the dictionary misreads ("ID - Boise" as Indonesia). Where a title or
// description restriction also fired it agreed 63 times and disagreed 3, all of which
// that restriction wins.
//
// So it is deliberately narrow:
//   - only a location that is a bare remote marker. "Remote - EMEA", "Americas Remote",
//     "Europe & Remote" state a scope the dictionary cannot read, and an office there
//     would narrow a region to one country;
//   - an office is read by its geocoded location, and by its label only when it has none;
//     an office labelled just "Remote" is no place and is skipped;
//   - every other office must resolve to a country (an "EMEA" or "Headquarters" office
//     the dictionary cannot place makes the set unknown), and together they must name
//     exactly one: a list of offices in several countries is the employer's footprint,
//     not this posting's scope.
func greenhouseOfficeCountry(j GreenhousePosting) []string {
	if !isBareRemote(j.Location.Name) {
		return nil
	}
	var country string
	for _, o := range j.Offices {
		loc := strings.TrimSpace(o.Location)
		if isBareRemote(o.Name) && (loc == "" || isBareRemote(loc)) {
			continue
		}
		place := loc
		if place == "" {
			place = o.Name
		}
		cs := location.Parse(place).Countries
		if len(cs) != 1 || (country != "" && cs[0] != country) {
			return nil
		}
		country = cs[0]
	}
	if country == "" {
		return nil
	}
	return []string{country}
}

// isBareRemote reports whether a location string says "remote" and nothing else — no
// place, no region, no scope.
func isBareRemote(s string) bool {
	switch strings.ToLower(strings.Trim(s, " \t()[]-–—:,.|/")) {
	case "remote", "fully remote", "100% remote", "remote location":
		return true
	}
	return false
}

// greenhouseMetadataEmploymentType reads the employer-configured "Employment Type"
// custom field a minority of Greenhouse boards carry (confirmed live: 1800contacts,
// solidpower, carrotfertility all set it to "Full-time") — a genuine structured
// signal, not a free-text guess: the employer explicitly labeled and typed this
// value, the same trust level as Lever's commitment or SmartRecruiters'
// typeOfEmployment. Most boards carry no such field at all.
func greenhouseMetadataEmploymentType(metadata []GreenhouseMetadataField) string {
	for _, m := range metadata {
		if !strings.EqualFold(strings.TrimSpace(m.Name), "Employment Type") {
			continue
		}
		var value string
		if json.Unmarshal(m.Value, &value) != nil {
			return "" // null, an empty array (unset multi-select), or another shape
		}
		return greenhouseEmploymentType(value)
	}
	return ""
}

// greenhouseEmploymentType maps the "Employment Type" field's free-text value (e.g.
// "Full-time", "Part-Time", "1099 Contractor") onto the freehire vocabulary via
// keyword containment — the same style as leverEmploymentType, since the field is
// employer-typed free text, not a fixed enum. "" for an unrecognized value, so the
// description parser decides rather than a guess here.
func greenhouseEmploymentType(value string) string {
	v := strings.ToLower(value)
	switch {
	case strings.Contains(v, "intern"):
		return "internship"
	case strings.Contains(v, "part-time") || strings.Contains(v, "part time"):
		return "part_time"
	case strings.Contains(v, "full-time") || strings.Contains(v, "full time"):
		return "full_time"
	case strings.Contains(v, "contract") || strings.Contains(v, "temporary") || strings.Contains(v, "seasonal"):
		return "contract"
	default:
		return ""
	}
}
