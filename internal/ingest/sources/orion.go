package sources

import (
	"context"
	"fmt"
	"strconv"
	"strings"
)

// orion adapts Orion Group (orionjobs.com), an international oil & gas / engineering
// recruitment agency. Boardless: one public, keyless JSON API paginates the whole
// catalogue. NOT an aggregator — the API never names a real end client (standard
// recruitment-agency anonymization, confirmed live), so every posting is attributed to
// the agency itself. The listing already carries the full HTML description, so unlike
// emagine.go there is no separate detail fetch.
type orion struct {
	list JSONGetter
}

const (
	orionListingURL = "https://www.orionjobs.com/api/recruitment/job/data/"
	orionBaseURL    = "https://www.orionjobs.com"
	orionCompany    = "Orion Group"
	// orionMaxPages bounds pagination as a backstop, the same role emagineMaxPages plays,
	// in case pagination.total ever understates the true count.
	orionMaxPages = 500
)

// NewOrion builds the Orion Group adapter over the given JSON client.
func NewOrion(c JSONGetter) Source { return orion{list: c} }

func (orion) Provider() string { return "orion" }

func (orion) boardless() {}

func (o orion) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= orionMaxPages; page++ {
		url := fmt.Sprintf("%s?folder=uk&hasexpired=false&page=%d", orionListingURL, page)
		var resp orionPage
		if err := o.list.GetJSON(ctx, url, &resp); err != nil {
			return nil, fmt.Errorf("orion: page %d: %w", page, err)
		}
		if len(resp.Items) == 0 {
			break
		}
		for _, it := range resp.Items {
			if j, ok := it.toJob(); ok {
				jobs = append(jobs, j)
			}
		}
		if resp.Pagination.To >= resp.Pagination.Total {
			break
		}
	}
	return jobs, nil
}

// orionValue is the wrapper every leaf field in Orion's API carries — only Value is used.
type orionValue struct {
	Value string `json:"value"`
}

type orionPage struct {
	Items      []orionItem     `json:"items"`
	Pagination orionPagination `json:"pagination"`
}

type orionPagination struct {
	Total int `json:"total"`
	To    int `json:"to"`
}

type orionItem struct {
	ID             int          `json:"id"`
	URL            string       `json:"url"`
	Title          orionValue   `json:"title"`
	PostDate       orionValue   `json:"postdate"`
	Description    orionValue   `json:"description"`
	LocationText   orionValue   `json:"locationtext"`
	EmploymentType []orionValue `json:"employment_type"`
}

// toJob maps one listing item directly to a Job — the API's listing already carries the
// full description, so this is the whole mapping, not a partial pre-detail stub. An item
// with no id or title is dropped (ok=false): id 0 would collide with every other such
// item's ExternalID, the same risk emagine.toJob's own guard exists to prevent.
func (it orionItem) toJob() (Job, bool) {
	if it.ID == 0 || strings.TrimSpace(it.Title.Value) == "" {
		return Job{}, false
	}
	employmentType := ""
	if len(it.EmploymentType) > 0 {
		employmentType = orionEmploymentType(it.EmploymentType[0].Value)
	}
	return Job{
		ExternalID:     strconv.Itoa(it.ID),
		URL:            orionBaseURL + it.URL,
		Title:          strings.TrimSpace(it.Title.Value),
		Company:        orionCompany,
		Location:       strings.TrimSpace(it.LocationText.Value),
		Description:    sanitizeHTML(it.Description.Value),
		Remote:         isRemote(it.LocationText.Value),
		PostedAt:       parseLayout("02/01/2006", it.PostDate.Value),
		EmploymentType: employmentType,
	}, true
}

// orionEmploymentType maps Orion's employment_type label onto vocab.EmploymentTypeValues.
// "Contract" is confirmed live; "full time"/"part time"/"internship" follow this
// codebase's usual mapping for a self-evident business-vocabulary field (e.g.
// recruiterflowEmploymentType). "permanent" is a UK recruitment-agency synonym for
// full-time added for Orion specifically — a reasonable reading, not independently
// confirmed live the way "Contract" was.
func orionEmploymentType(t string) string {
	switch strings.ToLower(strings.TrimSpace(t)) {
	case "full time", "full-time", "fulltime", "permanent":
		return "full_time"
	case "part time", "part-time", "parttime":
		return "part_time"
	case "contract", "contractor", "temporary", "freelance":
		return "contract"
	case "internship", "intern":
		return "internship"
	default:
		return ""
	}
}
