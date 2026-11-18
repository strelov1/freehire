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

// matchLag is how far before a group's last search its next one keeps paging: a
// posting reaches the index minutes after its created_at, so stopping exactly at the
// last search's instant would miss it.
const matchLag = time.Hour

// maxMatchPages bounds how far one query pages back in a pass; it only cuts a query
// that matches most of the catalogue.
const maxMatchPages = 50

// matchGroup is the set of subscriptions one search serves: those sharing a query. A
// watchlist query (`watchlist=true`) means different companies to each person, so it is
// grouped per user and userID names them; otherwise userID is 0.
type matchGroup struct {
	query  string
	userID int64
}

// match re-runs each distinct saved-search query against the index and records the
// matching jobs for every subscription on that query. Subscriptions are grouped by
// their canonical query string, so a shared query costs one search, except a
// watchlist query, which is one search per subscriber. A job is gated against each
// subscription's start_at, the subscriber's live excluded_skills, and the query's
// minimum skill match over their live skills.
//
// One failing query is logged, COUNTED and skipped, not fatal. The count matters:
// Stats.MatchingCollapsed turns a pass where every query failed into an exit code,
// which would otherwise look identical to a pass with nothing new.
//
// A cancelled context ends the stage instead of counting the rest as failures: the
// remaining queries would all fail instantly against the dead context.
//
// A query is searched only when one of its subscriptions is due (searchDue): a search
// whose result cannot be sent this pass is load on the index for nothing. A query
// searched after a gap pages back to its previous search (matchLag, maxMatchPages),
// so the gap loses nothing.
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
		sctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		if err := r.store.MarkSubscriptionsSearched(sctx, db.MarkSubscriptionsSearchedParams{
			SearchedAt: pgconv.Timestamptz(&now),
			Ids:        searched,
		}); err != nil {
			log.Printf("notify: stamp %d searched subscriptions: %v", len(searched), err)
		}
	}()
	for key, gsubs := range groups {
		if err := r.matchQuery(ctx, key, gsubs, skillsByUser, watched[key.userID], stats); err != nil {
			// Stops but does not fail: the next pass matches what is left, so a plain
			// `systemctl stop` must not paint the unit red, and returning an error would
			// skip Run's summary line.
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
// longer honor, and drops its group from this pass. A parse failure repeats on every
// pass and never delivers. New and resumed subscriptions are refused at the door
// (internal/engage/subscription), so this catches queries saved before a rule existed
// or invalidated by a tightened one. Paused rather than deleted so the owner can fix
// and resume. A failed pause is retried next pass; the query is not searched either
// way, since it would fail or silently widen.
func (r *Runner) pauseUnrunnable(ctx context.Context, groups map[matchGroup][]db.ListActiveSubscriptionsRow, stats *Stats) {
	for key, gsubs := range groups {
		query := key.query
		vals, _ := url.ParseQuery(query)
		// Invalid visa values used to mean false. Dropping them now would widen
		// existing alerts, so pause those queries until their owner fixes them.
		invalidVisa := search.HasInvalidVisaSponsorship(vals)
		if _, err := search.ParseQuery(vals.Get("q")); err == nil && !invalidVisa {
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
// webhook subscription. It mirrors the grouping ClaimSubscriptionMatches and deliver use.
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

// searchDue reports whether one person's subscriptions must be searched this pass:
// when a message could go out, and at least once per interval while it could. The
// paging in matchQuery records the same set whenever the search does run.
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

// skillSetsByUser fetches every distinct subscriber's held and avoided skills in one
// round trip, keyed by user id. A user with no profile row is absent; callers treat
// that as empty sets.
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

// watchlists reads the companies each watchlist-alert owner watches, keyed by user id.
// A user with an empty list is absent.
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
// share it. For a watchlist group, slugs is its owner's companies and narrows the
// search; with none, no search runs, since dropping the filter would widen the alert
// to the whole catalogue.
//
// It reads the newest MatchLimit hits and keeps paging while a full page still
// reaches back past the group's previous search (less matchLag).
func (r *Runner) matchQuery(ctx context.Context, key matchGroup, subs []db.ListActiveSubscriptionsRow, skillsByUser map[int64]subscriberSkills, slugs []string, stats *Stats) error {
	// A group with a never-searched subscription reads one page.
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
			// ledger dedups re-scans.
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

	// Collect every (subscription, job) pair, then record the batch in one round trip.
	var subIDs, jobIDs []int64
	var revisions []int32
	for _, hit := range hits {
		created, ok := hitCreatedAt(hit)
		if !ok {
			// Cannot gate against start_at without a date; skip rather than mis-notify.
			// created_at is NOT NULL, so log it to catch an index change that drops it.
			log.Printf("notify: hit %d has no created_at, skipping", hit.ID)
			continue
		}
		for _, s := range subs {
			// Only jobs that became matchable at/after the subscription's cutoff.
			if created.Before(s.StartAt.Time) {
				continue
			}
			// Skip a job carrying a skill the subscriber currently avoids, checked per
			// (hit, subscriber) against the live excluded_skills, not the saved query.
			skills := skillsByUser[s.UserID]
			if hasAvoidedSkill(hit.Skills, skills.excluded) {
				continue
			}
			// Also per pair, since the percent depends on the reader; it can only thin
			// the pages the shared search read.
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
// query. It is no search filter: the percent depends on the reader, so the index
// cannot answer it.
const minSkillMatchParam = "min_skill_match"

// minSkillMatch reads the query's threshold; 0 means none. It accepts what the web
// filter accepts (1-100), so a value the job list ignores cannot empty a digest.
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
// excluded_skills. Both sides come from the same normalized skill vocabulary, so a
// case-insensitive exact match suffices.
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
