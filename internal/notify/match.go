package notify

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/db"
	"github.com/strelov1/freehire/internal/search"
)

// match re-runs each distinct saved-search query against the index and records
// the matching jobs for every subscription on that query. Subscriptions are
// grouped by their (already-canonical, SPA-serialized) query string, so a query
// shared by many subscribers costs one search. A job is gated against each
// subscription's start_at, and against the subscriber's live excluded_skills
// (avoid-skills) preference, so a subscription never receives a job older than
// it or carrying a skill its subscriber currently avoids.
//
// One failing query is logged and skipped, not fatal — the same isolation as the
// per-board ingest crawl.
func (r *Runner) match(ctx context.Context, stats *Stats) error {
	subs, err := r.store.ListActiveSubscriptions(ctx)
	if err != nil {
		return err
	}

	excludedByUser, err := r.excludedSkillsByUser(ctx, subs)
	if err != nil {
		return err
	}

	groups := make(map[string][]db.ListActiveSubscriptionsRow)
	for _, s := range subs {
		groups[s.Query] = append(groups[s.Query], s)
	}

	stats.Queries = len(groups)
	for query, gsubs := range groups {
		if err := r.matchQuery(ctx, query, gsubs, excludedByUser, stats); err != nil {
			log.Printf("notify: match query %q failed: %v", query, err)
			continue
		}
	}
	return nil
}

// excludedSkillsByUser batch-fetches every distinct subscriber's avoid-skills preference in
// one round trip, keyed by user id — cheaper than fetching per query group or per
// subscription, since the same subscriber can appear in several groups. A user id with no
// profile row is simply absent from the result; callers treat that as an empty exclude set.
func (r *Runner) excludedSkillsByUser(ctx context.Context, subs []db.ListActiveSubscriptionsRow) (map[int64][]string, error) {
	seen := make(map[int64]struct{}, len(subs))
	userIDs := make([]int64, 0, len(subs))
	for _, s := range subs {
		if _, ok := seen[s.UserID]; ok {
			continue
		}
		seen[s.UserID] = struct{}{}
		userIDs = append(userIDs, s.UserID)
	}
	if len(userIDs) == 0 {
		return nil, nil
	}
	rows, err := r.store.ListUserProfilesExcludedSkills(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	excludedByUser := make(map[int64][]string, len(rows))
	for _, row := range rows {
		excludedByUser[row.UserID] = row.ExcludedSkills
	}
	return excludedByUser, nil
}

// matchQuery runs one query and records its matches across the subscriptions that
// share it.
func (r *Runner) matchQuery(ctx context.Context, query string, subs []db.ListActiveSubscriptionsRow, excludedByUser map[int64][]string, stats *Stats) error {
	vals, _ := url.ParseQuery(query)
	res, err := r.searcher.Search(ctx, search.SearchParams{
		Query:  vals.Get("q"),
		Filter: search.FilterFromValues(vals),
		// Freshest first so the bounded window holds the newest matches; the
		// ledger dedups, so re-scanning the same recent jobs each pass is free.
		Sort:  []string{"created_at:desc"},
		Limit: r.cfg.MatchLimit,
	})
	if err != nil {
		return err
	}

	// Collect every (subscription, job) pair this query's hits produced across every
	// subscription that shares it, then record the whole batch in one round trip —
	// a popular query with many subscribers no longer costs one sequential INSERT per
	// (hit, subscription) pair.
	var subIDs, jobIDs []int64
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
			// Skip a job carrying any skill the subscriber currently avoids. This is
			// evaluated per (hit, subscriber) pair against the live excluded_skills,
			// not a value frozen into the saved search's own query string — no extra
			// search call, since the shared search above already ran once per query.
			if hasAvoidedSkill(hit.Skills, excludedByUser[s.UserID]) {
				continue
			}
			subIDs = append(subIDs, s.ID)
			jobIDs = append(jobIDs, hit.ID)
		}
	}
	if len(subIDs) == 0 {
		return nil
	}
	n, err := r.store.RecordSubscriptionMatches(ctx, db.RecordSubscriptionMatchesParams{
		SubscriptionIds: subIDs,
		JobIds:          jobIDs,
	})
	if err != nil {
		return fmt.Errorf("record matches: %w", err)
	}
	stats.Matched += int(n) // n is the count of newly recorded pairs; already-known pairs are skipped
	return nil
}

// hasAvoidedSkill reports whether any of a job's skills is in the subscriber's
// excluded_skills. Both sides come from the same canonical, dict-only skill
// vocabulary, normalized (lowercased/trimmed) on save, so a case-insensitive exact
// match is sufficient — no partial or fuzzy matching.
func hasAvoidedSkill(jobSkills, excluded []string) bool {
	if len(excluded) == 0 {
		return false
	}
	for _, skill := range jobSkills {
		for _, avoided := range excluded {
			if strings.EqualFold(skill, avoided) {
				return true
			}
		}
	}
	return false
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
