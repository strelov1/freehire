package contribution

import (
	"net/url"
	"strings"

	"github.com/MoeForge/moego/regexp"
)

// This file holds the link parsing specific to the contribution service: pulling a job id
// out of a link that embeds an ATS on a company's own domain, so the board can be looked
// up in the catalogue by that id. Recognising which board a URL belongs to is not service
// logic and lives in internal/ingest/atsboard, shared with link resolution and boardresolve.

// ghNumericID matches a Greenhouse-style numeric job id.
var ghNumericID = regexp.MustCompile(`^[0-9]{7,12}$`)

// greenhouseJobID extracts a Greenhouse job id from a link that carries one but no board token:
// the gh_jid query param (Greenhouse's embed param, e.g. company.com/careers?gh_jid=123), or an
// all-numeric path segment (company.com/careers/…/<id>/…). ok=false when neither is present.
//
// The path is read from the RIGHT, and not only at the tail: a storefront over Greenhouse often
// appends a human-readable slug after the id (dropbox.jobs/en/jobs/<id>/<title>/), and matching
// the last segment alone missed those. Scanning the whole path costs nothing in precision —
// the id is only ever believed if the catalogue holds a Greenhouse posting under it, so a
// numeric segment that is not a job id simply finds nothing.
func greenhouseJobID(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if id := u.Query().Get("gh_jid"); ghNumericID.MatchString(id) {
		return id, true
	}
	segs := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := len(segs) - 1; i >= 0; i-- {
		if ghNumericID.MatchString(segs[i]) {
			return segs[i], true
		}
	}
	return "", false
}

// ashbyUUID matches an Ashby job id (a UUID) — the value Ashby's embed widget carries.
var ashbyUUID = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`)

// ashbyJobID extracts an Ashby job id from a link that embeds an Ashby board on a company's own
// domain (company.com/careers?ashby_jid=<uuid>): the ashby_jid query param the embed widget
// carries. The board slug never appears in the URL or the (JS-rendered) markup, so the recognizer
// and page-fetch resolver both miss it — but external_id is "<board>:<uuid>", so the id resolves
// the board. ok=false when absent. A non-Ashby UUID won't be found downstream, so a false
// positive is harmless.
func ashbyJobID(rawURL string) (string, bool) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", false
	}
	if id := u.Query().Get("ashby_jid"); ashbyUUID.MatchString(id) {
		return id, true
	}
	return "", false
}

// stripQueryFragment returns rawURL without its query or fragment, and without a trailing
// path slash; the raw string on parse error.
//
// The trailing-slash trim matters because this is the canonicalization RecordReview keys
// its dedup on (migrations/0037_link_contributions_review.sql's partial unique index on
// url WHERE source IS NULL): two people pasting the same unrecognized page that differ
// only by a trailing slash (".../title" vs ".../title/") must land as the SAME review-queue
// row, not two. It stops short of internal/ingest/atsboard's fuller canonicalization (which also
// drops a trailing "/apply" segment) — that extra trim is meaningful for a recognized ATS
// board and would be over-reaching for an arbitrary unrecognized URL here.
func stripQueryFragment(rawURL string) string {
	u, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	u.RawQuery, u.Fragment = "", ""
	u.Path = strings.TrimSuffix(u.Path, "/")
	return u.String()
}
