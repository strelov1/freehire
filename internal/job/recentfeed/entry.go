// Package recentfeed drains recent_feed_outbox and turns it into the homepage's
// live "recently added jobs" feed: grouping bursts of same-role or same-company
// postings into one aggregated entry, and broadcasting the result to connected
// SSE clients. See openspec/changes/add-homepage-recent-jobs-feed for the
// product and design context.
package recentfeed

import "time"

// Kind distinguishes a feed Entry that represents one posting from one that
// represents an aggregated burst of postings — either the same role across
// companies (KindAggregate) or the same company across roles
// (KindCompanyAggregate).
type Kind string

const (
	// KindSingle is one eligible posting.
	KindSingle Kind = "single"
	// KindAggregate is a burst of postings for the same role across different
	// companies, collapsed into one entry once their count reaches
	// AggregationThreshold. It names a representative posting's title and
	// company but must never be presented as if all counted postings came from
	// that one company — see design.md, "Aggregated entries do not attribute a
	// single company".
	KindAggregate Kind = "aggregate"
	// KindCompanyAggregate is the other direction of burst: one company posting
	// many DIFFERENT roles at once (a mass hiring push), collapsed into one
	// entry once their count reaches AggregationThreshold. A posting already
	// counted toward a KindAggregate entry is never also counted here — see
	// Group's two-pass ordering.
	KindCompanyAggregate Kind = "company_aggregate"
)

// Entry is one item in the live feed, ready to serialize as an SSE event.
type Entry struct {
	Kind Kind `json:"kind"`
	// Title and CompanyName are always populated. On a KindAggregate entry they
	// come from one representative posting in the group, not a synthesized
	// label. On a KindCompanyAggregate entry CompanyName is exact (the one
	// company the whole entry is about) and Title is one representative role
	// from the burst.
	Title       string `json:"title"`
	CompanyName string `json:"company_name"`
	// JobSlug links a KindSingle entry to its posting. Empty on either
	// aggregate kind, which links to the catalogue instead (there is no single
	// posting to link to).
	JobSlug string `json:"job_slug,omitempty"`
	// Count is the number of postings an aggregate entry (either kind)
	// represents. Unused (zero) on KindSingle, which is always exactly one
	// posting.
	Count int `json:"count,omitempty"`
	// ProducedAt is when the Poller emitted this entry (not when the
	// underlying posting was ingested) — stamped once, so it stays fixed as
	// the entry sits in the Broadcaster's backlog and a client renders a
	// live-ticking "N seconds ago" against it.
	ProducedAt time.Time `json:"produced_at"`
}
