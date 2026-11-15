package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// orion adapts Orion Group (orionjobs.com), an oil & gas / engineering recruitment agency.
// Boardless: one public, keyless JSON API paginates the whole catalogue. NOT an aggregator: the
// API never names the end client, so every posting is attributed to the agency. The listing
// carries the full HTML description, so there is no detail fetch.
//
// The listing is fetched as text (TextGetter), not GetJSON: the backend double-encodes its
// response as a JSON string whenever the request carries "Accept: application/json", which
// GetJSON always sends. A different or absent Accept header gets the real object.
type orion struct {
	list TextGetter
}

const (
	orionListingURL = "https://www.orionjobs.com/api/recruitment/job/data/"
	orionBaseURL    = "https://www.orionjobs.com"
	orionCompany    = "Orion Group"
	// orionMaxPages is a backstop in case pagination.total understates the count.
	orionMaxPages = 500
)

// NewOrion builds the Orion Group adapter over the given text client.
func NewOrion(c TextGetter) Source { return orion{list: c} }

func (orion) Provider() string { return "orion" }

func (orion) boardless() {}

func (o orion) Fetch(ctx context.Context, _ CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= orionMaxPages; page++ {
		url := fmt.Sprintf("%s?folder=uk&hasexpired=false&page=%d", orionListingURL, page)
		body, err := o.list.GetText(ctx, url)
		if err != nil {
			return nil, fmt.Errorf("orion: page %d: %w", page, err)
		}
		var resp orionPage
		if err := json.Unmarshal([]byte(body), &resp); err != nil {
			return nil, fmt.Errorf("orion: page %d: decode: %w", page, err)
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

// toJob maps one listing item to a Job. An item with no id or title is dropped: id 0 would
// collide on ExternalID with every other such item.
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
// "permanent" is the UK recruitment-agency synonym for full-time.
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
