package sources

import (
	"net/url"
	"regexp"
	"strings"
)

// PostingRef is a catalog identity — the (source, external_id) key jobs are
// deduplicated by — recovered from the URL of a job posting in the wild.
//
// It exists so a client that is *looking at* a posting can ask which catalog job
// it is, exactly, instead of describing it and hoping: matching a page title
// against catalog titles is both unreliable (an ATS page title is full of noise)
// and expensive (no index can serve it).
type PostingRef struct {
	Source     string
	ExternalID string
}

// greenhouseJobPath matches the job-detail form, /<board>/jobs/<id>.
var greenhouseJobPath = regexp.MustCompile(`^/([^/]+)/jobs/(\d+)/?$`)

// RefFromURL recovers the catalog identity from a posting URL, reporting false
// when the URL is not one we can resolve — an unknown ATS, or a link that
// carries the job id but not the board it belongs to (a company's own careers
// page, say). A false answer is the honest one: the caller should then leave the
// posting unrecognised rather than guess.
//
// Only Greenhouse is understood today. Each ATS numbers and addresses its
// postings differently, and the board segment of the URL has to be the same
// token the ingest board file uses, so support is added per provider against
// real links rather than inferred from a pattern.
func RefFromURL(raw string) (PostingRef, bool) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return PostingRef{}, false
	}

	switch strings.ToLower(u.Hostname()) {
	case "job-boards.greenhouse.io", "boards.greenhouse.io":
		return greenhouseRef(u)
	default:
		return PostingRef{}, false
	}
}

// greenhouseRef reads a Greenhouse link in either of its two shapes: the
// embedded application form (/embed/job_app?for=<board>&token=<id>) and the job
// detail page (/<board>/jobs/<id>). The form's `jr_id` is Greenhouse's internal
// requisition id, which the catalog does not store — the job id is `token`.
func greenhouseRef(u *url.URL) (PostingRef, bool) {
	if strings.HasPrefix(u.Path, "/embed/job_app") {
		board := u.Query().Get("for")
		id := u.Query().Get("token")
		return greenhousePosting(board, id)
	}
	if m := greenhouseJobPath.FindStringSubmatch(u.Path); m != nil {
		return greenhousePosting(m[1], m[2])
	}
	return PostingRef{}, false
}

func greenhousePosting(board, jobID string) (PostingRef, bool) {
	board = strings.ToLower(strings.TrimSpace(board))
	jobID = strings.TrimSpace(jobID)
	if board == "" || jobID == "" || !isDigits(jobID) {
		return PostingRef{}, false
	}
	return PostingRef{Source: "greenhouse", ExternalID: NamespaceExternalID(board, jobID)}, true
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// applyFormSuffix is the path an ATS appends to a posting for its application
// form, per host. The form is the same posting — a candidate reaches it from the
// detail page and spends most of their time there, so it is the page a browser
// extension asks about most.
//
// Keyed by host and kept to providers whose shape has been checked against the
// catalog, because the assumption does not generalise: on plenty of career sites a
// path segment is a different vacancy, and collapsing it would answer with the
// wrong one.
var applyFormSuffix = map[string]string{
	"jobs.ashbyhq.com":   "/application",
	"jobs.lever.co":      "/apply",
	"jobs.eu.lever.co":   "/apply",
	"apply.workable.com": "/apply",
	"jobs.workable.com":  "/apply",
}

// applyFormSuffixApex is applyFormSuffix's counterpart for a multi-tenant ATS whose
// tenants each get their own host, so no single hostname names it — CatsOne serves
// <tenant>.catsone.com per company, the same shape internal/atsboard's "catsone"
// modeHost entry already recognises. Matched by domain label rather than a tenant
// list, so a new CatsOne tenant needs no entry here either.
var applyFormSuffixApex = map[string]string{
	"catsone": "/apply",
}

// CanonicalPostingURL rewrites a posting's application-form URL to the posting
// itself, and returns everything else unchanged.
//
// It exists because the catalog stores one URL per job — the detail page — and a
// lookup by URL is a string match against it. Without this, standing on the form
// (`…/<id>/application`) reports a posting freehire does not have, when it is the
// very posting it is showing on the page behind.
//
// Deliberately NOT part of `normalize_job_url` in the database: that function backs
// an expression index, so changing it means rebuilding the index on a live table,
// and it is applied to both sides of the comparison — including the stored URLs,
// which never carry an apply suffix. This is a property of the query, not of the
// data.
func CanonicalPostingURL(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return raw
	}
	host := strings.ToLower(u.Hostname())
	suffix, known := applyFormSuffix[host]
	if !known {
		suffix, known = applyFormSuffixByApex(host)
	}
	if !known {
		return raw
	}
	trimmed := strings.TrimSuffix(u.Path, "/")
	if !strings.HasSuffix(strings.ToLower(trimmed), suffix) {
		return raw
	}
	// Cut by length rather than by value: the match above ignored case, so cutting
	// the literal would leave a shouting suffix in place and hand back a URL that
	// was altered (its trailing slash gone) but not canonicalised.
	//
	// A bare "/apply" is not a posting with a form; it is a page named apply.
	stripped := trimmed[:len(trimmed)-len(suffix)]
	if stripped == "" {
		return raw
	}
	u.Path = stripped
	return u.String()
}

// applyFormSuffixByApex matches host against applyFormSuffixApex's domain labels,
// the same "apex sits somewhere in the middle" test internal/atsboard's matchHost
// uses for its own modeHost entries: a HasSuffix(host, ".catsone") check would never
// match "<tenant>.catsone.com" at all — the label is followed by a TLD, not the end
// of the string — so this looks for the apex bracketed by dots instead, exactly like
// atsboard's own "catsone" entry does when it recognises the same hosts for ingest.
func applyFormSuffixByApex(host string) (string, bool) {
	for apex, suffix := range applyFormSuffixApex {
		if strings.Contains(host, "."+apex+".") {
			return suffix, true
		}
	}
	return "", false
}
