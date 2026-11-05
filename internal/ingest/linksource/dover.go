package linksource

import (
	"context"
	"fmt"
	"net/url"
	"regexp"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"github.com/strelov1/freehire/internal/platform/externalid"
)

// dover resolves Dover-hosted vacancies. Like Ashby it is multi-tenant, so it writes the SAME
// identity the ingest pipeline does (source="dover", external_id="<board>:<id>") to dedup
// against an already-crawled board and add an unlisted one under the canonical key.
//
// Dover has a cheap per-job detail endpoint (the same one the board adapter hydrates with),
// so — unlike Ashby — this adapter reads the posting directly and takes the employer's real
// name off the response's client record, never a humanized link segment. The link's uuid IS
// the posting id: on the submitted Deepspatial link the client was 1aeeba55-… while the uuid
// was a5546acf-… (live-verified 2026-11-05), so trusting that uuid as anything but the
// posting id was exactly the defect review caught in the recognizer's first design.
type dover struct {
	http Client
}

// NewDover builds the Dover link-source adapter.
func NewDover(c Client) Source { return dover{http: c} }

func (dover) Source() string { return "dover" }

// doverJobPath captures the posting uuid from a jobs.dover.com link path
// (/<Org>/<uuid>[/slug]). The Org slug names the board only via the contribution flow's own
// recognition; this resolver files the posting under the client record the detail response
// carries (see Resolve), so employer naming cannot drift from the link's spelling.
var doverJobPath = regexp.MustCompile(`^/[^/]+/([0-9a-fA-F-]{36})(?:/|$)`)

// Match handles jobs.dover.com/<Org>/<uuid> links only — the same shape the atsboard
// recognizer accepts (its UUID gate is what keeps the marketing shell and the vendor's own
// pages out), and nothing else on the host.
func (dover) Match(u *url.URL) bool {
	return host(u) == "jobs.dover.com" && doverJobPath.MatchString(u.Path)
}

// Resolve reads the posting's public detail endpoint and maps it, mirroring the ingest dover
// adapter's detail mapping (compensation, locations, employment type) with the company taken
// from the posting's own client_name. The external id is namespaced by the company's Dover
// slug — the board the contribution flow and the board adapter file this company under —
// keeping link-imported and board-crawled rows of one posting deduped.
func (d dover) Resolve(ctx context.Context, raw string) (sources.Job, bool, error) {
	u, err := url.Parse(raw)
	if err != nil {
		return sources.Job{}, false, err
	}
	m := doverJobPath.FindStringSubmatch(u.Path)
	if m == nil {
		return sources.Job{}, false, nil
	}
	id := m[1]

	var resp sources.DoverJobDetail
	api := fmt.Sprintf("https://app.dover.com/api/v1/inbound/application-portal-job/%s", id)
	if err := d.http.GetJSON(ctx, api, &resp); err != nil {
		return sources.Job{}, false, err
	}

	// The canonical board slug the contribution flow files this company under comes from the
	// slug lookup (the detail response names the company but not its slug); both spellings of
	// the name answer with the same lower-case slug (verified live 2026-11-05). A lookup that
	// fails leaves the slug empty and fails the resolve — an unresolvable posting is not a
	// vacancy to store.
	var slugResp struct {
		Slug string `json:"slug"`
	}
	if err := d.http.GetJSON(ctx, "https://app.dover.com/api/v1/careers-page-slug/"+url.PathEscape(resp.ClientName), &slugResp); err != nil {
		return sources.Job{}, false, err
	}
	resp.ClientSlug = slugResp.Slug

	job := resp.ToJob()
	job.ExternalID = externalid.Namespace(resp.ClientSlug, id)
	job.URL = raw
	return job, true, nil
}
