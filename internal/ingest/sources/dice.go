package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	xhtml "golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/dict/location"
)

// dice adapts Dice (dice.com), the US job board that began as a technology one. The provider is
// a boardless aggregator: one site, one catalogue, and the employer is read per posting.
//
// The listing is the site's own search page, /jobs, which server-renders its results into the
// Next.js flight payload (self.__next_f.push) as a JSON array — the same rows the page draws,
// with the posting's id, employer, location, salary label and work arrangement. The page's
// search API (job-search-api.svc.dhigroupinc.com) is keyed and is not used. A posting page
// carries the full body as a schema.org JobPosting.
//
// The whole catalogue cannot be walked: a search stops at page 25 (750 results of 112k), so a
// run reads a WINDOW — the postings of the last day (postedDate=ONE, ~3.5k) — and cuts it into
// slices of at most 750 by the site's own filters. A posting is therefore re-listed on the runs
// of its first day and whenever Dice reposts it, and the sweep waits diceSweepGrace, the 31
// days Dice itself states as a posting's validThrough. See providers/dice.md.
//
// Dice is not a technology-only board (a tax manager, an HVAC technician and a helpdesk
// technician share a page), so no posting is asserted technical here; the pipeline's
// catalogue filter judges each.
type dice struct {
	http HTMLGetter
	// maxNewPerRun bounds how many posting pages one run hydrates; the rest wait for a later run.
	maxNewPerRun int
}

const (
	diceBase = "https://www.dice.com"
	// dicePageSize and diceMaxPages are the search's own shape: 30 results a page, and a page
	// past 25 answers page 25 again with nothing on it, so a slice larger than their product
	// cannot be read whole and is cut further.
	dicePageSize = 30
	diceMaxPages = 25
	diceMaxSlice = dicePageSize * diceMaxPages
	// diceWindow is the search's narrowest "posted" option: the last day.
	diceWindow = "ONE"
	// diceSweepGrace is Dice's own posting lifetime: a JobPosting states a validThrough exactly
	// 31 days after the posting's modifiedDate.
	diceSweepGrace = 31 * 24 * time.Hour

	diceDefaultMaxNewPerRun = 500
	// The pacer serializes requests, so more workers would only queue on it.
	diceDetailWorkers = 2
)

// Dice answered 40 listing requests at 0.4s apart and the same with the project's own
// User-Agent without a refusal (2026-11-17, from a residential IP). The pace is far below that,
// because a page is ~360 KB and a run is a few hundred of them; it is courtesy, not a measured
// limit.
const (
	diceRequestInterval = time.Second
	diceRequestBurst    = 1
)

// diceDimension is one filter the window is cut by when a slice is too large to read.
type diceDimension struct {
	param  string
	values []string
}

// diceDimensions are tried in order, each splitting only the slices still over diceMaxSlice.
// employerType and easyApply partition the catalogue exactly (their facet counts sum to the
// total); employmentType and workplaceTypes can hold several values on one posting, so their
// slices overlap (the walk de-duplicates) and a posting stating none of them falls out of a
// slice cut by them — which only ever happens inside an already oversized slice. On
// 2026-11-17 the 3,480-posting window needed 19 probes to cut into slices of at most 606.
var diceDimensions = []diceDimension{
	{"employerType", []string{"Direct Hire", "Recruiter", "Other"}},
	{"easyApply", []string{"true", "false"}},
	{"employmentType", []string{"FULLTIME", "PARTTIME", "CONTRACTS", "THIRD_PARTY"}},
	{"workplaceTypes", []string{"On-Site", "Remote", "Hybrid"}},
}

func NewDice(c HTMLGetter) Source {
	return dice{http: c, maxNewPerRun: diceDefaultMaxNewPerRun}
}

func (dice) Provider() string          { return "dice" }
func (dice) Boardless()                {}
func (dice) Aggregator()               {}
func (dice) SweepGrace() time.Duration { return diceSweepGrace }

// diceRow is one result of the search page.
type diceRow struct {
	GUID        string `json:"guid"`
	URL         string `json:"detailsPageUrl"`
	CompanyName string `json:"companyName"`
	Title       string `json:"title"`
	PostedDate  string `json:"postedDate"`
	// Salary is the employer's free text ("USD 140,001.00 - 180,000.00 per year", "120000",
	// "Depends on Experience"); read as a string when it is one.
	Salary         json.RawMessage `json:"salary"`
	EmploymentType string          `json:"employmentType"`
	WorkplaceTypes []string        `json:"workplaceTypes"`
	JobLocation    struct {
		Country     string `json:"country"`
		DisplayName string `json:"displayName"`
	} `json:"jobLocation"`
}

// diceResults is the page's jobList: the rows and what the search states about the whole result.
type diceResults struct {
	Data []diceRow `json:"data"`
	Meta struct {
		PageCount    int `json:"pageCount"`
		TotalResults int `json:"totalResults"`
	} `json:"meta"`
}

func (r diceRow) job() Job {
	modes := map[string]string{"Remote": "remote", "Hybrid": "hybrid", "On-Site": "onsite"}
	j := Job{
		ExternalID:     r.GUID,
		URL:            r.URL,
		Title:          strings.Join(strings.Fields(r.Title), " "),
		Company:        strings.TrimSpace(r.CompanyName),
		Location:       strings.TrimSpace(r.JobLocation.DisplayName),
		PostedAt:       parseRFC3339(r.PostedDate),
		EmploymentType: diceEmploymentType(r.EmploymentType),
	}
	if j.URL == "" {
		j.URL = diceBase + "/job-detail/" + r.GUID
	}
	if c := location.NormalizeCountry(r.JobLocation.Country); c != "" {
		j.Countries = []string{c}
	}
	// A posting stating two arrangements ("Remote, On-Site") states none.
	if len(r.WorkplaceTypes) == 1 {
		j.WorkMode = modes[r.WorkplaceTypes[0]]
	}
	j.Remote = j.WorkMode == "remote"
	var label string
	if json.Unmarshal(r.Salary, &label) == nil {
		diceSalary(&j, label)
	}
	return j
}

// diceEmploymentType reads the label ("Full-time", "Contract, Third Party"). "Third Party" says
// the employer is an agency, not how the work is paid, so it is set aside; what remains states
// an employment type only when it is one.
func diceEmploymentType(label string) string {
	var kept []string
	for _, p := range strings.Split(label, ",") {
		if p = strings.TrimSpace(p); p != "" && p != "Third Party" {
			kept = append(kept, p)
		}
	}
	if len(kept) != 1 {
		return ""
	}
	switch kept[0] {
	case "Full-time":
		return "full_time"
	case "Part-time":
		return "part_time"
	case "Contract":
		return "contract"
	}
	return ""
}

// diceSalaryLabel is the one salary shape Dice renders itself: a currency, a figure or a range,
// and a period. The rest of what employers type there ("120000", "$$70 - $89", "USD116000-144000",
// "Depends on Experience") states no period or no currency and is left alone.
var diceSalaryLabel = regexp.MustCompile(`^([A-Z]{3}) ([\d,]+(?:\.\d+)?)(?: - ([\d,]+(?:\.\d+)?))? per (hour|day|month|year)$`)

func diceSalary(j *Job, label string) {
	m := diceSalaryLabel.FindStringSubmatch(strings.TrimSpace(label))
	if m == nil {
		return
	}
	figure := func(s string) *int {
		f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
		if err != nil {
			return nil
		}
		return roundSalaryPart(f)
	}
	lo, hi := figure(m[2]), figure(m[3])
	if hi == nil {
		hi = lo
	}
	if lo == nil || *lo > *hi {
		return
	}
	j.SalaryMin, j.SalaryMax, j.SalaryCurrency, j.SalaryPeriod = lo, hi, m[1], m[4]
}

// Fetch is the list-only fallback used when the pipeline cannot supply a seen set: every listed
// posting, body-less.
func (s dice) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	jobs := make([]Job, 0, len(rows))
	for _, r := range rows {
		jobs = append(jobs, r.job())
	}
	return jobs, nil
}

func (s dice) FetchNew(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	return s.hydrate(ctx, rows, seen, nil)
}

// FetchNewGated is FetchNew told what the pipeline is going to discard: the postings of an
// employer a first-party ATS already covers. A discarded posting is never stored, so without
// the gate every run would buy its body again.
func (s dice) FetchNewGated(ctx context.Context, _ CompanyEntry, seen func(externalID string) bool, gate DetailGate) ([]Job, error) {
	rows, err := s.list(ctx)
	if err != nil {
		return nil, err
	}
	names := make([]string, 0, len(rows))
	for _, r := range rows {
		names = append(names, strings.TrimSpace(r.CompanyName))
	}
	return s.hydrate(ctx, rows, seen, gate.Covered(names))
}

// list reads the window, cut into slices, and returns each posting once.
//
// The first search page of the whole window failing fails the run. After that the walk is a
// window and not a catalogue: a slice that cannot be read is logged and skipped, since its
// postings are listed again by the next run and the sweep's grace outlasts a gap. A run that
// listed nothing read a changed page, never a quiet market — Dice posts thousands a day.
func (s dice) list(ctx context.Context) ([]diceRow, error) {
	var out []diceRow
	listed := map[string]bool{}
	var failed int
	add := func(res diceResults) {
		for _, r := range res.Data {
			if r.GUID != "" && r.Title != "" && strings.TrimSpace(r.CompanyName) != "" && !listed[r.GUID] {
				listed[r.GUID] = true
				out = append(out, r)
			}
		}
	}
	var walkSlice func(filters url.Values, depth int) error
	walkSlice = func(filters url.Values, depth int) error {
		first, err := s.page(ctx, filters, 1)
		if err != nil {
			return err
		}
		if first.Meta.TotalResults > diceMaxSlice {
			if depth < len(diceDimensions) {
				dim := diceDimensions[depth]
				for _, v := range dim.values {
					next := cloneValues(filters)
					next.Set("filters."+dim.param, v)
					if err := walkSlice(next, depth+1); err != nil {
						if ctx.Err() != nil {
							return ctx.Err()
						}
						failed++
						log.Printf("dice: slice %s: %v; skipped", next.Encode(), err)
					}
				}
				return nil
			}
			log.Printf("dice: slice %s holds %d postings, more than the %d a search serves; the rest wait for a repost",
				filters.Encode(), first.Meta.TotalResults, diceMaxSlice)
		}
		add(first)
		for p := 2; p <= min(first.Meta.PageCount, diceMaxPages); p++ {
			res, err := s.page(ctx, filters, p)
			if err != nil {
				if ctx.Err() != nil {
					return ctx.Err()
				}
				failed++
				log.Printf("dice: slice %s page %d: %v; the slice ends there", filters.Encode(), p, err)
				return nil
			}
			add(res)
		}
		return nil
	}
	window := url.Values{}
	window.Set("filters.postedDate", diceWindow)
	if err := walkSlice(window, 0); err != nil {
		return nil, fmt.Errorf("dice: listing: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("dice: no listing page carried a posting")
	}
	if failed > 0 {
		log.Printf("dice: %d slices or pages could not be read; %d postings listed", failed, len(out))
	}
	return out, nil
}

func cloneValues(v url.Values) url.Values {
	c := make(url.Values, len(v)+1)
	for k, vs := range v {
		c[k] = append([]string(nil), vs...)
	}
	return c
}

// page reads one search page of a slice. The page parameter goes last, as the site's own
// pagination links put it; a page that does not carry the result list is an error, never an
// empty page, so a changed markup cannot pass for a quiet day.
func (s dice) page(ctx context.Context, filters url.Values, page int) (diceResults, error) {
	q := filters.Encode()
	if page > 1 {
		q += "&page=" + strconv.Itoa(page)
	}
	root, err := s.http.GetHTML(ctx, diceBase+"/jobs?"+strings.ReplaceAll(q, "+", "%20"))
	if err != nil {
		return diceResults{}, err
	}
	res, ok := parseDiceResults(root)
	if !ok {
		return diceResults{}, fmt.Errorf("the page carries no result list")
	}
	return res, nil
}

// diceFlightChunk is one self.__next_f.push([1,"<chunk>"]) call, the chunk being a JSON string.
var diceFlightChunk = regexp.MustCompile(`self\.__next_f\.push\(\[1,("(?:[^"\\]|\\.)*")\]\)`)

// parseDiceResults reads the first jobList of the page's flight payload: the script chunks,
// each a JSON string, concatenated, and the object after the "jobList" key decoded.
func parseDiceResults(root *xhtml.Node) (diceResults, bool) {
	var flight strings.Builder
	walk(root, func(n *xhtml.Node) bool {
		if n.Type == xhtml.ElementNode && n.Data == "script" {
			for _, m := range diceFlightChunk.FindAllStringSubmatch(textContent(n), -1) {
				var chunk string
				if json.Unmarshal([]byte(m[1]), &chunk) == nil {
					flight.WriteString(chunk)
				}
			}
			return false
		}
		return true
	})
	const key = `"jobList":`
	payload := flight.String()
	i := strings.Index(payload, key+`{"data":[`)
	if i < 0 {
		return diceResults{}, false
	}
	var res diceResults
	dec := json.NewDecoder(strings.NewReader(payload[i+len(key):]))
	if dec.Decode(&res) != nil {
		return diceResults{}, false
	}
	return res, true
}

// hydrate buys a posting page only for a posting the pipeline will keep, within the run's
// budget. A covered posting is still yielded, body-less, so the pipeline makes and counts the
// decision.
//
// A new posting whose page could not be read is DEFERRED rather than stored body-less: a stored
// row is seen and would never be hydrated again once past pipeline.HydrationRetryWindow, while a
// deferred one is simply new on a later run. Once the edge refuses one request the rest of the
// run's hydration is deferred too: the refusal is about the IP, and every further request would
// be refused alike and only prolong it.
func (s dice) hydrate(ctx context.Context, rows []diceRow, seen func(string) bool,
	covered map[string]bool) ([]Job, error) {
	var out, toHydrate []Job
	deferred := 0
	for _, r := range rows {
		base := r.job()
		switch {
		case seen(r.GUID):
			base.SeenRefresh = true
			out = append(out, base)
		case covered[base.Company]:
			out = append(out, base)
		case len(toHydrate) < s.maxNewPerRun:
			toHydrate = append(toHydrate, base)
		default:
			deferred++
		}
	}
	if deferred > 0 {
		log.Printf("dice: hydration budget %d reached; %d new postings wait for a later run", s.maxNewPerRun, deferred)
	}

	var refused atomic.Bool
	var failed atomic.Int64
	hydrated := fetchDetails(toHydrate, diceDetailWorkers, func(base Job) (Job, bool) {
		if refused.Load() {
			return Job{}, false
		}
		root, err := s.http.GetHTML(ctx, base.URL)
		if err != nil {
			if isRateLimited(err) && !refused.Swap(true) {
				log.Printf("dice: posting pages refused (%v); deferring the rest of this run's hydration", err)
			}
			if detailUnreadable(err) { // a 404/410 is the posting gone, which is an answer
				failed.Add(1)
			}
			return Job{}, false
		}
		job, ok := parseDicePosting(root, base)
		if !ok {
			failed.Add(1)
			log.Printf("dice: %s carries no usable JobPosting; deferring", base.URL)
			return Job{}, false
		}
		return job, true
	})
	if n := failed.Load(); n > 0 {
		log.Printf("dice: %d/%d posting pages could not be read; deferred to a later run", n, len(toHydrate))
	}
	// A run that needed bodies and read none of them is broken, not quiet.
	if failed.Load() > 0 && len(hydrated) == 0 {
		return nil, fmt.Errorf("dice: needed %d posting pages and read none of them", len(toHydrate))
	}
	return append(out, hydrated...), nil
}

// parseDicePosting completes the row's Job with the posting page's body. The page's schema.org
// JobPosting states a single salary figure and no work arrangement, so everything but the body
// stays as the listing stated it.
func parseDicePosting(root *xhtml.Node, base Job) (Job, bool) {
	var p struct {
		Description string `json:"description"`
		DatePosted  string `json:"datePosted"`
	}
	if !ldJobPosting(root, &p) {
		return Job{}, false
	}
	description := sanitizeHTML(p.Description)
	if strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	job := base
	job.Description = description
	if job.PostedAt == nil {
		job.PostedAt = parseRFC3339(p.DatePosted)
	}
	return job, true
}
