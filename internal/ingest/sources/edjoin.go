package sources

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// edjoin adapts EDJOIN (edjoin.org), the hiring board California's K-12 school districts,
// county offices of education and community colleges publish on. It is one central index —
// there is no per-district tenancy to crawl — so it is a multi-company aggregator whose
// employer is read from each posting.
//
// THE BOARD IS A JOB-TYPE ID, NOT A DISTRICT. The numeric ids that show up beside an
// employer on edjoin.org are districtID, one more facet of the same central index, and there
// are 1,573 of them. What the board selects here is the slice to crawl — the hh
// (professional_role) and trudvsem (OKATO region) shape, not a tenant. That choice is
// measured, not stylistic (whole platform crawled live 2026-09-02):
//
//   - The platform holds 16,447 open postings and freehire's catalogue filter rejects only
//     38.2% of them by title. The other 10,163 are paraeducators, instructional aides, yard
//     monitors, campus supervisors and cafeteria assistants — classify's non-tech dictionary
//     was written against tech-company ATS boards and carries no term for K-12
//     classified-staff vocabulary, so it admits them. Crawling the whole index would put ten
//     thousand school-support postings into an IT catalogue.
//   - Job type 25, "Information Tech. / Computer Svcs.", is the platform's OWN technology
//     facet and the only technical entry in its 66-value job-type vocabulary. It held 71
//     postings across 59 districts, of which 65 are genuinely IT practice.
//   - The listing carries no body, so every posting costs a 147 KB detail page. A rejected
//     posting is never stored, so it is never `seen`, so a whole-index crawl would buy and
//     throw away ~2.4 GB of detail pages on EVERY run, forever — the trap the ukgready note
//     in this package's AGENTS.md describes, at ten times the ratio. The IT slice costs 71
//     pages.
//
// Traps, all verified live on 2026-09-02:
//
//   - The listing endpoint is /Home/LoadJobs, the site's own XHR. catID, districtID and
//     recruitmentCenterID are dereferenced without a null check: omit any one of the three,
//     or send it empty, and the endpoint answers a 500 .NET NullReferenceException PAGE —
//     HTML, not JSON — however valid the rest of the query is.
//   - totalRecords is exact and stable across page sizes (16,450 at rows=1, 25, 100, 500 and
//     1000 alike), unlike SEEK's totalCount, so it can bound the walk. rows above 1,000
//     answers 500; rows=1 serves 10 rows while still reporting a totalPages computed from 1,
//     so neither the requested page size nor totalPages is worth trusting. There is no result
//     window: the whole 16,447-posting index paginates to the end.
//   - The same posting can appear on two adjacent pages (3 of 16,447 did, ties on the
//     postingDate sort), so the walk dedups on postingID rather than assuming pages partition.
//   - An unknown job-type id is answered 200 with totalRecords 0, not an error, so a mistyped
//     board is indistinguishable from an empty one. The board ids in sources/edjoin.yml carry
//     the platform's own label beside them for that reason.
//   - The body lives only on the posting page, as a schema.org JobPosting in ld+json whose
//     description subsumes the experienceRequirements and skills fields beside it (214/214 and
//     120/120 on the live sample), so nothing else on the page needs reading. Hence
//     HydratingSource.
//   - The pay fields are employer-typed text boxes, not currency-formatted platform fields:
//     alongside "$25.04" they hold "TBD", "Stipend", "District Salary Schedule", "Range 1:
//     $18.30" and "$22 - $26/hr". Only the PERIOD beside them is a picklist. So a figure is
//     read only when the whole value is a bare amount and nothing else — see edjoinAmount.
//   - No metering observed: 300 detail pages at 16-way concurrency (~12 req/s) all answered
//     200 bar one 410 for a taken-down posting, so no pacer is wired. pacer.go is where one
//     would go.
type edjoin struct {
	http edjoinHTTP
}

// edjoinHTTP is the transport edjoin needs: the listing XHR as JSON, and each posting page as
// HTML so its ld+json block can be read.
type edjoinHTTP interface {
	JSONGetter
	HTMLGetter
}

// NewEdjoin builds the EDJOIN adapter over the given HTTP client.
func NewEdjoin(c edjoinHTTP) Source { return edjoin{http: c} }

func (edjoin) Provider() string { return "edjoin" }

// Every district on the platform is its own employer, read from the posting, so edjoin stays
// in the source facet and in the cross-source duplicate suppression set.
func (edjoin) aggregator() {}

const (
	edjoinBaseURL = "https://www.edjoin.org"
	// edjoinPageSize is half the largest page the endpoint serves — asking for more than 1,000
	// rows is refused with an HTTP 500 — so the walk keeps a margin below the refusal and still
	// reads any realistic slice in one or two requests.
	edjoinPageSize = 500
	// edjoinMaxPages bounds the walk well above the whole platform's 33 pages at that size, so
	// a listing change that keeps yielding new ids cannot loop unboundedly.
	edjoinMaxPages = 60
)

// edjoinListing is the /Home/LoadJobs envelope; only the exact total and the rows are read.
type edjoinListing struct {
	TotalRecords int             `json:"totalRecords"`
	Data         []edjoinPosting `json:"data"`
}

// edjoinPosting is one listing row: everything the platform states about a posting except its
// body. The capitalised names are the endpoint's own — the payload mixes casings.
type edjoinPosting struct {
	PostingID     int    `json:"postingID"`
	PositionTitle string `json:"positionTitle"`
	DistrictName  string `json:"districtName"`
	City          string `json:"city"`
	CountyName    string `json:"countyName"`
	StateName     string `json:"stateName"`
	PostingDate   string `json:"postingDate"`
	// FullTimePartTime is a comma-joined multi-select ("Full Time", "Part Time, Temporary",
	// "Part Time, Remote"), holding both the schedule and the platform's remote flag.
	FullTimePartTime string `json:"FullTimePartTime"`
	// SalaryInfoSelect says which pay pair the district filled in: "Pay Range", "Single Rate",
	// or "Dependent" for a posting that publishes no figure.
	SalaryInfoSelect   string `json:"SalaryInfoSelect"`
	PayRangeFrom       string `json:"PayRangeFrom"`
	PayRangeTo         string `json:"PayRangeTo"`
	PayRangeDropdown   string `json:"PayRangeDropdown"`
	SingleRate         string `json:"SingleRate"`
	SingleRateDropdown string `json:"SingleRateDropdown"`
}

// edjoinLDPosting selects the one field the posting page's schema.org block is read for.
type edjoinLDPosting struct {
	Description string `json:"description"`
}

// Fetch hydrates every listed posting. It is what a caller that cannot supply a seen set gets;
// the live pipeline always prefers FetchNew (see internal/ingest/pipeline.fetchBoard). There is
// no cheaper list-only tier to fall back on, because the listing carries no body at all — so
// this is FetchNew with nothing seen rather than a second walk.
func (s edjoin) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	return s.FetchNew(ctx, e, func(string) bool { return false })
}

// FetchNew fetches a posting page only for a posting the catalogue does not already have —
// seen reports whether an id is already ingested. A seen posting yields its list-only job
// flagged SeenRefresh, so the pipeline refreshes liveness without spending a request or
// overwriting the body hydrated when the posting was new. The title travels with that refresh
// on purpose: the refresh path re-applies the catalogue filter to it, which is how a stored
// posting the dictionary now turns away ages out instead of being kept alive by its re-listing.
func (s edjoin) FetchNew(ctx context.Context, e CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	postings, err := s.list(ctx, e)
	if err != nil {
		return nil, err
	}
	return fetchDetails(postings, defaultDetailWorkers, func(p edjoinPosting) (Job, bool) {
		if seen(p.externalID()) {
			base := p.job("")
			base.SeenRefresh = true
			return base, true
		}
		return s.detail(ctx, p)
	}), nil
}

// list walks the job type's slice of the central index and returns every posting it holds. The
// FIRST page failing is a board-level error; a later page failing ends the walk with what was
// gathered, so a mid-listing hiccup costs a page rather than the board. A row with no id or no
// district is skipped: the id is the dedup key and the district is the employer, and a posting
// missing either would be filed under a placeholder that collects unrelated postings.
func (s edjoin) list(ctx context.Context, e CompanyEntry) ([]edjoinPosting, error) {
	var (
		out    []edjoinPosting
		listed = map[int]bool{}
		total  int
	)
	for page := 1; page <= edjoinMaxPages; page++ {
		var resp edjoinListing
		if err := s.http.GetJSON(ctx, edjoinListingURL(e.Board, page), &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("edjoin: listing job type %s: %w", e.Board, err)
			}
			break // a later page failing just ends pagination; the earlier pages still ingest
		}
		if page == 1 {
			total = resp.TotalRecords
		}
		added := 0
		for _, p := range resp.Data {
			if p.PostingID == 0 || strings.TrimSpace(p.DistrictName) == "" || listed[p.PostingID] {
				continue
			}
			listed[p.PostingID] = true
			out = append(out, p)
			added++
		}
		if added == 0 || (total > 0 && len(out) >= total) {
			break
		}
	}
	return out, nil
}

// edjoinListingURL builds the listing XHR's URL for a job type and a 1-based page number.
// catID, districtID and recruitmentCenterID are sent as explicit zeroes because the endpoint
// dereferences all three without a null check and answers a 500 error page without them; the
// remaining filters the site sends are omitted, since leaving one out simply does not narrow
// the search.
func edjoinListingURL(jobType string, page int) string {
	q := url.Values{
		"rows":                {strconv.Itoa(edjoinPageSize)},
		"page":                {strconv.Itoa(page)},
		"sort":                {"postingDate"},
		"sortVal":             {"0"},
		"order":               {"desc"},
		"searchType":          {"all"},
		"jobTypes":            {jobType},
		"days":                {"0"},
		"catID":               {"0"},
		"districtID":          {"0"},
		"recruitmentCenterID": {"0"},
	}
	return edjoinBaseURL + "/Home/LoadJobs?" + q.Encode()
}

// detail fetches one posting page and returns the listing row's job with its body filled in.
// It reports ok=false — so the caller skips just this posting — when the page cannot be
// fetched or carries no usable schema.org body. Deferring a posting by one crawl is
// recoverable; storing it body-less is not, because a stored row is `seen` and so is never
// hydrated again once it ages past the pipeline's hydration-retry window, leaving a posting no
// search can reach (the listing carries no fallback text of any kind).
func (s edjoin) detail(ctx context.Context, p edjoinPosting) (Job, bool) {
	root, err := s.http.GetHTML(ctx, p.url())
	if err != nil {
		return Job{}, false
	}
	var ld edjoinLDPosting
	if !LDJobPosting(root, &ld) {
		return Job{}, false
	}
	description := sanitizeHTML(ld.Description)
	if description == "" {
		return Job{}, false
	}
	return p.job(description), true
}

// job maps a listing row plus the body read from its posting page onto a Job. The employer is
// the posting's own district, never the board file's company — one board is a slice of the
// whole platform, so its entry names the slice rather than an employer.
func (p edjoinPosting) job(description string) Job {
	schedule := edjoinSchedule(p.FullTimePartTime)
	remote := edjoinRemote(schedule)
	j := Job{
		ExternalID:     p.externalID(),
		URL:            p.url(),
		Title:          strings.TrimSpace(p.PositionTitle),
		Company:        strings.TrimSpace(p.DistrictName),
		Location:       p.location(),
		Description:    description,
		Remote:         remote,
		WorkMode:       workModeFromRemote(remote),
		EmploymentType: edjoinEmploymentType(schedule),
		PostedAt:       edjoinDate(p.PostingDate),
	}
	p.applySalary(&j)
	return j
}

// externalID is the platform's own posting id, which is also what its URL is keyed on.
func (p edjoinPosting) externalID() string { return strconv.Itoa(p.PostingID) }

func (p edjoinPosting) url() string {
	return fmt.Sprintf("%s/Home/JobPosting/%d", edjoinBaseURL, p.PostingID)
}

// location renders the posting's place as the location dictionary reads US postings
// ("Salinas, California"). Roughly one posting in sixty states no city, where the county it
// sits in is the narrowest place the listing offers.
func (p edjoinPosting) location() string {
	place := strings.TrimSpace(p.City)
	if place == "" {
		place = strings.TrimSpace(p.CountyName)
	}
	return joinNonEmpty(place, strings.TrimSpace(p.StateName))
}

// edjoinSchedule splits the FullTimePartTime multi-select into the set of options the district
// ticked. It is read ONCE, because the field states two different things — the schedule and the
// platform's remote flag — and reading it under two grammars made them disagree: matching the
// whole string for the schedule while scanning the parts for "Remote" left "Part Time, Remote"
// remote but with no employment type, though it plainly names one.
func edjoinSchedule(field string) map[string]bool {
	set := map[string]bool{}
	for _, part := range strings.Split(field, ",") {
		if opt := strings.TrimSpace(part); opt != "" {
			set[opt] = true
		}
	}
	return set
}

// edjoinRemote reports whether the district ticked the platform's own "Remote" option. It is
// EDJOIN's only structured remote signal, and it is rare — 3 of 16,447 live postings set it,
// which is what a K-12 board looks like. A posting that leaves it unticked says nothing about
// work mode rather than claiming to be onsite.
func edjoinRemote(schedule map[string]bool) bool { return schedule["Remote"] }

// edjoinEmploymentType maps the ticked schedule options onto the freehire vocabulary. A posting
// ticking BOTH schedules — as does the one-word spelling "Full and Part Time" — is the district
// saying the position may be either, so it names no single type. The remaining options are
// orthogonal to the schedule rather than absent from our vocabulary: "Management" is a pay
// class, and "Temporary" is a duration that the platform offers ALONGSIDE a schedule ("Full
// Time, Temporary"), so unlike the single-valued "Temporary" some ATSs expose it is not the
// posting's employment type and does not become "contract".
func edjoinEmploymentType(schedule map[string]bool) string {
	full, part := schedule["Full Time"], schedule["Part Time"]
	switch {
	case full && part, schedule["Full and Part Time"]:
		return ""
	case full:
		return "full_time"
	case part:
		return "part_time"
	default:
		return ""
	}
}

// edjoinSalaryPeriod maps the pay-period picklist onto freehire's salary periods. "Stipend",
// "Bi-weekly" and "Semi-Monthly" have no value here, so an amount qualified by one of them is
// dropped rather than published under a period it does not have.
func edjoinSalaryPeriod(period string) string {
	switch strings.TrimSpace(period) {
	case "Per Hour":
		return "hour"
	case "Daily":
		return "day"
	case "Monthly":
		return "month"
	case "Annually":
		return "year"
	default:
		return ""
	}
}

// edjoinAmountPattern matches a pay value that is a bare money amount and nothing else,
// optionally dollar-prefixed and thousands-separated. The fields it reads are free-text boxes
// a district types into, so the pattern is the whole safeguard: it must reject "TBD",
// "Stipend", "District Salary Schedule", "Range 1: $18.30", "$22 - $26/hr" and "$24.300" (a
// three-decimal typo) while admitting "$25.04", "6,097.87" and "60000". Anchoring both ends
// and capping the fraction at two digits is what does it.
var edjoinAmountPattern = regexp.MustCompile(`^\$?(\d[\d,]*(?:\.\d{1,2})?)$`)

// edjoinAmount parses one bound of a pay range, nil when the value is anything but an amount.
func edjoinAmount(v string) *int {
	m := edjoinAmountPattern.FindStringSubmatch(strings.TrimSpace(v))
	if m == nil {
		return nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", ""), 64)
	if err != nil {
		return nil
	}
	return roundSalaryPart(f)
}

// applySalary maps the district's pay fields onto the job's structured salary. Which pair to
// read is stated by the platform's own radio (SalaryInfoSelect); "Dependent" means the district
// published no figure. A single rate is one amount, so it is written as both bounds, matching
// what the posting states. The currency is stated rather than read off a "$": EDJOIN is a US
// board and every posting's state is a US state.
func (p edjoinPosting) applySalary(j *Job) {
	var from, to, period string
	switch strings.TrimSpace(p.SalaryInfoSelect) {
	case "Pay Range":
		from, to, period = p.PayRangeFrom, p.PayRangeTo, p.PayRangeDropdown
	case "Single Rate":
		from, to, period = p.SingleRate, p.SingleRate, p.SingleRateDropdown
	default:
		return
	}
	unit := edjoinSalaryPeriod(period)
	if unit == "" {
		return
	}
	min, max := edjoinAmount(from), edjoinAmount(to)
	if min == nil && max == nil {
		return
	}
	j.SalaryMin, j.SalaryMax = min, max
	j.SalaryCurrency, j.SalaryPeriod = "USD", unit
}

// edjoinDatePattern matches the .NET JSON date wrapper the listing serves every timestamp in.
var edjoinDatePattern = regexp.MustCompile(`^/Date\((-?\d+)\)/$`)

// edjoinDate reads a listing timestamp into a posted_at. A non-positive epoch is treated as
// absent rather than as 1970 or earlier: the payload spells .NET's DateTime.MinValue as
// /Date(-62135568000000)/, which is the platform's way of saying the field was never set.
func edjoinDate(v string) *time.Time {
	m := edjoinDatePattern.FindStringSubmatch(v)
	if m == nil {
		return nil
	}
	ms, err := strconv.ParseInt(m[1], 10, 64)
	if err != nil || ms <= 0 {
		return nil
	}
	return parseEpochMillis(ms)
}
