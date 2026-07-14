package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/strelov1/freehire/internal/sources"
)

// bairesdev resolves BairesDev vacancies reached from an apply link
// (applicants.bairesdev.com/job/<careerId>/<jobId>/apply). BairesDev publishes no crawlable
// board — its openings listing is login-gated — so it is a link-only source: the sole public
// surface is a per-job endpoint keyed by the numeric job id. The job id is the stable
// canonical key (careerId is just the career-site the link came through and is ignored for
// dedup), and the posting's schema.org JSON-LD comes straight from that endpoint.
type bairesdev struct {
	http Client
}

// NewBairesDev builds the BairesDev link-source adapter.
func NewBairesDev(c Client) Source { return bairesdev{http: c} }

func (bairesdev) Source() string { return "bairesdev" }

// bairesDevJobPath captures the career-site id and the job id from an apply link path
// (/job/<careerId>/<jobId> with an optional trailing /apply).
var bairesDevJobPath = regexp.MustCompile(`^/job/(\d+)/(\d+)(?:/apply)?/?$`)

// Match handles applicants.bairesdev.com/job/<careerId>/<jobId>[/apply] links only.
func (bairesdev) Match(u *url.URL) bool {
	return host(u) == "applicants.bairesdev.com" && bairesDevJobPath.MatchString(u.Path)
}

// bairesDevPosting selects the JobPosting ld+json fields the public endpoint returns.
type bairesDevPosting struct {
	Title              string `json:"title"`
	Description        string `json:"description"`
	DatePosted         string `json:"datePosted"`
	JobLocationType    string `json:"jobLocationType"`
	HiringOrganization struct {
		Name string `json:"name"`
	} `json:"hiringOrganization"`
}

// Resolve fetches the posting from the public JobPosting endpoint keyed by the job id parsed
// from the link. The id is numeric and the API URL is built here (the raw link is never
// followed), so a hijacked link cannot redirect the fetch to another host.
func (b bairesdev) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := bairesDevJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	careerID, jobID := m[1], m[2]

	var p bairesDevPosting
	api := "https://applicants.bairesdev.com/api/JobPosting?JobPostingId=" + jobID
	if err := b.http.GetJSON(ctx, api, &p); err != nil {
		return sources.Job{}, false, err
	}
	if p.Title == "" {
		return sources.Job{}, false, nil // no live posting under this id — skip
	}

	// JobPosting carries only a teaser; the full HTML description lives on the apply-flow endpoint.
	// Fall back to the teaser when it is unavailable so a hiccup degrades the text, not the job.
	desc := b.fullDescription(ctx, jobID)
	if desc == "" {
		desc = p.Description
	}
	// datePosted has no timezone (e.g. "2025-06-02T10:23:21.503"), so RFC3339 rejects it;
	// fall back to the date alone, keeping the approximate posted_at.
	posted := sources.ParseRFC3339(p.DatePosted)
	if posted == nil && len(p.DatePosted) >= 10 {
		posted = sources.ParseDate(p.DatePosted[:10])
	}
	company := p.HiringOrganization.Name
	if company == "" {
		company = "BairesDev"
	}
	return sources.Job{
		ExternalID:  jobID,
		URL:         fmt.Sprintf("https://applicants.bairesdev.com/job/%s/%s/apply", careerID, jobID),
		Title:       p.Title,
		Company:     company,
		Description: sources.SanitizeHTML(desc),
		Remote:      isTelecommute(p.JobLocationType),
		PostedAt:    posted,
	}, true, nil
}

// fullDescription reads the full HTML description from the apply-flow endpoint
// (jobResults[0].description), or "" when the endpoint fails or carries no result.
func (b bairesdev) fullDescription(ctx context.Context, jobID string) string {
	var resp struct {
		JobResults []struct {
			Description string `json:"description"`
		} `json:"jobResults"`
	}
	api := "https://applicants.bairesdev.com/api/Job?JobOfferId=" + jobID
	if err := b.http.GetJSON(ctx, api, &resp); err != nil {
		return ""
	}
	if len(resp.JobResults) == 0 {
		return ""
	}
	return resp.JobResults[0].Description
}
