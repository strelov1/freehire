// Package recentfeed drains recent_feed_outbox and turns it into the homepage's
// live "recently added jobs" feed: grouping bursts of same-role postings into one
// aggregated entry, and broadcasting the result to connected SSE clients. See
// openspec/changes/add-homepage-recent-jobs-feed for the product and design context.
package recentfeed

// Kind distinguishes a feed Entry that represents one posting from one that
// represents an aggregated burst of postings for the same role.
type Kind string

const (
	// KindSingle is one eligible posting.
	KindSingle Kind = "single"
	// KindAggregate is a burst of postings for the same role, collapsed into one
	// entry once their count reaches AggregationThreshold. It names a
	// representative posting's title and company but must never be presented as
	// if all counted postings came from that one company — see design.md,
	// "Aggregated entries do not attribute a single company".
	KindAggregate Kind = "aggregate"
)

// Entry is one item in the live feed, ready to serialize as an SSE event.
type Entry struct {
	Kind Kind `json:"kind"`
	// Title and CompanyName are always populated: on a KindAggregate entry they
	// come from one representative posting in the group, not a synthesized label.
	Title       string `json:"title"`
	CompanyName string `json:"company_name"`
	// JobSlug links a KindSingle entry to its posting. Empty on KindAggregate,
	// which links to the catalogue instead (there is no single posting to link to).
	JobSlug string `json:"job_slug,omitempty"`
	// Count is the number of postings a KindAggregate entry represents. Unused
	// (zero) on KindSingle, which is always exactly one posting.
	Count int `json:"count,omitempty"`
}
