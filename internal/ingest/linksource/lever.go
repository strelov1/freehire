package linksource

import (
	"context"
	"fmt"
	"net/url"

	"github.com/MoeForge/moego/regexp"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/ingest/sources/ats"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// lever resolves Lever-hosted vacancies. Lever is multi-tenant, so a TG link points at an
// arbitrary company's board — many of which sources/lever.yml does not list. The adapter
// writes the SAME identity the ingest pipeline would (source="lever", external_id
// "<board>:<posting id>" via externalid.Namespace, the URL's company slug being the
// board), so UpsertJob's ON CONFLICT dedups against an already-crawled company and a
// not-yet-crawled one is added under the canonical key rather than a thin telegram dup.
type lever struct {
	http Client
}

// NewLever builds the Lever link-source adapter.
func NewLever(c Client) Source { return lever{http: c} }

func (lever) Source() string { return "lever" }

// leverJobPath captures the company and posting UUID from a job link
// (jobs.lever.co/<company>/<uuid>, optionally trailing /apply).
var leverJobPath = regexp.MustCompile(`^/([^/]+)/([0-9a-fA-F-]{36})(?:/apply)?/?$`)

// Match handles jobs.lever.co posting links only — a board listing (no posting id) is left
// for the ingest adapter.
func (lever) Match(u *url.URL) bool {
	return host(u) == "jobs.lever.co" && leverJobPath.MatchString(u.Path)
}

// Resolve reads the public per-posting API for the linked company+id and maps it exactly as
// the ingest lever adapter does, namespacing external_id by board (the URL's company slug) to
// match the pipeline's dedup key.
func (l lever) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := leverJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	company, id := m[1], m[2]

	api := fmt.Sprintf("https://api.lever.co/v0/postings/%s/%s?mode=json", company, id)
	var p ats.LeverPosting
	if err := l.http.GetJSON(ctx, api, &p); err != nil {
		return sources.Job{}, false, err
	}
	if p.ID == "" {
		return sources.Job{}, false, nil // not a live posting (closed/removed) — skip
	}

	// The posting→Job mapping is shared with the lever board adapter so the two produce
	// identical facets; only the identity differs (namespaced id, and the company humanized
	// from the board slug — the per-posting API carries no name).
	job := ats.MapLeverPosting(p)
	job.ExternalID = externalid.Namespace(company, p.ID)
	job.Company = sources.HumanizeBoard(company)
	return job, true, nil
}
