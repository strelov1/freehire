package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/dict/skilltag"
)

// pyjamahr adapts PyjamaHR (jobs.pyjamahr.com), an Indian multi-tenant ATS. The board is the
// tenant's path segment. The listing is the tenant's internal frontend API on a different host
// (api.pyjamahr.com) than the page; the page's own host (app.pyjamahr.com) answers every static
// endpoint with its SPA's index.html shell. The listing is a plain, keyless GET with DRF-style
// pagination via its own "next" URL. Each item already carries
// id/slug/title/location/workplace_type; the description and the richer structured fields (job
// type, salary, skills, experience) come from a per-posting detail fetch, also a keyless GET.
type pyjamahr struct {
	http JSONGetter
}

// NewPyjamahr builds the PyjamaHR adapter over the given JSON client.
func NewPyjamahr(c JSONGetter) Source { return pyjamahr{http: c} }

func (pyjamahr) Provider() string { return "pyjamahr" }

// pyjamahrMaxPages bounds the "next" walk so a feed that never returns a null next cannot loop
// forever. Reaching it while next is still non-empty is a hard Fetch failure (see list), never a
// silent partial success, which fullBoardListing rests on; manatal.go's next-URL adapter is NOT
// marked fullBoardListing because its ceiling stops silently.
const pyjamahrMaxPages = 500

// fullBoardListing: the "next"-URL walk proves the whole set of open postings by reaching a
// null next; reaching pyjamahrMaxPages while next is still set fails the whole Fetch instead.
// A detail-fetch failure for one posting becomes an Unreadable marker rather than a silent drop.
func (pyjamahr) fullBoardListing() {}

func (s pyjamahr) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	items, err := s.list(ctx, e.Board)
	if err != nil {
		return nil, err
	}
	return fetchDetails(items, defaultDetailWorkers, func(it pyjamahrItem) (Job, bool) {
		return s.detail(ctx, e, it)
	}), nil
}

// pyjamahrItem is one listing posting.
type pyjamahrItem struct {
	ID       int    `json:"id"`
	Slug     string `json:"slug"`
	Title    string `json:"title"`
	Location string `json:"location"`
}

type pyjamahrListResponse struct {
	Next    string         `json:"next"`
	Results []pyjamahrItem `json:"results"`
}

// list walks the listing's own "next" URL to exhaustion.
func (s pyjamahr) list(ctx context.Context, board string) ([]pyjamahrItem, error) {
	next := "https://api.pyjamahr.com/api/career/jobs/?company_slug=" + board + "&page=1"
	var items []pyjamahrItem
	for page := 1; page <= pyjamahrMaxPages; page++ {
		var resp pyjamahrListResponse
		if err := s.http.GetJSON(ctx, next, &resp); err != nil {
			return nil, fmt.Errorf("pyjamahr: board %q page %d: %w", board, page, err)
		}
		items = append(items, resp.Results...)
		if resp.Next == "" {
			return items, nil
		}
		next = resp.Next
	}
	return nil, fmt.Errorf("pyjamahr: board %q: reached the %d-page safety ceiling without finding the board's end",
		board, pyjamahrMaxPages)
}

// pyjamahrDetail is the full posting object a detail fetch returns.
type pyjamahrDetail struct {
	Description     string   `json:"description"`
	JobType         string   `json:"job_type"`
	WorkplaceType   string   `json:"workplace_type"`
	Remote          bool     `json:"remote"`
	MinSalary       *float64 `json:"min_salary"`
	MaxSalary       *float64 `json:"max_salary"`
	Currency        string   `json:"currency"`
	SalaryType      string   `json:"salary_type"`
	IsSalaryVisible bool     `json:"is_salary_visible"`
	Skill           []string `json:"skill"`
	MinExperience   float64  `json:"min_experience"`
	CreatedAt       string   `json:"created_at"`
}

// detail fetches one posting's detail and maps it (with its listing fields) to a Job. A
// transport failure that states nothing about the posting yields an Unreadable marker,
// since the detail is this adapter's only source for the description and structured fields.
func (s pyjamahr) detail(ctx context.Context, e CompanyEntry, it pyjamahrItem) (Job, bool) {
	id := strconv.Itoa(it.ID)
	url := fmt.Sprintf("https://jobs.pyjamahr.com/%s/%s", e.Board, it.Slug)

	apiURL := fmt.Sprintf("https://api.pyjamahr.com/api/career/jobs/%d/?company_slug=%s", it.ID, e.Board)
	var d pyjamahrDetail
	if err := s.http.GetJSON(ctx, apiURL, &d); err != nil {
		if detailUnreadable(err) {
			return unreadableDetail(id, url, e.Company), true
		}
		return Job{}, false
	}

	workMode := firstNonEmpty(workplaceTypeMode(strings.ReplaceAll(d.WorkplaceType, "_", "-")), workModeFromRemote(d.Remote))
	salaryMin, salaryMax, salaryCurrency, salaryPeriod := pyjamahrSalary(d)

	// An explicit 0 is "no prior experience required", a stated fact, not absent data (see
	// internal/job/jobfacts.go ExperienceYearsMin), so it is always set rather than gated behind a
	// truthiness check that would fall back to the description-text heuristic.
	minExperience := int(d.MinExperience)
	experienceMin := &minExperience

	return Job{
		ExternalID:         id,
		URL:                url,
		Title:              strings.TrimSpace(it.Title),
		Company:            e.Company,
		Location:           it.Location,
		Description:        sanitizeHTML(d.Description),
		Remote:             d.Remote || isRemote(it.Title+" "+it.Location),
		WorkMode:           workMode,
		EmploymentType:     pyjamahrEmploymentType(d.JobType),
		Skills:             pyjamahrSkills(d.Skill),
		SalaryMin:          salaryMin,
		SalaryMax:          salaryMax,
		SalaryCurrency:     salaryCurrency,
		SalaryPeriod:       salaryPeriod,
		ExperienceYearsMin: experienceMin,
		PostedAt:           parseRFC3339(d.CreatedAt),
	}, true
}

// pyjamahrSalary reports the detail's salary bounds only when the platform marks them visible and
// both bounds are present with a recognized period: bounds with their unit, or nothing.
func pyjamahrSalary(d pyjamahrDetail) (min, max *int, currency, period string) {
	if !d.IsSalaryVisible || d.MinSalary == nil || d.MaxSalary == nil {
		return nil, nil, "", ""
	}
	switch strings.ToUpper(strings.TrimSpace(d.SalaryType)) {
	case "ANNUAL":
		period = "year"
	case "MONTHLY":
		period = "month"
	default:
		return nil, nil, "", "" // an unrecognized unit would misstate the figure
	}
	minV := int(*d.MinSalary)
	maxV := int(*d.MaxSalary)
	return &minV, &maxV, d.Currency, period
}

// pyjamahrEmploymentType maps PyjamaHR's job_type onto vocab.EmploymentTypeValues. The plain-English
// spellings follow the usual defensive mapping for a business-vocabulary field.
func pyjamahrEmploymentType(t string) string {
	switch strings.ToUpper(strings.TrimSpace(t)) {
	case "FULLTIME", "FULL_TIME", "FULL-TIME":
		return "full_time"
	case "PARTTIME", "PART_TIME", "PART-TIME":
		return "part_time"
	case "CONTRACT", "TEMPORARY", "FREELANCE":
		return "contract"
	case "INTERN", "INTERNSHIP":
		return "internship"
	default:
		return ""
	}
}

// pyjamahrSkills canonicalizes PyjamaHR's raw skill strings through the shared skilltag
// dictionary. Entries are compound phrases ("stakeholder management"), which need mining rather
// than whole-string matching.
func pyjamahrSkills(skills []string) []string {
	return skilltag.Parse(strings.Join(skills, " "), skilltag.WithSkillList())
}
