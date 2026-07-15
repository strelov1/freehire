package sources

import (
	"context"
	"fmt"
	"strings"
)

// spark adapts Spark.work's public career API (hub.spark.work), the ATS behind
// <workspace>.spark.work career sites. A board is the workspace slug (the subdomain),
// passed on every request as the X-workspace header, which is how the shared API resolves
// the tenant. The listing carries each posting's rich fields inline, so no per-posting
// detail call is needed.
type spark struct {
	http HeaderJSONGetter
}

const (
	sparkListURL    = "https://hub.spark.work/api/jobOpenings?pageNumber=%d&pageSize=%d"
	sparkVacancyURL = "https://%s.spark.work/career/job/%d/%s"
	sparkPageSize   = 100
	// sparkMaxPages bounds the page walk well above any single workspace's postings.
	sparkMaxPages = 100
	// sparkActive is the vacancyStatusId marking a live posting (the platform's status enum
	// is Upcoming=10, Active=20, Closed=30, Discarded=40, Expired=50); anything else is skipped.
	sparkActive = 20
)

// NewSpark builds the Spark.work adapter over the given HTTP client.
func NewSpark(c HeaderJSONGetter) Source { return spark{http: c} }

func (spark) Provider() string { return "spark" }

// sparkJob is one posting from the listing. name and openDate are stable top-level fields;
// fields is a form-template-keyed map (numeric field ids -> string values, some null) whose
// ids vary by workspace, so its semantics are recovered by content, not by id (see sparkBody
// and sparkMode).
type sparkJob struct {
	ID              int64             `json:"id"`
	Name            string            `json:"name"`
	VacancyStatusID int               `json:"vacancyStatusId"`
	OpenDate        string            `json:"openDate"`
	Fields          map[string]string `json:"fields"`
}

func (s spark) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	var jobs []Job
	for page := 1; page <= sparkMaxPages; page++ {
		var resp struct {
			Items     []sparkJob `json:"items"`
			PageCount int        `json:"pageCount"`
		}
		url := fmt.Sprintf(sparkListURL, page, sparkPageSize)
		if err := s.http.GetJSONWithHeaders(ctx, url, sparkWorkspace(e.Board), &resp); err != nil {
			if page == 1 {
				return nil, fmt.Errorf("spark: list workspace %s: %w", e.Board, err)
			}
			break // partial: keep what earlier pages yielded
		}
		for _, it := range resp.Items {
			if it.VacancyStatusID != sparkActive {
				continue
			}
			mode := sparkMode(it.Fields)
			jobs = append(jobs, Job{
				ExternalID:  fmt.Sprintf("%d", it.ID),
				URL:         fmt.Sprintf(sparkVacancyURL, e.Board, it.ID, sparkSlug(it.Name)),
				Title:       strings.TrimSpace(it.Name),
				Company:     e.Company,
				Description: sparkBody(it.Fields),
				Remote:      mode == "remote",
				WorkMode:    mode,
				PostedAt:    parseRFC3339(it.OpenDate),
			})
		}
		if resp.PageCount > 0 && page >= resp.PageCount {
			break
		}
		if len(resp.Items) < sparkPageSize {
			break
		}
	}
	return jobs, nil
}

// sparkWorkspace is the per-tenant routing header every Spark.work call carries.
func sparkWorkspace(board string) map[string]string {
	return map[string]string{"X-workspace": board}
}

// sparkBody recovers the description from the form-template fields by content rather than by
// a workspace-specific field id: the description is the rich-text field, so it is the longest
// value carrying HTML markup. The value is literal (not entity-escaped) HTML, so it goes
// straight to the sanitizer. Returns "" when no field looks like rich text.
func sparkBody(fields map[string]string) string {
	var best string
	for _, v := range fields {
		if strings.Contains(v, "<") && len(v) > len(best) {
			best = v
		}
	}
	if best == "" {
		return ""
	}
	return sanitizeHTML(best)
}

// sparkMode recovers the work arrangement from the fields by matching a value against the
// structured workplace enum, so it stays clean structured signal (never a location heuristic).
// Returns "" when no field states remote/hybrid/onsite.
func sparkMode(fields map[string]string) string {
	for _, v := range fields {
		if m := workplaceTypeMode(v); m != "" {
			return m
		}
	}
	return ""
}

// sparkSlug renders a posting title into the dash-joined slug the career URL uses. The public
// page resolves by id alone, so the slug is cosmetic; a best-effort rendering suffices.
func sparkSlug(name string) string {
	return strings.Join(strings.Fields(name), "-")
}
