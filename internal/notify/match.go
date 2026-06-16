package notify

import (
	"context"
	"log"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// match re-runs each distinct saved-search query against the index and records
// the matching jobs for every subscription on that query. Subscriptions are
// grouped by their (already-canonical, SPA-serialized) query string, so a query
// shared by many subscribers costs one search. A job is gated against each
// subscription's start_at so a subscription never receives jobs older than it.
//
// One failing query is logged and skipped, not fatal — the same isolation as the
// per-board ingest crawl.
func (r *Runner) match(ctx context.Context, stats *Stats) error {
	subs, err := r.store.ListActiveSubscriptions(ctx)
	if err != nil {
		return err
	}

	groups := make(map[string][]db.ListActiveSubscriptionsRow)
	for _, s := range subs {
		groups[s.Query] = append(groups[s.Query], s)
	}

	stats.Queries = len(groups)
	for query, gsubs := range groups {
		if err := r.matchQuery(ctx, query, gsubs, stats); err != nil {
			log.Printf("notify: match query %q failed: %v", query, err)
			continue
		}
	}
	return nil
}

// matchQuery runs one query and records its matches across the subscriptions that
// share it.
func (r *Runner) matchQuery(ctx context.Context, query string, subs []db.ListActiveSubscriptionsRow, stats *Stats) error {
	vals, _ := url.ParseQuery(query)
	res, err := r.searcher.Search(ctx, search.SearchParams{
		Query:  vals.Get("q"),
		Filter: search.FilterFromValues(vals),
		// Freshest first so the bounded window holds the newest matches; the
		// ledger dedups, so re-scanning the same recent jobs each pass is free.
		Sort:  []string{"created_at:desc"},
		Limit: r.cfg.MatchLimit,
		// Pure keyword matching (no semantic blend): a notification must be a
		// precise match of the saved filter, not a fuzzy nearest-neighbour.
		SemanticRatio: 0,
	})
	if err != nil {
		return err
	}

	for _, hit := range res.Hits {
		created, ok := hitCreatedAt(hit)
		if !ok {
			// Cannot gate against start_at without a date; skip rather than
			// mis-notify (jobs.created_at is NOT NULL, so this should not happen —
			// log it so a future index change that drops created_at is detectable).
			log.Printf("notify: hit %d has no created_at, skipping", hit.ID)
			continue
		}
		for _, s := range subs {
			// Only jobs that became matchable at/after the subscription's cutoff.
			if created.Before(s.StartAt.Time) {
				continue
			}
			n, err := r.store.RecordSubscriptionMatch(ctx, db.RecordSubscriptionMatchParams{
				SubscriptionID: s.ID,
				JobID:          hit.ID,
			})
			if err != nil {
				log.Printf("notify: record match sub=%d job=%d: %v", s.ID, hit.ID, err)
				continue
			}
			stats.Matched += int(n) // n is 1 for a newly recorded match, 0 if already known
		}
	}
	return nil
}

// hitCreatedAt parses a hit's created_at (an RFC3339 string in the index) into a
// time, reporting whether it was present and valid.
func hitCreatedAt(hit search.JobDocument) (time.Time, bool) {
	if hit.CreatedAt == nil {
		return time.Time{}, false
	}
	t, err := time.Parse(time.RFC3339, *hit.CreatedAt)
	if err != nil {
		return time.Time{}, false
	}
	return t, true
}
