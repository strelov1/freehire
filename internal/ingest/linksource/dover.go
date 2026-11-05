package linksource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"

	"github.com/strelov1/freehire/internal/dict/normalize"
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
// (/<Org>/<uuid>[/slug]). The uuid gate is the exact canonical 8-4-4-4-12 hex spelling — the
// same shape the atsboard recognizer requires of the second segment — and the slug gate is
// its mirror. The Org slug names the board only via the contribution flow's own recognition;
// this resolver files the posting under the client record the detail response carries (see
// Resolve), so employer naming cannot drift from the link's spelling.
var doverJobPath = regexp.MustCompile(`^/[^/]+/([0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12})(?:/[^/]+)?$`)

// statusCoder is satisfied by sources.StatusError without this package importing its type:
// the same local-interface shape sources itself uses for callers it imports (see
// StatusError.StatusCode's doc comment).
type statusCoder interface{ StatusCode() int }

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
//
// A posting the platform says is GONE (a 404 detail) is a skip — ok=false, not an error, the
// same "not on the board anymore" answer the ashby resolver yields; every other failure
// (transport, 5xx, an undecodable body) is the ABSENCE of an answer and propagates so the
// caller can retry. A detail answering inactive or private is likewise a skip: the posting is
// not a live vacancy to import, exactly as the board adapter's detail stage drops it.
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
		var se statusCoder
		if errors.As(err, &se) && se.StatusCode() == 404 {
			return sources.Job{}, false, nil // the platform's own answer: posting gone
		}
		return sources.Job{}, false, err
	}
	if !resp.Active || resp.IsPrivate {
		return sources.Job{}, false, nil // not a live vacancy — skip, as the board crawl drops it
	}

	// The canonical board slug the contribution flow files this company under comes from the
	// slug lookup (the detail response names the company but not its slug); both spellings of
	// the name answer with the same lower-case slug (verified live 2026-11-05). An empty slug
	// or client name fails the resolve rather than storing a ":<id>" external id that dedups
	// against nothing — the absence of an answer is not an answer.
	var slugResp struct {
		Slug string `json:"slug"`
	}
	if resp.ClientName == "" {
		return sources.Job{}, false, nil // a posting with no client identity is not storable
	}
	if err := d.http.GetJSON(ctx, "https://app.dover.com/api/v1/careers-page-slug/"+url.PathEscape(resp.ClientName), &slugResp); err != nil {
		return sources.Job{}, false, err
	}
	if slugResp.Slug == "" {
		return sources.Job{}, false, nil // unresolvable company — not a vacancy to store
	}

	job := resp.ToJob()
	// Namespaced by the slug the CRAWL path namespaces by: the contribution flow stores the
	// board as the link spelled it (verbatim capture), the adapter crawls that spelling, and
	// the pipeline derives each posting's dedup key from the board string — so the identity
	// here must be normalize.CompanySlug of the same name the board row is filed under.
	// normalize.CompanySlug lower-cases and drops legal-form tails, which for Dover clients
	// ("Deepspatial") lands on the platform's own lower-case slug — the spelling this
	// lookup returns, asserted against it below so a future divergence fails loudly here
	// instead of silently forking the dedup key.
	namespaced := normalize.CompanySlug(resp.ClientName)
	if namespaced != slugResp.Slug {
		return sources.Job{}, false, fmt.Errorf("dover: client %q slug %q disagrees with its own name", resp.ClientName, slugResp.Slug)
	}
	job.ExternalID = externalid.Namespace(namespaced, id)
	job.URL = raw
	return job, true, nil
}
