package sources

import (
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

// trudvsem adapts opendata.trudvsem.ru — the open-data API of "Работа России", the Russian
// federal employment service's job board. It is keyless. Postings are enumerated per federal
// subject (region), carried as the board file entry's board: a 13-digit OKATO region code
// (e.g. 7700000000000 = Moscow). Multi-company (employer per posting), board-based like
// arbeitsagentur — no new source shape needed.
//
// The API hard-caps pagination — the offset param is a 0-based page index, page size maxes at
// 100, and page 100 errors — so one query reaches only the freshest 10k of a listing (results
// are date_modify-descending). Six regions hold more than that (Krasnodar ~20k, Moscow ~19k,
// measured 2026-10-30), so a region past the ceiling is read in date WINDOWS: the API's
// modifiedTo bound selects what is older than the oldest posting the previous query reached,
// and each window is walked to its own end. Until then the tail past 10k was never listed, so
// nothing could tell its live postings from its dead ones (see fullBoardListing below).
type trudvsem struct {
	http JSONGetter
}

const (
	trudvsemRegionURL = "https://opendata.trudvsem.ru/api/v1/vacancies/region/%s"
	trudvsemPageSize  = 100
	// trudvsemMaxPages is the API's pagination ceiling. The offset param is a 0-based PAGE index
	// (not a record offset); page index 100 (offset=100) errors with a 500, so at most 100 pages
	// — the freshest 10k postings — are reachable per query.
	trudvsemMaxPages = 100
	// trudvsemWindowOverlap widens each window's modifiedTo bound past the oldest date_modify the
	// previous query read. The listing is only roughly sorted (adjacent postings were measured a
	// second out of order) and the bound's inclusivity is undocumented, so a bound exactly at that
	// date could skip a posting; the overlap's repeats cost a page or so and are dropped by id.
	trudvsemWindowOverlap = time.Hour
	// trudvsemModifyLayout is date_modify's format: "2026-10-30T13:39:45+0300".
	trudvsemModifyLayout = "2006-01-02T15:04:05-0700"
)

// NewTrudvsem builds the Trudvsem adapter over the given JSON client.
func NewTrudvsem(c JSONGetter) Source { return trudvsem{http: c} }

func (trudvsem) Provider() string { return "trudvsem" }

// fullBoardListing: Fetch walks every window of a region to its natural end (a short page, or
// the stated total covered), splits past the API's page ceiling instead of stopping at it, and
// fails the board on any page error rather than returning the pages already read. Postings the
// listing reorders mid-walk (a posting edited during the crawl jumps to the head of the
// listing) can be missed by one crawl; the sweep's 48h unseen window, against a region crawled
// several times a day, is what absorbs that. See the fullBoardListing interface for the bar.
func (trudvsem) fullBoardListing() {}

// trudvsemResponse is one vacancies page. A page past the end of a region's results 500s
// (surfaced as a transport error), so the loop stops before requesting one by watching the
// per-page count and the reported total.
type trudvsemResponse struct {
	Meta struct {
		Total int `json:"total"`
	} `json:"meta"`
	Results struct {
		Vacancies []struct {
			Vacancy trudvsemVacancy `json:"vacancy"`
		} `json:"vacancies"`
	} `json:"results"`
}

// trudvsemVacancy is one posting. Only the fields the adapter maps are decoded.
type trudvsemVacancy struct {
	ID     string `json:"id"`
	Region struct {
		Name string `json:"name"`
	} `json:"region"`
	Company struct {
		Name string `json:"name"`
	} `json:"company"`
	CreationDate string `json:"creation-date"`
	DateModify   string `json:"date_modify"`
	JobName      string `json:"job-name"`
	VacURL       string `json:"vac_url"`
	Employment   string `json:"employment"`
	Requirements string `json:"requirements"`
	Duty         string `json:"duty"`
	// SalaryMin/SalaryMax are the employer's own monthly bounds in roubles; 0 means
	// unstated. The free-text "salary" beside them always reads "от N" even for a real
	// range, so the numbers are the signal and the text is not read.
	SalaryMin *float64 `json:"salary_min"`
	SalaryMax *float64 `json:"salary_max"`
}

// trudvsemEmployment maps Trudvsem's employment enum to freehire's controlled EmploymentType
// vocabulary. Only the confidently-known values are listed; an unrecognized value maps to ""
// so the dictionaries decide, rather than guessing at a mapping the board does not warrant
// (e.g. temporary/seasonal has no clean equivalent).
var trudvsemEmployment = map[string]string{
	"Полная занятость":    "full_time",
	"Частичная занятость": "part_time",
	"Стажировка":          "internship",
}

// trudvsemRemoteEmployment is the employment value under which the board files remote work —
// the Labour Code's "дистанционная работа", picked from the same closed list as the values
// above. It names where the work is done, not its hours, so it is the posting's structured work
// mode rather than an employment type. No other employment value or schedule states a work
// mode ("Полный рабочий день" is hours), so everything else is left to the dictionaries.
const trudvsemRemoteEmployment = "Дистанционная (удаленная) работа"

// Fetch reads a region's whole listing, one date window at a time (see the type comment). Any
// page failing fails the board: the region is a fullBoardListing, so a partial result returned
// as a success would let the board-scoped sweep close every posting the walk did not reach.
func (t trudvsem) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	w := trudvsemWalk{seen: map[string]bool{}}
	modifiedTo, prevTotal := "", 0
	for {
		total, capped, err := t.walkWindow(ctx, e, modifiedTo, &w)
		if err != nil {
			return nil, err
		}
		if !capped {
			return w.jobs, nil
		}
		// Past the ceiling: the next window must hold strictly less than this one, or the walk
		// would read the same ceiling again forever.
		if prevTotal > 0 && total >= prevTotal {
			return nil, fmt.Errorf("trudvsem: region %q: window modifiedTo=%s holds %d postings, no fewer than the %d before it",
				e.Board, modifiedTo, total, prevTotal)
		}
		if w.oldest.IsZero() {
			return nil, fmt.Errorf("trudvsem: region %q: %d postings past the page ceiling and no date_modify to split them by",
				e.Board, total)
		}
		prevTotal = total
		modifiedTo = w.oldest.Add(trudvsemWindowOverlap).UTC().Format(time.RFC3339)
	}
}

// trudvsemWalk is what one region's walk has gathered across its windows: the postings, each
// once (windows overlap), and the oldest date_modify read so far, which bounds the next window.
type trudvsemWalk struct {
	jobs   []Job
	seen   map[string]bool
	oldest time.Time
}

// walkWindow walks one window of a region — the whole listing when modifiedTo is "" — to its
// end, appending to w. total is the window's size as the API stated it; capped reports that
// the walk stopped at the page ceiling with the window not yet exhausted.
func (t trudvsem) walkWindow(ctx context.Context, e CompanyEntry, modifiedTo string, w *trudvsemWalk) (total int, capped bool, err error) {
	for page := 0; page < trudvsemMaxPages; page++ {
		var resp trudvsemResponse
		if err := t.http.GetJSON(ctx, t.pageURL(e.Board, modifiedTo, page), &resp); err != nil {
			return 0, false, fmt.Errorf("trudvsem: region %q modifiedTo=%q page %d: %w", e.Board, modifiedTo, page, err)
		}
		if page == 0 {
			total = resp.Meta.Total
		}
		for _, item := range resp.Results.Vacancies {
			v := item.Vacancy
			if m, err := time.Parse(trudvsemModifyLayout, strings.TrimSpace(v.DateModify)); err == nil &&
				(w.oldest.IsZero() || m.Before(w.oldest)) {
				w.oldest = m
			}
			if w.seen[v.ID] {
				continue
			}
			if j, ok := t.toJob(v, e.Company); ok {
				w.seen[v.ID] = true
				w.jobs = append(w.jobs, j)
			}
		}
		// Stop at a short/empty page (the last one) or once the reported total is covered — the
		// API pages by index, so after this page we have seen (page+1)*pageSize records. Both
		// guards keep the loop from requesting a page past the end, which 500s.
		if len(resp.Results.Vacancies) < trudvsemPageSize || (page+1)*trudvsemPageSize >= resp.Meta.Total {
			return total, false, nil
		}
	}
	return total, true, nil
}

// pageURL builds a region request for the given 0-based page index (the API's offset param is
// a page number, not a record offset), bounded by modifiedTo when it is set.
func (trudvsem) pageURL(region, modifiedTo string, page int) string {
	q := url.Values{}
	q.Set("limit", strconv.Itoa(trudvsemPageSize))
	q.Set("offset", strconv.Itoa(page))
	if modifiedTo != "" {
		q.Set("modifiedTo", modifiedTo)
	}
	return fmt.Sprintf(trudvsemRegionURL, region) + "?" + q.Encode()
}

// toJob maps a vacancy to a Job, dropping one with no id or blank title. hubName is the board
// entry's company, used only as the employer fallback when a posting omits its own.
func (trudvsem) toJob(v trudvsemVacancy, hubName string) (Job, bool) {
	if v.ID == "" || strings.TrimSpace(v.JobName) == "" {
		return Job{}, false
	}
	employment := strings.TrimSpace(v.Employment)
	var workMode string
	if employment == trudvsemRemoteEmployment {
		workMode = "remote"
	}
	salMin, salMax, salCurrency, salPeriod := rubMonthlySalary(v.SalaryMin, v.SalaryMax)
	return Job{
		ExternalID:     v.ID,
		URL:            strings.TrimSpace(v.VacURL),
		Title:          strings.TrimSpace(v.JobName),
		Company:        firstNonEmpty(strings.TrimSpace(v.Company.Name), hubName),
		Location:       strings.TrimSpace(v.Region.Name),
		Description:    trudvsemDescription(v),
		Remote:         workMode == "remote",
		WorkMode:       workMode,
		EmploymentType: trudvsemEmployment[employment],
		PostedAt:       parseDate(strings.TrimSpace(v.CreationDate)),
		SalaryMin:      salMin,
		SalaryMax:      salMax,
		SalaryCurrency: salCurrency,
		SalaryPeriod:   salPeriod,
	}, true
}

// trudvsemDescription assembles the posting body from its duty and requirements text — both
// plain text with no markup — under Russian headings, then rebuilds paragraph/list structure
// via plainTextToHTML so it does not render as one unbroken block. An empty result is fine:
// a description-less posting is still worth emitting.
func trudvsemDescription(v trudvsemVacancy) string {
	var b strings.Builder
	if duty := strings.TrimSpace(v.Duty); duty != "" {
		b.WriteString("Обязанности:\n" + duty)
	}
	if req := strings.TrimSpace(v.Requirements); req != "" {
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString("Требования:\n" + req)
	}
	if b.Len() == 0 {
		return ""
	}
	return sanitizeHTML(plainTextToHTML(b.String()))
}
