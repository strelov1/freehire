package linksource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"

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

// doverOrgPath is the same shape doverJobPath matches; doverOrg re-captures the Org segment
// from an already-matched path for the existence lookup.
var doverOrgPath = regexp.MustCompile(`^/([^/]+)/[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}(?:/[^/]+)?$`)

// doverOrg extracts the Org segment of a matched jobs.dover.com path.
func doverOrg(path string) string {
	if m := doverOrgPath.FindStringSubmatch(path); len(m) == 2 {
		return m[1]
	}
	return ""
}

// statusCoder is satisfied by sources.StatusError without this package importing its type:
// the same local-interface shape sources itself uses for callers it imports (see
// StatusError.StatusCode's doc comment).
type statusCoder interface{ StatusCode() int }

// Match handles jobs.dover.com/<Org>/<uuid> links only — the same shape the atsboard
// recognizer accepts (its UUID gate is what keeps the marketing shell and the vendor's own
// pages out), and nothing else on the host. The host check is the recognizer's exact-host rule
// too: u.Hostname() rather than the www.-trimming host helper, since Dover serves no boards on
// www.jobs.dover.com, compared case-insensitively because hostnames are.
func (dover) Match(u *url.URL) bool {
	return strings.EqualFold(u.Hostname(), "jobs.dover.com") && doverJobPath.MatchString(u.Path)
}

// Resolve reads the posting's public detail endpoint and maps it, mirroring the ingest dover
// adapter's detail mapping (compensation, locations, employment type) with the company taken
// from the posting's own client_name. The external id is the API's own posting id namespaced
// by the link's Org spelling — the board the contribution flow and the crawl file this posting
// under — keeping link-imported and board-crawled rows of one posting deduped.
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

	// The slug lookup is the OWNERSHIP check: the jobs.dover.com shell answers any slug and
	// any pair, so nothing on the host proves the Org segment names the company that owns this
	// posting — the careers API does, by resolving Org to the same client the detail names. A
	// link pairing one company's Org with another's posting would otherwise file that posting
	// under a board whose crawl never writes it. The link's Org spelling is also the spelling
	// the contribution flow files the board under and the crawl adapter namespaces postings by
	// (verbatim capture, pipeline jobIdentity = Namespace(board, id)), so the resolver
	// namespaces by that SAME spelling — resolving through the client's own slug instead would
	// fork the dedup key and file one posting twice. A posting with no id, client id or client
	// name, an unresolvable slug, or a slug owned by another client is a skip: a posting with
	// no verifiable company is not a vacancy to store.
	if resp.ID == "" || resp.ClientID == "" || resp.ClientName == "" {
		return sources.Job{}, false, nil // a posting with no identity is not storable
	}
	org := doverOrg(u.Path)
	var slugResp struct {
		ID   string `json:"id"`
		Slug string `json:"slug"`
	}
	if err := d.http.GetJSON(ctx, "https://app.dover.com/api/v1/careers-page-slug/"+url.PathEscape(org), &slugResp); err != nil {
		var se statusCoder
		if errors.As(err, &se) && se.StatusCode() == 404 {
			return sources.Job{}, false, nil // the shell's slug names no company — not a retryable failure
		}
		return sources.Job{}, false, err
	}
	if slugResp.Slug == "" {
		return sources.Job{}, false, nil // unresolvable company — not a vacancy to store
	}
	if slugResp.ID != resp.ClientID {
		return sources.Job{}, false, nil // Org names another company — not this posting's board
	}

	job := resp.ToJob()
	// resp.ID, not the link's spelling: the API accepts any case of the uuid and answers with
	// the lower-case id the crawl stores, so an upper-case link cannot fork the dedup key.
	job.ExternalID = externalid.Namespace(org, resp.ID)
	job.URL = raw
	return job, true, nil
}
