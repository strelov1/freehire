package search

import (
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// UnseenWindow is how long a crawled posting may go unseen by its source before search stops
// showing it. It is a safety net under the ingest sweep, not a close: the sweep closes a
// posting only when a run PROVED it covered the posting's board, so a board whose crawl keeps
// failing or timing out never sweeps, and its removed postings stayed searchable for weeks
// (measured 2026-10-26: 96,479 of 848,549 open technical postings unseen for more than 14
// days). Fourteen days is many crawl cycles for every provider on the hourly-to-daily
// cadence, the quiet-board pause included (72h at most), and seven times the sweep's own 48h.
const UnseenWindow = 14 * 24 * time.Hour

// UnseenWindows maps a source to how long one of its postings may go unseen before search
// withholds it. A source absent from the map is never withheld on this ground. The map is
// built by the caller from internal/ingest/sources.UnseenWindows over the adapter taxonomy —
// this package may not import ingest — which leaves out every source whose stale
// last_seen_at is not evidence of removal (no adapter re-crawls it, it self-closes from a
// change stream, or a liveness probe judges it instead).
type UnseenWindows map[string]time.Duration

// Unseen reports whether j's source has not shown it for longer than that source's window.
// A row never stamped is not evidence either way and is not withheld.
func (w UnseenWindows) Unseen(j db.Job, now time.Time) bool {
	window, ok := w[j.Source]
	return ok && j.LastSeenAt.Valid && now.Sub(j.LastSeenAt.Time) > window
}

// Withheld reports whether search keeps an otherwise-listable job (open, canonical, public)
// out of its index: no category (CategoryUnresolved), no body (DescriptionMissing), or a
// source that has stopped showing it (UnseenWindows.Unseen). It is the one predicate every
// index writer asks — cmd/reindex, cmd/search-drain, linkimport — so the three cannot drift.
//
// The unseen rule is applied here, when a document is written, rather than as a query-time
// filter on an indexed last_seen_at, because the crawl's liveness refresh (RefreshUnchangedJob)
// deliberately pushes nothing to search_outbox: an indexed last_seen_at would freeze at the
// last content write and hide live, unchanged postings by the thousand. The full rebuild
// reads Postgres and re-asks this for every row, so a posting that goes unseen leaves search
// at the next rebuild after its window, and one its source lists again returns at the next
// rebuild — or sooner, on its next content write. The job page reads Postgres and keeps
// serving a withheld posting, so a shared link still opens.
func Withheld(j db.Job, now time.Time, unseen UnseenWindows) bool {
	return WithheldBy(j, now, unseen) != ""
}

// WithholdReason names which of Withheld's rules keeps a job out of search; empty means none.
type WithholdReason string

const (
	WithheldNoCategory    WithholdReason = "category_unresolved"
	WithheldNoDescription WithholdReason = "description_missing"
	WithheldUnseen        WithholdReason = "unseen"
)

// WithheldBy is Withheld with its reason: the first rule that holds, in the same order, so
// counting reasons partitions the withheld jobs rather than counting one twice.
func WithheldBy(j db.Job, now time.Time, unseen UnseenWindows) WithholdReason {
	switch {
	case CategoryUnresolved(j):
		return WithheldNoCategory
	case DescriptionMissing(j):
		return WithheldNoDescription
	case unseen.Unseen(j, now):
		return WithheldUnseen
	}
	return ""
}
