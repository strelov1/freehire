package telegram

import (
	"context"
	"fmt"
)

// StoredPost is a telegram_posts row as the re-filter pass reads it: the two columns that
// decide admission, plus the primary key it resumes from.
type StoredPost struct {
	Channel string
	MsgID   int64
	Text    string
	Links   []Link
}

// RefilterStore is the storage half of the re-filter pass. Both halves are narrow on
// purpose — this pass reads posts the prefilter declined and clears one column.
type RefilterStore interface {
	// ListRejected pages prefilter-declined posts after the given primary key, in
	// (channel, msg_id) order.
	ListRejected(ctx context.Context, afterChannel string, afterMsgID int64, limit int32) ([]StoredPost, error)
	// Requeue hands one post back to the extraction queue, reporting how many rows it
	// changed — zero when the post no longer qualifies, which is not an error.
	Requeue(ctx context.Context, channel string, msgID int64) (int64, error)
}

// RefilterStats summarises one re-filter run.
type RefilterStats struct {
	Scanned  int // posts read
	Admitted int // posts today's rule admits (what a dry run reports)
	Requeued int // posts actually handed back; zero unless Apply

	ByChannel map[string]int // admitted, per channel — which channels the widening reached

	// Stopped reports that the run ended on its own bound rather than at the end of the
	// table, and the two fields below are where to continue. A run states this rather
	// than leaving it to be inferred from a row count, the rule cmd/backfill-derive
	// documents after losing its cursor twice in one evening.
	Stopped     bool
	NextChannel string
	NextMsgID   int64
}

// RefilterRunner re-offers the posts the crawl's prefilter declined, for the case the
// markers have since widened. A declined post is stamped extracted_at at INSERT time and
// nothing revisits it, so a marker added today reaches new posts only — measured
// 2026-09-23, 96 stored posts already matched the marker set and were stuck behind an
// older one, and the Spanish marker landing that day left 6,335 more behind it.
//
// It decides with AdmitsPost, the same function the crawl calls, so the two cannot drift.
type RefilterRunner struct {
	Store RefilterStore
	Links LinkMatcher // optional, as in CrawlRunner; nil means only the text can admit

	// Apply writes. Unset, the run reports what it WOULD requeue and changes nothing —
	// the shape cmd/merge-companies and cmd/close-chronic-boards already use.
	Apply bool

	Batch int32 // rows per page; <= 0 means defaultRefilterBatch
	Max   int64 // stop after scanning this many rows; <= 0 means unbounded

	// AfterChannel/AfterMsgID resume a bounded run at the key the previous one printed.
	// Without them Max is a trap rather than a bound: a REFUSED post never leaves the
	// predicate, so a second run starting from the top rescans exactly what the first one
	// rejected, and a dry run — where nothing leaves the predicate at all — repeats its
	// report forever. That is the failure cmd/backfill-derive carried until freehire#2864.
	//
	// They are EXCLUSIVE — the first row done is the one after this key — and named After
	// rather than From for exactly that reason: the sibling passes' BACKFILL_*_FROM_ID is
	// inclusive, and two knobs sharing a name and a shape while disagreeing about whether
	// the key is done or to-do lose one row per hop.
	AfterChannel string
	AfterMsgID   int64
}

const defaultRefilterBatch = 500

// Run walks the declined posts once. A failed read ends the run — unlike a per-post
// write, which cannot fail in a way worth stopping for: Requeue reporting zero rows means
// the post stopped qualifying, and that is the guard doing its job.
func (r RefilterRunner) Run(ctx context.Context) (RefilterStats, error) {
	batch := r.Batch
	if batch <= 0 {
		batch = defaultRefilterBatch
	}

	stats := RefilterStats{ByChannel: map[string]int{}}
	afterChannel, afterMsgID := r.AfterChannel, r.AfterMsgID

	for {
		limit := batch
		if r.Max > 0 {
			remaining := r.Max - int64(stats.Scanned)
			if remaining <= 0 {
				// The bound is spent, but spent is not the same as stopped short: a Max
				// that happens to land on the last row would otherwise report a resume
				// cursor and cost the operator a follow-up run to be told there was
				// nothing. One indexed row settles it, and only on a bounded run.
				more, err := r.Store.ListRejected(ctx, afterChannel, afterMsgID, 1)
				if err != nil {
					return stats, fmt.Errorf("telegram refilter: probe after %s/%d: %w", afterChannel, afterMsgID, err)
				}
				if len(more) > 0 {
					stats.Stopped = true
					stats.NextChannel, stats.NextMsgID = afterChannel, afterMsgID
				}
				return stats, nil
			}
			if remaining < int64(limit) {
				limit = int32(remaining)
			}
		}

		posts, err := r.Store.ListRejected(ctx, afterChannel, afterMsgID, limit)
		if err != nil {
			return stats, fmt.Errorf("telegram refilter: list after %s/%d: %w", afterChannel, afterMsgID, err)
		}
		if len(posts) == 0 {
			return stats, nil
		}

		for _, p := range posts {
			stats.Scanned++
			// The cursor advances per POST, not per page, so the resume point is the last
			// row this run actually finished rather than the last one its reader read.
			afterChannel, afterMsgID = p.Channel, p.MsgID

			if !AdmitsPost(p.Text, p.Links, r.Links) {
				continue
			}
			stats.Admitted++
			stats.ByChannel[p.Channel]++

			if !r.Apply {
				continue
			}
			rows, err := r.Store.Requeue(ctx, p.Channel, p.MsgID)
			if err != nil {
				return stats, fmt.Errorf("telegram refilter: requeue %s/%d: %w", p.Channel, p.MsgID, err)
			}
			if rows > 0 {
				stats.Requeued++
			}
		}

		if int32(len(posts)) < limit {
			return stats, nil // the page was short: that was the end of the table
		}
	}
}
