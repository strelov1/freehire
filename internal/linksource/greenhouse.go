package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/strelov1/freehire/internal/sources"
)

// greenhouse resolves Greenhouse-hosted vacancies. Greenhouse is multi-tenant, so a TG
// link points at an arbitrary company's board — many of which sources/greenhouse.yml does not list.
// The adapter writes the SAME identity the ingest pipeline would (source="greenhouse",
// external_id="<board>:<id>"), so UpsertJob's ON CONFLICT dedups against an already-crawled
// company and a not-yet-crawled one is added under the canonical key rather than as a
// thin telegram-source duplicate.
type greenhouse struct {
	http Client
}

// NewGreenhouse builds the Greenhouse link-source adapter.
func NewGreenhouse(c Client) Source { return greenhouse{http: c} }

func (greenhouse) Source() string { return "greenhouse" }

// greenhouseJobPath captures the board and numeric job id from a job link path
// (job-boards.greenhouse.io/<board>/jobs/<id>, and the boards.* / EU variants).
var greenhouseJobPath = regexp.MustCompile(`^/([^/]+)/jobs/(\d+)/?$`)

// Match handles any greenhouse.io job link (job-boards/boards, US or EU host).
func (greenhouse) Match(u *url.URL) bool {
	return strings.HasSuffix(host(u), "greenhouse.io") && greenhouseJobPath.MatchString(u.Path)
}

// Resolve reads the public per-job boards API for the linked board+id and maps it exactly
// as the ingest greenhouse adapter does, namespacing the external id by board to match.
func (g greenhouse) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := greenhouseJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	board, id := m[1], m[2]

	// EU-hosted boards are served by the EU API host; everything else by the global one.
	apiHost := "boards-api.greenhouse.io"
	if strings.Contains(host(u), "eu.greenhouse.io") {
		apiHost = "boards-api.eu.greenhouse.io"
	}
	api := fmt.Sprintf("https://%s/v1/boards/%s/jobs/%s?content=true", apiHost, board, id)

	var j sources.GreenhousePosting
	if err := g.http.GetJSON(ctx, api, &j); err != nil {
		return sources.Job{}, false, err
	}
	if j.ID == 0 {
		return sources.Job{}, false, nil // not a live posting (closed/removed) — skip
	}

	// The posting→Job mapping is shared with the greenhouse board adapter so the two
	// produce identical facets; only the identity differs (namespaced id, and the company
	// from the API's company_name — the board list endpoint does not serve it).
	job := sources.MapGreenhousePosting(j)
	job.ExternalID = sources.NamespaceExternalID(board, id)
	job.Company = j.CompanyName
	return job, true, nil
}
