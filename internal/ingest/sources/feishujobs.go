package sources

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"

	"github.com/strelov1/freehire/internal/dict/location"
)

// feishuJobs adapts Feishu Hire (飞书招聘, internally "ATSX"), ByteDance's applicant-tracking
// platform. Multi-tenant: a board is one tenant's careers-site HOST — ByteDance's own
// jobs.bytedance.com, or a customer's <tenant>.jobs.feishu.cn (MiniMax is vrfi1sk8a0) — and
// every tenant's site calls the same keyless JSON search, whose rows carry the whole posting,
// body included. A board is therefore a handful of search requests and no hydration.
//
// The crawl is scoped to the tenant's R&D (研发) job category, the platform's own top-level
// technical bucket. Our title dictionary does not read Chinese titles, so a whole-tenant crawl
// would store thousands of postings nothing could place, while the source's category is a
// structured statement that a posting is engineering. Both hire types are read: experienced
// (社招) and campus (校招: new graduates and interns), since both are real postings.
//
// fullBoardListing: each hire type is walked until the distinct postings read equal the
// search's own count, any page failing fails the board, and a count at the search's result
// window (where it stops counting) fails rather than reading a truncated slice. Every row is
// checked against the hire type and category the request asked for, since a filter the search
// stopped honouring would otherwise read as a much larger board. See the Feishu Jobs traps in
// AGENTS.md.
type feishuJobs struct {
	get      JSONGetter
	post     JSONPoster
	pageSize int
}

// feishuJobsHTTP is the transport the adapter needs: GET for the tenant's filter
// configuration, POST for the search.
type feishuJobsHTTP interface {
	JSONGetter
	JSONPoster
}

const (
	feishuJobsByteDanceHost = "jobs.bytedance.com"
	// feishuJobsPageSize is one search request's rows: the search serves 500 (and more), and a
	// 500-row response is a few MB.
	feishuJobsPageSize = 500
	// feishuJobsWindow is where the search stops counting: an unfiltered query answers
	// count=10000 for a larger catalogue. A count at it is the cap, not the total.
	feishuJobsWindow = 10000
	// feishuJobsTechCategory is the en_name of the platform's top-level technical category.
	// The id differs per tenant; the name comes from the platform's default category template.
	feishuJobsTechCategory = "R&D"
)

// feishuJobsHireTypes are the platform's two top-level recruitment types, the same ids on
// every tenant checked: 1 is experienced hire (社招), 2 is campus (校招). A search with no
// recruitment filter answers experienced postings only, so campus has to be asked for.
var feishuJobsHireTypes = []string{"1", "2"}

// feishuJobsTenantRE is a customer tenant's careers host on the platform's shared domain.
var feishuJobsTenantRE = regexp.MustCompile(`^[a-z0-9-]+\.jobs\.feishu\.cn$`)

// NewFeishuJobs builds the Feishu Jobs adapter over the given HTTP client, which must send a
// macOS browser User-Agent (see macBrowserUserAgent).
func NewFeishuJobs(c feishuJobsHTTP) Source {
	return feishuJobs{get: c, post: c, pageSize: feishuJobsPageSize}
}

func (feishuJobs) Provider() string { return "feishujobs" }

// fullBoardListing: see the type comment and the fullBoardListing interface (source.go).
func (feishuJobs) FullBoardListing() {}

type feishuJobsNode struct {
	ID     string          `json:"id"`
	Name   string          `json:"name"`
	EnName string          `json:"en_name"`
	Parent *feishuJobsNode `json:"parent"`
}

// under reports whether the node or one of its ancestors is in ids. A tenant's category tree
// has no fixed depth: ByteDance's R&D is a top-level node with its disciplines below it, while
// MiniMax's R&D sits under an industry root and carries a third level.
func (n *feishuJobsNode) under(ids map[string]bool) bool {
	for ; n != nil; n = n.Parent {
		if ids[n.ID] {
			return true
		}
	}
	return false
}

// id is the node's id, "" for an absent node.
func (n *feishuJobsNode) id() string {
	if n == nil {
		return ""
	}
	return n.ID
}

type feishuJobsFilters struct {
	Code int `json:"code"`
	Data *struct {
		JobTypes []feishuJobsNode `json:"job_type_list"`
	} `json:"data"`
}

type feishuJobsSearch struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
	Data    *struct {
		Posts []feishuJobsPost `json:"job_post_list"`
		Count *int             `json:"count"`
	} `json:"data"`
}

type feishuJobsPost struct {
	ID          string           `json:"id"`
	Title       string           `json:"title"`
	Description string           `json:"description"`
	Requirement string           `json:"requirement"`
	Category    *feishuJobsNode  `json:"job_category"`
	RecruitType *feishuJobsNode  `json:"recruit_type"`
	PublishTime int64            `json:"publish_time"`
	Cities      []feishuJobsCity `json:"city_list"`
	Info        *struct {
		Addresses []struct {
			Country *struct {
				EnName string `json:"en_name"`
			} `json:"country"`
		} `json:"address_list"`
	} `json:"job_post_info"`
}

type feishuJobsCity struct {
	EnName string `json:"en_name"`
	Name   string `json:"name"`
}

type feishuJobsQuery struct {
	Limit       int      `json:"limit"`
	Offset      int      `json:"offset"`
	Categories  []string `json:"job_category_id_list"`
	Recruitment []string `json:"recruitment_id_list"`
}

func (s feishuJobs) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	host := strings.ToLower(strings.TrimSpace(e.Board))
	if host != feishuJobsByteDanceHost && !feishuJobsTenantRE.MatchString(host) {
		return nil, fmt.Errorf("feishujobs: board %q is not jobs.bytedance.com or a <tenant>.jobs.feishu.cn host", e.Board)
	}
	origin := "https://" + host
	cats, err := s.techCategories(ctx, origin)
	if err != nil {
		return nil, fmt.Errorf("feishujobs: board %q: %w", host, err)
	}
	var jobs []Job
	for _, hire := range feishuJobsHireTypes {
		posts, err := s.walk(ctx, origin, cats, hire)
		if err != nil {
			return nil, fmt.Errorf("feishujobs: board %q, hire type %s: %w", host, hire, err)
		}
		for _, p := range posts {
			if j, ok := p.toJob(host, e.Company, hire, cats); ok {
				jobs = append(jobs, j)
			}
		}
	}
	if len(jobs) == 0 {
		// A tenant whose R&D category lists nothing at all is far more likely a changed search
		// than a company that stopped hiring engineers, and must not read as every posting closed.
		return nil, fmt.Errorf("feishujobs: board %q lists no R&D postings", host)
	}
	return jobs, nil
}

// techCategories reads the ids of the tenant's categories named R&D, at whatever depth, from
// the filter configuration its own careers site loads.
func (s feishuJobs) techCategories(ctx context.Context, origin string) (map[string]bool, error) {
	var f feishuJobsFilters
	if err := s.get.GetJSON(ctx, origin+"/api/v1/config/job/filters/2", &f); err != nil {
		return nil, fmt.Errorf("filters: %w", err)
	}
	if f.Code != 0 || f.Data == nil {
		return nil, fmt.Errorf("filters: code %d", f.Code)
	}
	cats := map[string]bool{}
	for _, t := range f.Data.JobTypes {
		if t.ID != "" && (t.EnName == feishuJobsTechCategory || t.Name == "研发") {
			cats[t.ID] = true
		}
	}
	if len(cats) == 0 {
		return nil, fmt.Errorf("the tenant names no %s category", feishuJobsTechCategory)
	}
	return cats, nil
}

// walk reads one hire type of the tenant's R&D postings in full: until the distinct postings
// read equal the search's count. Any page failing fails the walk.
func (s feishuJobs) walk(ctx context.Context, origin string, cats map[string]bool, hire string) ([]feishuJobsPost, error) {
	ids := make([]string, 0, len(cats))
	for id := range cats {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	var (
		posts []feishuJobsPost
		seen  = map[string]bool{}
		count = -1
	)
	for offset := 0; count < 0 || offset < count; offset += s.pageSize {
		var r feishuJobsSearch
		q := feishuJobsQuery{Limit: s.pageSize, Offset: offset, Categories: ids, Recruitment: []string{hire}}
		if err := s.post.PostJSON(ctx, origin+"/api/v1/search/job/posts", q, &r); err != nil {
			return nil, fmt.Errorf("offset %d: %w", offset, err)
		}
		if r.Code != 0 || r.Data == nil || r.Data.Count == nil {
			return nil, fmt.Errorf("offset %d: code %d %s", offset, r.Code, r.Message)
		}
		if count < 0 {
			count = *r.Data.Count
			if count >= feishuJobsWindow {
				return nil, fmt.Errorf("count %d is the search's result window, not a total", count)
			}
		} else if *r.Data.Count != count {
			return nil, fmt.Errorf("count moved from %d to %d mid-walk", count, *r.Data.Count)
		}
		if count > 0 && len(r.Data.Posts) == 0 {
			return nil, fmt.Errorf("offset %d: empty page before the count of %d was read", offset, count)
		}
		for _, p := range r.Data.Posts {
			// A search that stopped honouring a filter answers a larger, mixed slice; one row
			// outside the asked-for category or hire type means the filter was not applied.
			if !p.Category.under(cats) || !p.RecruitType.under(map[string]bool{hire: true}) {
				return nil, fmt.Errorf("posting %s is category %q, hire type %q, outside the slice",
					p.ID, p.Category.id(), p.RecruitType.id())
			}
			if p.ID == "" || seen[p.ID] {
				continue
			}
			seen[p.ID] = true
			posts = append(posts, p)
		}
	}
	if len(posts) != count {
		return nil, fmt.Errorf("read %d distinct postings of %d", len(posts), count)
	}
	return posts, nil
}

// feishuJobsCategories maps the R&D sub-categories onto vocab.CategoryValues where the
// sub-category names one discipline. "Algorithm", "Client", "Infrastructure", "Multimedia" and
// "Data mining" span several and are left to the title dictionary.
var feishuJobsCategories = map[string]string{
	"Backend":  "backend",
	"Frontend": "frontend",
	// The same disciplines as another tenant's template spells them (MiniMax).
	"Back-end development":       "backend",
	"Front-end development":      "frontend",
	"Operations and maintenance": "devops",
	"Testing":                    "qa",
	"Hardware":                   "hardware",
	"Big data":                   "data_engineering",
	"Security":                   "security",
	"DevOps":                     "devops",
	"Machine learning":           "ml_ai",
	"Computer vision":            "ml_ai",
	"NLP":                        "ml_ai",
}

// feishuJobsCountries maps the address country names the platform spells its own way; any
// other name goes through location.NormalizeCountry.
var feishuJobsCountries = map[string]string{
	"Chinese Mainland": "cn",
	"Hong Kong, China": "hk",
	"Macao, China":     "mo",
	"Taiwan, China":    "tw",
}

func (p feishuJobsPost) toJob(host, company, hire string, cats map[string]bool) (Job, bool) {
	title := strings.Join(strings.Fields(p.Title), " ")
	if p.ID == "" || title == "" || company == "" {
		return Job{}, false
	}
	job := Job{
		ExternalID: p.ID,
		URL:        feishuJobsPostingURL(host, hire, p.ID),
		Title:      title,
		Company:    company,
		Description: sanitizeHTML(plainTextToHTML(p.Description) +
			plainTextToHTML(p.Requirement)),
		PostedAt:   parseEpochMillis(p.PublishTime),
		IsTechHint: true, // the crawl reads only the tenant's R&D category
	}
	// The discipline is the node directly below R&D; R&D itself names none.
	if c := p.Category; c != nil && c.Parent != nil && cats[c.Parent.ID] {
		job.Category = feishuJobsCategories[c.EnName]
	}
	// Campus hire (校招) is new graduates (Regular, 201) and interns (Intern, 202).
	if hire == "2" && p.RecruitType != nil {
		switch p.RecruitType.EnName {
		case "Intern":
			job.EmploymentType, job.Seniority = "internship", "intern"
		case "Regular":
			job.Seniority = "junior"
		}
	}
	job.Location = distinctJoin(p.Cities, "; ", func(c feishuJobsCity) string {
		// "Hong Kong (China)" would also place the posting in mainland China.
		return strings.TrimSuffix(firstNonEmpty(c.EnName, c.Name), " (China)")
	})
	if p.Info != nil {
		seen := map[string]bool{}
		for _, a := range p.Info.Addresses {
			if a.Country == nil {
				continue
			}
			code := feishuJobsCountries[a.Country.EnName]
			if code == "" {
				code = location.NormalizeCountry(a.Country.EnName)
			}
			if code != "" && !seen[code] {
				seen[code] = true
				job.Countries = append(job.Countries, code)
			}
		}
	}
	return job, true
}

// feishuJobsPostingURL is the posting's page on the tenant's careers site. ByteDance serves
// experienced postings under /experienced and campus ones under /campus; a customer tenant
// serves every posting under /index (its /campus route is a 404).
func feishuJobsPostingURL(host, hire, id string) string {
	switch {
	case host != feishuJobsByteDanceHost:
		return "https://" + host + "/index/position/" + id + "/detail"
	case hire == "2":
		return "https://" + host + "/campus/position/" + id + "/detail"
	default:
		return "https://" + host + "/experienced/position/" + id + "/detail"
	}
}
