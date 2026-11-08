package notify

import (
	"context"
	"fmt"
	"log"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/strelov1/freehire/internal/application/deliverywindow"
	"github.com/strelov1/freehire/internal/dict/skillvec"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/pgconv"
	"github.com/strelov1/freehire/internal/search/search"
)

// matchLag is how far before a group's last search its next one keeps paging. A
// posting reaches the index minutes after its created_at (the search drain runs
// every minute or two), so a search that stopped exactly at the last one's instant
// would miss what was created before it and indexed after.
const matchLag = time.Hour

// maxMatchPages bounds how far one query pages back in a pass. At MatchLimit 200 it
// is 10,000 postings — measured 2026-11-05, the busiest daily subscription matched
// ~4,000 in a day — so it only cuts a query that matches most of the catalogue.
const maxMatchPages = 50

// matchGroup is the set of subscriptions one search serves: those sharing a query. A
// query that reads the subscriber's own company list (`watchlist=true`) means a different
// set of companies to each person, so it is grouped per person as well and userID names
// them; for every other query userID is 0 and the group is shared.
type matchGroup struct {
	query  string
	userID int64
}

// match re-runs each distinct saved-search query against the index and records
// the matching jobs for every subscription on that query. Subscriptions are
// grouped by their (already-canonical, SPA-serialized) query string, so a query
// shared by many subscribers costs one search — except a watchlist query, which is
// one search per subscriber because it resolves to that subscriber's companies. A job is gated against each
// subscription's start_at, against the subscriber's live excluded_skills (avoid-skills)
// preference, and against the query's minimum skill match over their live skills, so a
// subscription never receives a job older than it, carrying a skill its subscriber
// currently avoids, or below the threshold they saved.
//
// One failing query is logged, COUNTED and skipped, not fatal — the same isolation as
// the per-board ingest crawl, which also counts its failed boards rather than only
// logging them. Without the count a pass in which every query failed printed
// `queries=53 matched=0 ... failed=0` and exited 0, which is byte-identical to a pass
// with genuinely nothing new to match; Stats.MatchingCollapsed is what the worker
// turns that into an exit code with.
//
// A cancelled context ends the stage instead of counting the rest as failures: the
// remaining queries would each fail instantly against the same dead context, so one
// SIGTERM would otherwise read as "every saved search is broken". Same reasoning, and
// the same shape, as forCompanyBatches in cmd/reindex.
//
// A query is searched only when one of its subscriptions is due (searchDue): a
// search whose result cannot be sent this pass is load on the index for nothing.
// Measured 2026-11-05, every five-minute pass ran ~2,100 searches while an
// hourly digest can use one an hour, and Meilisearch was the busiest process on
// the host. A query searched after a gap pages back to its previous search
// (matchLag, maxMatchPages), so the gap loses nothing.
func (r *Runner) match(ctx context.Context, stats *Stats) error {
	now := r.now()
	subs, err := r.store.ListActiveSubscriptions(ctx)
	if err != nil {
		return err
	}
	due := dueSubscriptions(now, subs)

	skillsByUser, err := r.skillSetsByUser(ctx, subs)
	if err != nil {
		return err
	}

	groups := make(map[matchGroup][]db.ListActiveSubscriptionsRow)
	var watchers []int64
	for _, s := range subs {
		key := matchGroup{query: s.Query}
		if vals, _ := url.ParseQuery(s.Query); search.WantsWatchlist(vals) {
			key.userID = s.UserID
			watchers = append(watchers, s.UserID)
		}
		groups[key] = append(groups[key], s)
	}
	watched, err := r.watchlists(ctx, watchers)
	if err != nil {
		return err
	}

	r.pauseUnrunnable(ctx, groups, stats)

	// Only the groups with a due subscription are searched; the rest wait for a
	// pass whose result could be sent.
	for key, gsubs := range groups {
		if !slices.ContainsFunc(gsubs, func(s db.ListActiveSubscriptionsRow) bool { return due[s.ID] }) {
			delete(groups, key)
			stats.SkippedQueries++
		}
	}

	stats.Queries = len(groups)
	done := 0
	var searched []int64
	// Stamped on every exit, a cancelled one included: what was searched and
	// recorded must not be searched again next pass. A failed stamp costs only
	// repeat searches, so it is logged rather than failing the pass.
	defer func() {
		if len(searched) == 0 {
			return
		}
		if err := r.store.MarkSubscriptionsSearched(context.WithoutCancel(ctx), db.MarkSubscriptionsSearchedParams{
			SearchedAt: pgconv.Timestamptz(&now),
			Ids:        searched,
		}); err != nil {
			log.Printf("notify: stamp %d searched subscriptions: %v", len(searched), err)
		}
	}()
	for key, gsubs := range groups {
		if err := r.matchQuery(ctx, key, gsubs, skillsByUser, watched[key.userID], stats); err != nil {
			// `done`, not the failure count: what a cancellation has to report is how
			// far the stage got, and this branch runs before the failure is counted.
			//
			// It STOPS but does not fail, the same rule and for the same reason as the
			// delivery loop below: an unmatched query is simply matched by the next pass,
			// so an ordinary `systemctl stop` is not an outage and must not paint the unit
			// red. Returning an error here would also have skipped Run's summary line —
			// losing the counters exactly when somebody wants to know how far it got.
			if cause := ctx.Err(); cause != nil {
				log.Printf("notify: matching stopped after %d of %d queries: %v", done, stats.Queries, cause)
				return nil
			}
			stats.FailedQueries++
			log.Printf("notify: match query %q failed: %v", key.query, err)
			continue
		}
		done++
		for _, sub := range gsubs {
			searched = append(searched, sub.ID)
		}
	}
	return nil
}

// pauseUnrunnable pauses every subscription whose saved query the search can no
// longer parse, and drops its group from this pass. Such a query fails on every pass
// and never delivers: measured 2026-11-05, one subscription with ten OR alternatives
// had failed every pass for a week while its owner received nothing. New and resumed
// subscriptions are refused at the door (internal/engage/subscription), so this
// catches what predates a rule — a query saved before it existed, or one a tightened
// rule later invalidates. Paused rather than deleted: the owner can fix the search
// and resume it. A failed pause is logged and retried next pass; the query is not
// searched either way, since it would only fail.
func (r *Runner) pauseUnrunnable(ctx context.Context, groups map[matchGroup][]db.ListActiveSubscriptionsRow, stats *Stats) {
	for key, gsubs := range groups {
		query := key.query
		vals, _ := url.ParseQuery(query)
		if _, err := search.ParseQuery(vals.Get("q")); err == nil {
			continue
		}
		delete(groups, key)
		ids := make([]int64, len(gsubs))
		for i, sub := range gsubs {
			ids[i] = sub.ID
		}
		n, err := r.store.DeactivateSubscriptions(ctx, db.DeactivateSubscriptionsParams{Ids: ids, Query: query})
		if err != nil {
			log.Printf("notify: pause %d subscription(s) on unrunnable query %q: %v", len(ids), query, err)
			continue
		}
		log.Printf("notify: paused %d subscription(s) on unrunnable query %q", n, query)
		stats.Paused += int(n)
	}
}

// person is who one delivery message goes to: a user on a channel, or a single
// webhook subscription (a webhook is sent one saved search at a time). It mirrors
// the grouping ClaimSubscriptionMatches and deliver use.
type person struct {
	userID  int64
	channel string
	subID   int64
}

// dueSubscriptions returns the ids of the subscriptions whose search is due this
// pass. A person's subscriptions are decided together, so they are always searched
// in the same pass and one message carries everything each of them has found.
func dueSubscriptions(now time.Time, subs []db.ListActiveSubscriptionsRow) map[int64]bool {
	byPerson := make(map[person][]db.ListActiveSubscriptionsRow)
	for _, s := range subs {
		k := person{userID: s.UserID, channel: s.Channel}
		if s.Channel == ChannelWebhook {
			k.subID = s.ID
		}
		byPerson[k] = append(byPerson[k], s)
	}
	due := make(map[int64]bool, len(subs))
	for _, ps := range byPerson {
		if searchDue(now, ps) {
			for _, s := range ps {
				due[s.ID] = true
			}
		}
	}
	return due
}

// oldestSearch is the earliest last_searched_at among subs, or the zero time when
// any of them has never been searched.
func oldestSearch(subs []db.ListActiveSubscriptionsRow) time.Time {
	var oldest time.Time
	for _, s := range subs {
		if !s.LastSearchedAt.Valid {
			return time.Time{}
		}
		if oldest.IsZero() || s.LastSearchedAt.Time.Before(oldest) {
			oldest = s.LastSearchedAt.Time
		}
	}
	return oldest
}

// searchDue reports whether one person's subscriptions must be searched this pass.
// The rule is "search when a message could go out, and at least once per
// interval while it could": a posting is then found no later than one interval
// after it appears and sent in the same pass, which is the latency the interval
// already promises. Searching between sends only recorded matches earlier — the
// paging in matchQuery records the same set when the search does run.
//
//   - instant: every pass.
//   - interval: when the interval since the last message has elapsed (the
//     ClaimSubscriptionMatches gate), outside quiet hours (the message would be
//     deferred), and only once per interval while nothing is found.
//   - daily: when the day's digest is due (deliverywindow.DigestDue, the delivery
//     gate), at most hourly while it stays due because nothing was found.
//
// A subscription never searched is always due.
func searchDue(now time.Time, subs []db.ListActiveSubscriptionsRow) bool {
	lastSearched := oldestSearch(subs)
	if lastSearched.IsZero() {
		return true
	}
	var lastSent time.Time
	for _, s := range subs {
		if s.LastDigestSentAt.Valid && s.LastDigestSentAt.Time.After(lastSent) {
			lastSent = s.LastDigestSentAt.Time
		}
	}
	// The settings are the account's, so any subscription answers for the person.
	s := subs[0]
	tz := s.Timezone.String
	switch s.DigestFrequency {
	case FrequencyInstant:
		return true
	case FrequencyDaily:
		var sent *time.Time
		if !lastSent.IsZero() {
			sent = &lastSent
		}
		return deliverywindow.DigestDue(now, tz, pgconv.DurationPtr(s.DigestTime), sent) &&
			!now.Before(lastSearched.Add(time.Hour))
	default:
		if deliverywindow.InQuietHours(now, tz, pgconv.DurationPtr(s.QuietHoursStart), pgconv.DurationPtr(s.QuietHoursEnd)) {
			return false
		}
		every := time.Duration(max(s.DigestIntervalHours, MinIntervalHours)) * time.Hour
		dueAt := lastSent.Add(every)
		if !lastSent.IsZero() && now.Before(dueAt) {
			return false
		}
		// Not searched since the message became due, or a whole interval since the
		// last search found nothing.
		return lastSearched.Before(dueAt) || !now.Before(lastSearched.Add(every))
	}
}

// subscriberSkills is what one subscriber's live profile says about skills.
type subscriberSkills struct {
	held     []string
	excluded []string
}

// skillSetsByUser batch-fetches every distinct subscriber's held and avoided skills in one
// round trip, keyed by user id — cheaper than fetching per query group or per subscription,
// since the same subscriber can appear in several groups. A user id with no profile row is
// simply absent from the result; callers treat that as empty sets.
func (r *Runner) skillSetsByUser(ctx context.Context, subs []db.ListActiveSubscriptionsRow) (map[int64]subscriberSkills, error) {
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
	rows, err := r.store.ListUserProfileSkillSets(ctx, userIDs)
	if err != nil {
		return nil, err
	}
	skillsByUser := make(map[int64]subscriberSkills, len(rows))
	for _, row := range rows {
		skillsByUser[row.UserID] = subscriberSkills{held: row.Skills, excluded: row.ExcludedSkills}
	}
	return skillsByUser, nil
}

// watchlists reads the companies each watchlist-alert owner watches, keyed by user id. A
// user with an empty list is simply absent.
func (r *Runner) watchlists(ctx context.Context, userIDs []int64) (map[int64][]string, error) {
	if len(userIDs) == 0 {
		return nil, nil
	}
	slices.Sort(userIDs)
	rows, err := r.store.ListWatchlistSlugsForUsers(ctx, slices.Compact(userIDs))
	if err != nil {
		return nil, fmt.Errorf("read watchlists: %w", err)
	}
	out := make(map[int64][]string)
	for _, row := range rows {
		out[row.UserID] = append(out[row.UserID], row.Slug)
	}
	return out, nil
}

// matchQuery runs one query and records its matches across the subscriptions that
// share it. For a watchlist group, slugs is its owner's companies and narrows the search to
// them; with none, nothing can match and no search runs — dropping the filter instead would
// widen the alert to the whole catalogue.
//
// It reads the newest MatchLimit hits, as it always did, and keeps paging while a
// full page still reaches back past the group's previous search (less matchLag):
// a group searched once an hour can match more than one page in that hour.
func (r *Runner) matchQuery(ctx context.Context, key matchGroup, subs []db.ListActiveSubscriptionsRow, skillsByUser map[int64]subscriberSkills, slugs []string, stats *Stats) error {
	// A group with a never-searched subscription reads one page, as a first search
	// always did.
	since := oldestSearch(subs)
	vals, _ := url.ParseQuery(key.query)
	minMatch := minSkillMatch(vals)
	filter := search.FilterFromValues(vals)
	if key.userID != 0 {
		if len(slugs) == 0 {
			return nil
		}
		filter = search.AndInStrings(filter, "company_slug", slugs)
	}
	for page := 0; page < maxMatchPages; page++ {
		res, err := r.searcher.Search(ctx, search.SearchParams{
			Query:  vals.Get("q"),
			Filter: filter,
			// Freshest first so the bounded window holds the newest matches; the
			// ledger dedups, so re-scanning the same recent jobs each pass is free.
			Sort:   []string{"created_at:desc"},
			Limit:  r.cfg.MatchLimit,
			Offset: page * r.cfg.MatchLimit,
		})
		stats.Searches++
		if err != nil {
			return err
		}
		if err := r.recordHits(ctx, res.Hits, subs, skillsByUser, minMatch, stats); err != nil {
			return err
		}
		if since.IsZero() || len(res.Hits) < r.cfg.MatchLimit {
			return nil
		}
		oldest, ok := hitCreatedAt(res.Hits[len(res.Hits)-1])
		if !ok || oldest.Before(since.Add(-matchLag)) {
			return nil
		}
	}
	return nil
}

// recordHits records one page of a query's hits for every subscription that shares it.
func (r *Runner) recordHits(ctx context.Context, hits []search.JobDocument, subs []db.ListActiveSubscriptionsRow, skillsByUser map[int64]subscriberSkills, minMatch int, stats *Stats) error {

	// Collect every (subscription, job) pair this query's hits produced across every
	// subscription that shares it, then record the whole batch in one round trip —
	// a popular query with many subscribers no longer costs one sequential INSERT per
	// (hit, subscription) pair.
	var subIDs, jobIDs []int64
	var revisions []int32
	for _, hit := range hits {
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
			skills := skillsByUser[s.UserID]
			if hasAvoidedSkill(hit.Skills, skills.excluded) {
				continue
			}
			// The same per-pair evaluation: the percent depends on who is reading. It runs
			// after the shared search, so it can only thin the pages that search read, not
			// reach past them.
			if !meetsSkillMatch(hit.Skills, skills.held, minMatch) {
				continue
			}
			subIDs = append(subIDs, s.ID)
			jobIDs = append(jobIDs, hit.ID)
			revisions = append(revisions, s.QueryRevision)
		}
	}
	if len(subIDs) == 0 {
		return nil
	}
	n, err := r.store.RecordSubscriptionMatches(ctx, db.RecordSubscriptionMatchesParams{
		SubscriptionIds: subIDs,
		JobIds:          jobIDs,
		QueryRevisions:  revisions,
	})
	if err != nil {
		return fmt.Errorf("record matches: %w", err)
	}
	stats.Matched += int(n) // n is the count of newly recorded pairs; already-known pairs are skipped
	return nil
}

// minSkillMatchParam carries the job list's "Minimum skill match" threshold in a saved
// query. It is no search filter — the percent depends on the reader, so the index cannot
// answer it, and FilterFromValues does not read it.
const minSkillMatchParam = "min_skill_match"

// minSkillMatch reads the query's threshold; 0 means none. It accepts what the web filter
// model accepts (1-100), so a value the job list ignores cannot empty a digest.
func minSkillMatch(vals url.Values) int {
	n, err := strconv.Atoi(strings.TrimSpace(vals.Get(minSkillMatchParam)))
	if err != nil || n < 1 || n > 100 {
		return 0
	}
	return n
}

// meetsSkillMatch applies the threshold the way the job list does (computeClientMatch and
// visibleJobs in web/): the rounded share of a job's skills the reader holds, compared
// case-insensitively. With no threshold it passes, and it passes when either side has no
// skills, because then there is no percent to test — the list keeps such a job and offers
// no slider to a reader with no skills.
func meetsSkillMatch(jobSkills, held []string, minPercent int) bool {
	if minPercent == 0 || len(jobSkills) == 0 || len(held) == 0 {
		return true
	}
	return skillvec.MatchPercent(jobSkills, held) >= minPercent
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
