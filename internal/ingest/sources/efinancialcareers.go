package sources

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strconv"
	"strings"
	"time"

	"golang.org/x/time/rate"
)

// efinancialcareers adapts eFinancialCareers (efinancialcareers.com), the finance and fintech job
// board. It is boardless and an aggregator: one global catalogue (~66k postings across the .com,
// .co.uk, .de, .sg and .hk sites), the employer read per posting. See
// providers/efinancialcareers.md for the measurements behind each rule.
//
// The listing is the JSON search API behind the site's own frontend
// (job-search-api.efinancialcareers.com). Its rows carry the whole posting, body included, so no
// posting page is ever read. A search serves at most 10,000 results (from + size), so the
// catalogue is read whole by cutting it with the site's own facets until every slice fits:
// country first (it partitions the catalogue exactly), then sector and its sub-sector, then
// seniority.
//
// The sector taxonomy is not a statement of IT-ness (its "Technology" sector holds security
// officers and construction coordinators), so no posting is asserted technical here.
type efinancialcareers struct {
	http    JSONGetter
	limiter waiter
}

const (
	efinancialcareersSearch = "https://job-search-api.efinancialcareers.com/v1/efc/jobs/search"
	efinancialcareersSite   = "https://www.efinancialcareers.com"
	// efinancialcareersMaxSlice is the search's result window: a page past it answers 400.
	efinancialcareersMaxSlice = 10000
	// efinancialcareersPageSize keeps a page near 2 MB; 1000 rows are ~9.5 MB.
	efinancialcareersPageSize = 200
	// efinancialcareersMinShare is the share of the stated total a walk must list. Sectors and
	// seniority overlap or leave gaps, so the walk's own distinct count is what is compared; the
	// 2026-11-22 walk listed 66,007 of 66,047.
	efinancialcareersMinShare = 0.98
	// efinancialcareersRequestInterval is courtesy: 200 requests in a row were never refused.
	efinancialcareersRequestInterval = 250 * time.Millisecond
	efinancialcareersAttempts        = 3
)

// NewEFinancialCareers builds the adapter over the given client.
func NewEFinancialCareers(c JSONGetter) Source {
	return efinancialcareers{
		http:    c,
		limiter: rate.NewLimiter(rate.Every(efinancialcareersRequestInterval), 1),
	}
}

func (efinancialcareers) Provider() string { return "efinancialcareers" }
func (efinancialcareers) Boardless()       {}
func (efinancialcareers) Aggregator()      {}

// fullCatalog: the walk fails on any slice or page that cannot be read, on a slice that stays
// over the search's window after every cut, and on a listing that falls short of the stated
// total, so a finished crawl has seen every posting the site lists.
func (efinancialcareers) FullCatalog() {}

type efcFacetValue struct {
	Value    string `json:"value"`
	Count    int    `json:"count"`
	Children []struct {
		Value string `json:"value"`
		Count int    `json:"count"`
	} `json:"children"`
}

type efcPage struct {
	Data []efcRow `json:"data"`
	Meta struct {
		TotalResults int `json:"totalResults"`
		Facets       []struct {
			Name    string          `json:"facetName"`
			Results []efcFacetValue `json:"facetResults"`
		} `json:"facetQueryResults"`
	} `json:"meta"`
}

type efcRow struct {
	JobID          string `json:"jobId"`
	DetailsPageURL string `json:"detailsPageUrl"`
	Title          string `json:"title"`
	CompanyName    string `json:"companyName"`
	BrandName      string `json:"clientBrandName"`
	PostedDate     string `json:"postedDate"`
	Description    string `json:"description"`
	EmploymentType string `json:"employmentType"`
	PositionType   string `json:"positionType"`
	WorkType       string `json:"workArrangementType"`
	MinSalary      string `json:"minSalary"`
	MaxSalary      string `json:"maxSalary"`
	SalaryCurrency string `json:"salaryCurrency"`
	JobLocation    struct {
		DisplayName string `json:"displayName"`
	} `json:"jobLocation"`
}

func (s efinancialcareers) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	err := s.FetchStream(ctx, e, func(j Job) { jobs = append(jobs, j) })
	return jobs, err
}

// efcDimensions are the cuts tried in order on a slice over the window. Each names the facet
// that lists the slice's values and the filter that selects one. Country partitions the
// catalogue exactly (64 values summing to the total on 2026-11-22); sector is multi-valued, so
// its slices overlap and the walk de-duplicates; seniority leaves a gap and is only the last
// resort.
var efcDimensions = []struct{ facet, filter string }{
	{"countryId", "filters.countryId"},
	{"sectors", "filters.sectors"},
	{"sectors", "filters.sectors"}, // the sub-sectors of the sector already chosen
	{"seniority", "filters.seniority"},
}

// FetchStream reads the whole catalogue, slice by slice, and emits each posting once as its page
// arrives. Any failure fails the run, because the adapter is swept by source: a listing missing
// a slice would close every posting in it. That includes a listing that, once de-duplicated,
// falls more than 2% short of the total the search states.
func (s efinancialcareers) FetchStream(ctx context.Context, _ CompanyEntry, emit func(Job)) error {
	listed := map[string]bool{}
	unreadable := map[string]bool{}
	walk := func(filters url.Values, total int) error {
		for p := 1; (p-1)*efinancialcareersPageSize < total; p++ {
			page, err := s.page(ctx, filters, p, "")
			if err != nil {
				return fmt.Errorf("slice %s page %d: %w", filters.Encode(), p, err)
			}
			for _, r := range page.Data {
				if r.JobID == "" || listed[r.JobID] {
					continue
				}
				j, ok := r.job()
				if !ok {
					unreadable[r.JobID] = true
					continue
				}
				listed[r.JobID] = true
				delete(unreadable, r.JobID)
				emit(j)
			}
			if len(page.Data) < efinancialcareersPageSize {
				return nil
			}
		}
		return nil
	}
	var cut func(filters url.Values, total, depth int) error
	cut = func(filters url.Values, total, depth int) error {
		if total <= efinancialcareersMaxSlice {
			return walk(filters, total)
		}
		if depth >= len(efcDimensions) {
			return fmt.Errorf("slice %s holds %d postings, more than a search serves", filters.Encode(), total)
		}
		dim := efcDimensions[depth]
		probe, err := s.page(ctx, filters, 1, dim.facet)
		if err != nil {
			return fmt.Errorf("slice %s: %w", filters.Encode(), err)
		}
		values := efcFacet(probe, dim.facet, filters.Get(dim.filter))
		if len(values) == 0 {
			// A dimension that cannot cut this slice (a sector without sub-sectors) hands it on.
			return cut(filters, total, depth+1)
		}
		for _, v := range values {
			next := cloneValues(filters)
			next.Set(dim.filter, v.Value)
			if err := cut(next, v.Count, depth+1); err != nil {
				return err
			}
		}
		return nil
	}

	root, err := s.page(ctx, url.Values{}, 1, "countryId")
	if err != nil {
		return fmt.Errorf("efinancialcareers: listing: %w", err)
	}
	if err := cut(url.Values{}, root.Meta.TotalResults, 0); err != nil {
		return fmt.Errorf("efinancialcareers: %w", err)
	}
	if want := float64(root.Meta.TotalResults) * efinancialcareersMinShare; float64(len(listed)) < want || len(listed) == 0 {
		return fmt.Errorf("efinancialcareers: listed %d distinct postings of the %d the search states", len(listed), root.Meta.TotalResults)
	}
	// Overlapping facet slices may later supply a usable copy of an unreadable row.
	for id := range unreadable {
		emit(unreadableDetail(id, "", ""))
	}
	log.Printf("efinancialcareers: listed %d distinct postings of %d", len(listed), root.Meta.TotalResults)
	return nil
}

// efcFacet reads one facet of a probe page. For a facet already chosen (parent non-empty, the
// sector filter) it returns that value's children; otherwise the top-level values.
func efcFacet(p efcPage, name, parent string) []efcFacetValue {
	var out []efcFacetValue
	for _, f := range p.Meta.Facets {
		if f.Name != name {
			continue
		}
		for _, r := range f.Results {
			switch {
			case parent == "" && r.Count > 0:
				out = append(out, efcFacetValue{Value: r.Value, Count: r.Count})
			case parent != "" && r.Value == parent:
				for _, c := range r.Children {
					if c.Count > 0 {
						out = append(out, efcFacetValue{Value: c.Value, Count: c.Count})
					}
				}
			}
		}
	}
	return out
}

// page reads one search page, newest first. A non-empty facet makes it a one-row probe that
// carries that facet's counts for the slice.
func (s efinancialcareers) page(ctx context.Context, filters url.Values, page int, facet string) (efcPage, error) {
	q := cloneValues(filters)
	q.Set("sortBy", "POSTED_DATE")
	q.Set("pageSize", strconv.Itoa(efinancialcareersPageSize))
	q.Set("page", strconv.Itoa(page))
	if facet != "" {
		q.Set("facets", facet)
		q.Set("pageSize", "1")
		q.Set("page", "1")
	}
	var res efcPage
	err := retryWhile(ctx, efinancialcareersAttempts-1, time.Second, isRefusedOrTransient, func() error {
		if err := s.limiter.Wait(ctx); err != nil {
			return err
		}
		res = efcPage{}
		return s.http.GetJSON(ctx, efinancialcareersSearch+"?"+q.Encode(), &res)
	})
	return res, err
}

// efcEmploymentTypes maps the row's positionType; a permanent or temporary post states no
// vocabulary value of its own.
var efcEmploymentTypes = map[string]string{
	"Contract":                       "contract",
	"Internships & Graduate Trainee": "internship",
}

func (r efcRow) job() (Job, bool) {
	title := strings.Join(strings.Fields(r.Title), " ")
	company := strings.Join(strings.Fields(firstNonEmpty(r.CompanyName, r.BrandName)), " ")
	description := sanitizeHTML(plainTextToHTML(r.Description))
	if r.JobID == "" || r.DetailsPageURL == "" || title == "" || company == "" ||
		strings.TrimSpace(textFromHTML(description)) == "" {
		return Job{}, false
	}
	j := Job{
		ExternalID:     r.JobID,
		URL:            efinancialcareersSite + r.DetailsPageURL,
		Title:          title,
		Company:        company,
		Location:       strings.TrimSpace(r.JobLocation.DisplayName),
		Description:    description,
		PostedAt:       NotFuture(parseRFC3339(r.PostedDate)),
		EmploymentType: efcEmploymentTypes[r.PositionType],
		WorkMode:       workplaceTypeMode(strings.NewReplacer("In-Office", "onsite").Replace(r.WorkType)),
	}
	if j.EmploymentType == "" {
		switch r.EmploymentType {
		case "Full time":
			j.EmploymentType = "full_time"
		case "Part time":
			j.EmploymentType = "part_time"
		}
	}
	j.Remote = j.WorkMode == "remote"
	r.salary(&j)
	return j, true
}

// salary takes the structured range of a permanent post. The row states no period, and a
// contract's range can be a day rate, so only a permanent post whose figures are annual-sized
// is read, as a yearly salary; "Competitive" arrives as 0 and sets nothing.
func (r efcRow) salary(j *Job) {
	lo, _ := strconv.ParseFloat(r.MinSalary, 64)
	hi, _ := strconv.ParseFloat(r.MaxSalary, 64)
	if r.PositionType != "Permanent" || r.SalaryCurrency == "" || lo < 10000 || hi < lo {
		return
	}
	j.SalaryMin, j.SalaryMax = roundSalaryPart(lo), roundSalaryPart(hi)
	j.SalaryCurrency, j.SalaryPeriod = r.SalaryCurrency, "year"
}
