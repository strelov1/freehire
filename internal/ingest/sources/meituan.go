package sources

import (
	"context"
	"fmt"
	"strings"
)

// meituan adapts Meituan's careers site, zhaopin.meituan.com. Single-company and boardless.
//
// The crawl is scoped to the site's own 技术类 (Technology) job family: our title dictionary
// cannot read Chinese titles, so the source's family is what states a posting is technical, and
// it sets IsTechHint. Experienced (社招), campus graduate (校招) and intern (实习) postings are
// all read, one walk per hire type, so the type can be stated.
//
// The listing is a keyless JSON POST whose rows carry the whole posting, body included; no
// hydration.
//
// fullBoardListing: each hire type is walked until the distinct postings equal the search's
// count, any page failing or coming back empty early fails the crawl, and a row of another
// family or hire type — a filter the search stopped honouring — fails it too.
type meituan struct {
	http JSONPoster
}

const (
	meituanSearchURL = "https://zhaopin.meituan.com/api/official/job/getJobList"
	meituanJobURL    = "https://zhaopin.meituan.com/web/position/detail?jobUnionId="
	// meituanTechFamily is the code of 技术类 in the site's job-family list
	// (GET /api/official/job/search/enum?enumType=JF).
	meituanTechFamily     = "11001"
	meituanTechFamilyName = "技术类"
	// meituanPageSize is one request's rows; the site serves 500, above any hire type's technology slice.
	meituanPageSize = 500
	// meituanMaxPages bounds a walk well past any hire type; reaching it fails the crawl.
	meituanMaxPages = 20
)

// meituanHireTypes are the site's jobType codes and what each states about a posting.
var meituanHireTypes = []struct{ code, seniority, employment string }{
	{"3", "", ""},                 // 社招, experienced
	{"1", "junior", ""},           // 校招, campus graduate
	{"2", "intern", "internship"}, // 实习, intern
}

// NewMeituan builds the Meituan careers adapter over the given HTTP client.
func NewMeituan(c JSONPoster) Source { return meituan{http: c} }

func (meituan) Provider() string { return "meituan" }

// meituan is single-company, so its config entry carries no board.
func (meituan) Boardless() {}

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (meituan) FullBoardListing() {}

type meituanCode struct {
	Code    string   `json:"code"`
	SubCode []string `json:"subCode"`
}

type meituanPage struct {
	PageNo   int `json:"pageNo"`
	PageSize int `json:"pageSize"`
}

// meituanQuery is the search body. Pagination is nested: a flat pageNo is ignored and always
// answers page 1.
type meituanQuery struct {
	Page         meituanPage   `json:"page"`
	Keywords     string        `json:"keywords"`
	JobShareType string        `json:"jobShareType"`
	JobType      []meituanCode `json:"jobType"`
	CityList     []string      `json:"cityList"`
	Department   []string      `json:"department"`
	JfJgList     []meituanCode `json:"jfJgList"`
	TypeCode     []string      `json:"typeCode"`
	SpecialCode  []string      `json:"specialCode"`
}

type meituanResult struct {
	Status  int    `json:"status"`
	Message string `json:"message"`
	Data    *struct {
		List []meituanPost `json:"list"`
		Page *struct {
			TotalCount *int `json:"totalCount"`
		} `json:"page"`
	} `json:"data"`
}

type meituanPost struct {
	ID          string `json:"jobUnionId"`
	Name        string `json:"name"`
	JobType     string `json:"jobType"`
	JobFamily   string `json:"jobFamily"`
	Duty        string `json:"jobDuty"`
	Requirement string `json:"jobRequirement"`
	HighLight   string `json:"highLight"`
	RefreshTime int64  `json:"refreshTime"`
	CityList    []struct {
		Name string `json:"name"`
	} `json:"cityList"`
}

func (s meituan) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for _, t := range meituanHireTypes {
		posts, err := s.walk(ctx, t.code)
		if err != nil {
			return nil, fmt.Errorf("meituan: hire type %s: %w", t.code, err)
		}
		for _, p := range posts {
			if j, ok := p.toJob(e.Company, t.seniority, t.employment); ok {
				jobs = append(jobs, j)
			}
		}
	}
	if len(jobs) == 0 {
		// The site always lists technology postings; none is a changed search, and must not
		// read as every posting closed.
		return nil, fmt.Errorf("meituan: the technology family lists no postings")
	}
	return jobs, nil
}

// walk reads one hire type's technology postings until the distinct postings equal the count.
func (s meituan) walk(ctx context.Context, jobType string) ([]meituanPost, error) {
	var (
		posts []meituanPost
		seen  = map[string]bool{}
		count = -1
	)
	for page := 1; count < 0 || len(seen) < count; page++ {
		if page > meituanMaxPages {
			return nil, fmt.Errorf("%d pages read %d of %d postings", meituanMaxPages, len(seen), count)
		}
		q := meituanQuery{
			Page:         meituanPage{PageNo: page, PageSize: meituanPageSize},
			JobShareType: "1",
			JobType:      []meituanCode{{Code: jobType, SubCode: []string{}}},
			CityList:     []string{}, Department: []string{}, TypeCode: []string{}, SpecialCode: []string{},
			JfJgList: []meituanCode{{Code: meituanTechFamily, SubCode: []string{}}},
		}
		var r meituanResult
		if err := s.http.PostJSON(ctx, meituanSearchURL, q, &r); err != nil {
			return nil, fmt.Errorf("page %d: %w", page, err)
		}
		if r.Status != 1 || r.Data == nil || r.Data.Page == nil || r.Data.Page.TotalCount == nil {
			return nil, fmt.Errorf("page %d: status %d %s", page, r.Status, r.Message)
		}
		if count < 0 {
			count = *r.Data.Page.TotalCount
			if count == 0 {
				return nil, nil
			}
		}
		// The site has been seen answering a mid-walk request with an empty list; that is a
		// failed page, never the end of the catalogue.
		if len(r.Data.List) == 0 {
			return nil, fmt.Errorf("page %d is empty with %d of %d postings read", page, len(seen), count)
		}
		for _, p := range r.Data.List {
			if p.JobFamily != meituanTechFamilyName || p.JobType != jobType {
				return nil, fmt.Errorf("posting %s is %q, hire type %q, outside the slice", p.ID, p.JobFamily, p.JobType)
			}
			if p.ID == "" || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			posts = append(posts, p)
		}
	}
	if len(seen) != count {
		return nil, fmt.Errorf("read %d distinct postings of %d", len(seen), count)
	}
	return posts, nil
}

func (p meituanPost) toJob(company, seniority, employment string) (Job, bool) {
	title := strings.Join(strings.Fields(p.Name), " ")
	body := plainTextToHTML(p.Duty) + plainTextToHTML(p.Requirement) + plainTextToHTML(p.HighLight)
	if p.ID == "" || title == "" || company == "" || body == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID:     p.ID,
		URL:            meituanJobURL + p.ID,
		Title:          title,
		Company:        company,
		Description:    sanitizeHTML(body),
		PostedAt:       parseEpochMillis(p.RefreshTime),
		Seniority:      seniority,
		EmploymentType: employment,
		IsTechHint:     true, // the crawl reads only the site's technology family
	}
	cities := make([]string, 0, len(p.CityList))
	for _, c := range p.CityList {
		cities = append(cities, c.Name)
	}
	job.Location, job.Countries = cnPlaces(cities)
	return job, true
}
