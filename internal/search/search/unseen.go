package search

import (
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// UnseenWindow is how long a crawled posting may go unseen by its source before search
// stops showing it. A safety net under the ingest sweep, which closes a posting only when a
// run PROVED it covered the board, so a board whose crawl keeps failing never sweeps. Many
// crawl cycles for every provider, the 72h quiet-board pause included.
const UnseenWindow = 14 * 24 * time.Hour

// UnseenWindows maps a source to how long one of its postings may go unseen before search
// withholds it; an absent source is never withheld on this ground. The caller builds it from
// internal/ingest/sources.UnseenWindows (this package may not import ingest), leaving out
// sources whose stale last_seen_at is not evidence of removal.
type UnseenWindows map[string]time.Duration

// Unseen reports whether j's source has not shown it for longer than its window. A row
// never stamped is not withheld.
func (w UnseenWindows) Unseen(j db.Job, now time.Time) bool {
	window, ok := w[j.Source]
	return ok && j.LastSeenAt.Valid && now.Sub(j.LastSeenAt.Time) > window
}

// Withheld reports whether search keeps an otherwise-listable job (open, canonical, public)
// out of its index: no category (CategoryUnresolved), no body (DescriptionMissing), or an
// unseen source (UnseenWindows.Unseen). Every index writer (cmd/reindex, cmd/search-drain,
// linkimport) asks this one predicate.
//
// The unseen rule applies when a document is written, not as a query-time filter on an
// indexed last_seen_at: the liveness refresh (RefreshUnchangedJob) pushes nothing to
// search_outbox, so that field would freeze and hide live postings. Instead cmd/reindex's
// time-crossings pass queues a posting whose window runs out, and the liveness write
// (RefreshUnchangedJob/TouchJob) queues one its source lists again. The job page reads
// Postgres, so a shared link still opens.
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

// WithheldBy is Withheld with its reason: the first rule that holds, so counts partition.
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
