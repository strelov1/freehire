package sources

import (
	"context"
	"fmt"
	"net/url"
	"strings"
)

// speedrunBaseURL is the a16z speedrun talent-network public API root.
const speedrunBaseURL = "https://speedrun-talent-network.com/api/v1"

// speedrunSourceTag identifies our crawler to the API, which asks every integration to name
// itself so it can tell agent traffic apart.
const speedrunSourceTag = "freehire"

// speedrun adapts the a16z speedrun talent-network API. The network is mostly a federating
// directory — a portfolio company's roles usually live on its own Ashby/Greenhouse/Lever board,
// which we crawl natively — so the boards listed here are the early-stage companies that have
// no ATS of their own yet and host their application on the network itself. Those roles exist
// nowhere else.
//
// Board is the company slug. The company endpoint enumerates its live roles (the ?company=
// filter takes the display name, which makes a poor board key), and each role's body, work
// arrangement and application target come from its own detail endpoint.
type speedrun struct {
	http JSONGetter
}

// NewSpeedrun builds the speedrun talent-network adapter over the given HTTP client.
func NewSpeedrun(c JSONGetter) Source { return speedrun{http: c} }

func (speedrun) Provider() string { return "speedrun" }

// speedrunCompany is a company page: the directory record plus the ids of its live roles. Only
// the ids matter — every field we map comes from the role's own detail payload.
type speedrunCompany struct {
	Company struct {
		Jobs []speedrunListItem `json:"jobs"`
	} `json:"company"`
}

// speedrunListItem is one listed role: the id that addresses its detail endpoint.
type speedrunListItem struct {
	ID string `json:"id"`
}

// speedrunJob is one role's detail record. Status flags a role taken down since the listing;
// description_text is the body as plain text; apply carries the application target, which is
// the company's own ATS for a federated role and the network's page for a hosted one.
type speedrunJob struct {
	Job struct {
		ID             string `json:"id"`
		Status         string `json:"status"`
		URL            string `json:"url"`
		Title          string `json:"title"`
		Location       string `json:"location"`
		WorkplaceType  string `json:"workplace_type"`
		EmploymentType string `json:"employment_type"`
		Remote         bool   `json:"remote"`
		CompSummary    string `json:"comp_summary"`
		PublishedAt    string `json:"published_at"`
		Apply          struct {
			URL string `json:"url"`
		} `json:"apply"`
		DescriptionText string `json:"description_text"`
	} `json:"job"`
}

func (s speedrun) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	listURL := fmt.Sprintf("%s/companies/%s?source=%s", speedrunBaseURL, url.PathEscape(e.Board), speedrunSourceTag)

	var company speedrunCompany
	if err := s.http.GetJSON(ctx, listURL, &company); err != nil {
		return nil, fmt.Errorf("speedrun: company %s: %w", e.Board, err)
	}

	return fetchDetails(company.Company.Jobs, defaultDetailWorkers, func(it speedrunListItem) (Job, bool) {
		return s.detail(ctx, e, it.ID)
	}), nil
}

// detail fetches one role and maps it, returning ok=false when the id is missing, the fetch
// fails, or the role has since closed — so the caller drops just that role.
func (s speedrun) detail(ctx context.Context, e CompanyEntry, id string) (Job, bool) {
	if id = strings.TrimSpace(id); id == "" {
		return Job{}, false
	}
	var detail speedrunJob
	jobURL := fmt.Sprintf("%s/jobs/%s?source=%s", speedrunBaseURL, url.PathEscape(id), speedrunSourceTag)
	if err := s.http.GetJSON(ctx, jobURL, &detail); err != nil {
		return Job{}, false
	}
	j := detail.Job
	if j.Status == "closed" || strings.TrimSpace(j.Title) == "" {
		return Job{}, false
	}

	location := strings.TrimSpace(j.Location)
	workMode := workplaceTypeMode(j.WorkplaceType)
	return Job{
		ExternalID: id,
		// Where one actually applies: the company's own ATS when the role federates out,
		// the network's own page when it hosts the application.
		URL:            firstNonEmpty(strings.TrimSpace(j.Apply.URL), strings.TrimSpace(j.URL)),
		Title:          strings.TrimSpace(j.Title),
		Company:        e.Company,
		Location:       location,
		Description:    speedrunDescription(j.CompSummary, j.DescriptionText),
		Remote:         j.Remote || isRemote(location),
		WorkMode:       workMode,
		EmploymentType: speedrunEmploymentType(j.EmploymentType),
		PostedAt:       parseRFC3339OrDate(j.PublishedAt),
	}, true
	// Seniority and Category stay unset on purpose: the API's function/seniority labels are
	// the network's own normalization of the title, not a field the employer filled in, and
	// an adapter-set facet outranks the pipeline's dictionaries. Its coarse "engineering"
	// would overwrite a precise backend/ml_ai classification, so the dictionaries decide.
}

// speedrunDescription renders the body, leading with the compensation band because Job has no
// salary field. The body is plain text, so it is rebuilt into the paragraphs and lists the
// renderer expects before sanitizing.
func speedrunDescription(compSummary, text string) string {
	var b strings.Builder
	if c := strings.TrimSpace(compSummary); c != "" {
		b.WriteString(plainTextToHTML(c))
	}
	b.WriteString(plainTextToHTML(text))
	return sanitizeHTML(b.String())
}

// speedrunEmploymentType maps the network's employmentType enum onto the freehire vocabulary,
// returning "" for an unknown/absent value so the description parser decides.
func speedrunEmploymentType(t string) string {
	switch t {
	case "FullTime":
		return "full_time"
	case "PartTime":
		return "part_time"
	case "Contract", "Temporary":
		return "contract"
	case "Intern", "Internship":
		return "internship"
	default:
		return ""
	}
}
