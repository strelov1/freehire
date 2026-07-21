package viewlog

import "strings"

// Kind is the kind of counted view a log line represents.
type Kind int

const (
	// KindPage is an SSR job detail page open (GET /jobs/<slug>). Bot-filtered.
	KindPage Kind = iota + 1
	// KindAPI is an external API read (GET /api/v1/jobs/<slug>). Not bot-filtered.
	KindAPI
)

// Signal is a counted view: which job (by slug) and how it was reached.
type Signal struct {
	Slug string
	Kind Kind
}

// Classify maps a record to a counted view signal, or ok=false to ignore it.
// Only a 2xx GET of exactly /jobs/<slug> or /api/v1/jobs/<slug> counts — a slug
// is a single path segment, so job list pages and sub-resources (similar, fit,
// copies, og.png) are ignored.
func Classify(rec Record) (Signal, bool) {
	if rec.Method != "GET" || rec.Status < 200 || rec.Status >= 300 {
		return Signal{}, false
	}
	path := rec.Path
	if i := strings.IndexByte(path, '?'); i >= 0 {
		path = path[:i]
	}
	// A SvelteKit client-side (SPA) navigation to the detail page fetches the load
	// data at /jobs/<slug>/__data.json instead of re-requesting the HTML; count it
	// as the same page view. A full/direct load hits /jobs/<slug> — the two are
	// mutually exclusive per view, and the (visitor, slug, day) dedup collapses any
	// overlap, so this never double-counts.
	path = strings.TrimSuffix(path, "/__data.json")
	if slug, ok := singleSegment(path, "/jobs/"); ok {
		return Signal{Slug: slug, Kind: KindPage}, true
	}
	if slug, ok := singleSegment(path, "/api/v1/jobs/"); ok {
		return Signal{Slug: slug, Kind: KindAPI}, true
	}
	return Signal{}, false
}

// singleSegment returns the remainder of path after prefix when it is exactly one
// non-empty segment (no further slashes), e.g. "/jobs/abc" -> "abc", but
// "/jobs/abc/similar" and "/jobs/" do not qualify.
func singleSegment(path, prefix string) (string, bool) {
	rest, ok := strings.CutPrefix(path, prefix)
	if !ok || rest == "" || strings.Contains(rest, "/") {
		return "", false
	}
	return rest, true
}
