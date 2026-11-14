package notify

import (
	"context"
	"slices"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/job/jobview"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
)

// --- fakes ---------------------------------------------------------------

type fakeSearcher struct {
	// byQuery maps a query's "q" value to the hits it should return; queries are
	// keyed by the parsed q so a test can return different hits per query.
	results []search.SearchResult
	// errs is indexed by call number like results: a non-nil entry fails that one
	// query, which is how the per-query isolation can be exercised without failing
	// them all. err fails every call.
	errs  []error
	err   error
	calls []search.SearchParams
}

func (f *fakeSearcher) Search(ctx context.Context, p search.SearchParams) (search.SearchResult, error) {
	f.calls = append(f.calls, p)
	// A real index client fails a cancelled call rather than serving it, which is what
	// makes a cancelled pass look like every query failing.
	if err := ctx.Err(); err != nil {
		return search.SearchResult{}, err
	}
	i := len(f.calls) - 1
	if f.err != nil {
		return search.SearchResult{}, f.err
	}
	if i < len(f.errs) && f.errs[i] != nil {
		return search.SearchResult{}, f.errs[i]
	}
	if i < len(f.results) {
		return f.results[i], nil
	}
	return search.SearchResult{}, nil
}

type recordedMatch struct{ sub, job int64 }

type fakeStore struct {
	// unlinkedTelegram records the user ids a delivery forgot the Telegram chat
	// for, so a test can assert the 403 path unlinks exactly once per user.
	unlinkedTelegram  []int64
	unlinkTelegramErr error

	// disabledWebhooks records the user ids a delivery disabled the webhook
	// destination for, mirroring unlinkedTelegram for the webhook channel's own
	// "recipient gone" signal (a 410 response).
	disabledWebhooks  []int64
	disableWebhookErr error

	// recordedWebhookSuccesses records the user ids a successful webhook
	// delivery stamped last_success_at for.
	recordedWebhookSuccesses []int64

	// recordedWebhookFailures records every RecordWebhookDeliveryFailure call a
	// non-410 webhook send error made, so a test can assert both that it
	// happened (and with which user/threshold) and that other channels never
	// trigger it.
	recordedWebhookFailures []db.RecordWebhookDeliveryFailureParams
	// webhookFailureResult is what the next RecordWebhookDeliveryFailure call
	// returns, standing in for the row the real UPDATE ... RETURNING would —
	// a test sets Enabled: false to simulate this call crossing the threshold.
	webhookFailureResult db.RecordWebhookDeliveryFailureRow
	webhookFailureErr    error

	// deliveryErr and digestJobsErr fail the two reads deliverMessage makes before it can
	// send anything. Neither burns a delivery attempt, so the counter is the only trace
	// they leave.
	deliveryErr   error
	digestJobsErr error

	active            []db.ListActiveSubscriptionsRow
	recorded          []recordedMatch
	recordedRevisions map[int64]int32 // revision forwarded per subscription
	recordCalls       int             // how many times RecordSubscriptionMatches was called, not how many pairs
	claimed           []db.ClaimSubscriptionMatchesRow
	delivery          map[int64]db.GetSubscriptionForDeliveryRow
	// listed, when set, is what ListDeliveryCandidates reads instead of delivery: the
	// state the pre-claim gate saw, for a test where the world moves before the recheck.
	listed map[int64]db.GetSubscriptionForDeliveryRow
	// claimParams records every ClaimSubscriptionMatches call.
	claimParams []db.ClaimSubscriptionMatchesParams
	digestJobs  map[int64]db.GetJobsForDigestRow

	// excludedSkills maps a user id to their avoid-skills preference; a user id absent from
	// this map has no profile row, mirroring the real query.
	excludedSkills map[int64][]string
	// heldSkills maps a user id to their profile skills, under the same absence rule.
	heldSkills map[int64][]string
	// watchlists maps a user id to the companies they watch; absent is an empty list.
	watchlists map[int64][]string

	notified   []db.MarkMatchesNotifiedParams
	failures   []db.RecordMatchDeliveryFailureParams
	released   []db.ReleaseMatchClaimParams
	digestSent []int64

	recordedNotifications []db.RecordNotificationParams
	recordNotificationErr error
	// nextNotificationID is the id the next RecordNotification hands back, so a
	// test can assert the digest carried it; deletedNotifications records the
	// ids a failed send withdrew.
	nextNotificationID   int64
	deletedNotifications []int64
	// notificationsByDedupKey stands in for the partial unique index on
	// (user_id, dedup_key). The fake enforces it rather than ignoring it: a
	// store that accepted every insert would report a key that dedupes nothing
	// as working, which is the bug these tests exist to catch.
	notificationsByDedupKey map[string]int64
	// beforeDedupRead fires between the conflicting insert and the read that
	// follows it, which is the only window a SECOND notify process can act in.
	// deliver() is one sequential goroutine, so nothing a single pass does can
	// open that window — a test that wants it has to say so.
	beforeDedupRead func(*fakeStore)
	// expireCalls records every ExpireStaleSubscriptionMatches call; expireResults
	// is what successive calls return (0 once exhausted) and expireErr fails them.
	// calls logs the order of the expire and claim statements, since expiring
	// AFTER the claim would hand a stale match to a digest.
	expireCalls   []db.ExpireStaleSubscriptionMatchesParams
	expireResults []int64
	expireErr     error
	calls         []string

	// coveredCount is what CountCoveredMatches answers and coveredErr fails it;
	// coveredCounts and coveredMarked record the cutoffs each was asked about.
	coveredCount  int64
	coveredErr    error
	coveredCounts []db.CountCoveredMatchesParams
	coveredMarked []db.MarkCoveredMatchesNotifiedParams

	// recordNotificationErrOnce fails only the FIRST record, so a test can
	// deliver with no row in hand and still observe what the send path does
	// about it afterwards.
	recordNotificationErrOnce error

	// searched records every MarkSubscriptionsSearched call; the stamp is also
	// written onto the matching active rows, so a second pass reads it back.
	searched []db.MarkSubscriptionsSearchedParams
	// paused records every id DeactivateSubscriptions was handed.
	paused []int64
}

func (s *fakeStore) DeactivateSubscriptions(_ context.Context, a db.DeactivateSubscriptionsParams) (int64, error) {
	s.paused = append(s.paused, a.Ids...)
	return int64(len(a.Ids)), nil
}

func (s *fakeStore) MarkSubscriptionsSearched(_ context.Context, a db.MarkSubscriptionsSearchedParams) error {
	s.searched = append(s.searched, a)
	for i := range s.active {
		if slices.Contains(a.Ids, s.active[i].ID) {
			s.active[i].LastSearchedAt = a.SearchedAt
		}
	}
	return nil
}

func (s *fakeStore) ListActiveSubscriptions(context.Context) ([]db.ListActiveSubscriptionsRow, error) {
	return s.active, nil
}

func (s *fakeStore) ListUserProfileSkillSets(_ context.Context, userIDs []int64) ([]db.ListUserProfileSkillSetsRow, error) {
	out := make([]db.ListUserProfileSkillSetsRow, 0, len(userIDs))
	for _, id := range userIDs {
		excluded, hasExcluded := s.excludedSkills[id]
		held, hasHeld := s.heldSkills[id]
		if !hasExcluded && !hasHeld {
			continue
		}
		out = append(out, db.ListUserProfileSkillSetsRow{UserID: id, Skills: held, ExcludedSkills: excluded})
	}
	return out, nil
}

func (s *fakeStore) ListWatchlistSlugsForUsers(_ context.Context, userIDs []int64) ([]db.ListWatchlistSlugsForUsersRow, error) {
	var out []db.ListWatchlistSlugsForUsersRow
	for _, id := range userIDs {
		for _, slug := range s.watchlists[id] {
			out = append(out, db.ListWatchlistSlugsForUsersRow{UserID: id, Slug: slug})
		}
	}
	return out, nil
}

func (s *fakeStore) RecordSubscriptionMatches(_ context.Context, a db.RecordSubscriptionMatchesParams) (int64, error) {
	s.recordCalls++
	var n int64
	for i, subID := range a.SubscriptionIds {
		jobID := a.JobIds[i]
		if s.recordedRevisions == nil {
			s.recordedRevisions = map[int64]int32{}
		}
		s.recordedRevisions[subID] = a.QueryRevisions[i]
		known := false
		for _, m := range s.recorded {
			if m.sub == subID && m.job == jobID {
				known = true // already recorded → idempotent no-op
				break
			}
		}
		if known {
			continue
		}
		s.recorded = append(s.recorded, recordedMatch{subID, jobID})
		n++
	}
	return n, nil
}

// ClaimSubscriptionMatches returns the scripted claim. A row that names no owner
// stands for a subscription of its own person (UserID = SubscriptionID), so a case
// written before messages were bundled per person still sends one per subscription;
// the bundling cases set UserID and Channel explicitly.
func (s *fakeStore) ClaimSubscriptionMatches(_ context.Context, a db.ClaimSubscriptionMatchesParams) ([]db.ClaimSubscriptionMatchesRow, error) {
	s.calls = append(s.calls, "claim")
	s.claimParams = append(s.claimParams, a)
	rows := make([]db.ClaimSubscriptionMatchesRow, 0, len(s.claimed))
	for _, c := range s.claimed {
		if c.UserID == 0 {
			c.UserID = c.SubscriptionID
		}
		// Like the SQL, only the subscriptions the caller named are leased.
		if slices.Contains(a.SubscriptionIds, c.SubscriptionID) {
			rows = append(rows, c)
		}
	}
	// Like the SQL, a row unscripted on the point carries the owner's newest stamp on
	// the channel, read from the delivery rows.
	for i := range rows {
		if rows[i].PersonLastSentAt.Valid {
			continue
		}
		info := s.delivery[rows[i].SubscriptionID]
		info.ID, info.UserID = rows[i].SubscriptionID, rows[i].UserID
		rows[i].PersonLastSentAt = s.lastDigestSent(s.delivery, info)
	}
	return rows, nil
}

// Delivery fixtures include active siblings even when they have no pending matches.
func (s *fakeStore) lastDigestSent(src map[int64]db.GetSubscriptionForDeliveryRow, info db.GetSubscriptionForDeliveryRow) pgtype.Timestamptz {
	last := info.PersonLastSentAt
	for id, other := range src {
		ownerID := other.UserID
		if ownerID == 0 {
			ownerID = id
			for _, claim := range s.claimed {
				if claim.SubscriptionID == id && claim.UserID != 0 {
					ownerID = claim.UserID
					break
				}
			}
		}
		if ownerID != info.UserID || other.Channel != info.Channel ||
			(info.Channel == ChannelWebhook && id != info.ID) {
			continue
		}
		if t := other.LastDigestSentAt; t.Valid && (!last.Valid || t.Time.After(last.Time)) {
			last = t
		}
	}
	return last
}

// ListDeliveryCandidates lists the subscriptions the scripted claim holds matches for,
// each with its delivery row, in subscription order.
func (s *fakeStore) ListDeliveryCandidates(context.Context) ([]db.ListDeliveryCandidatesRow, error) {
	s.calls = append(s.calls, "candidates")
	src := s.delivery
	if s.listed != nil {
		src = s.listed
	}
	var rows []db.ListDeliveryCandidatesRow
	seen := map[int64]bool{}
	for _, c := range s.claimed {
		if seen[c.SubscriptionID] {
			continue
		}
		seen[c.SubscriptionID] = true
		info := src[c.SubscriptionID]
		info.ID = c.SubscriptionID
		if info.UserID == 0 {
			info.UserID = c.UserID
			if info.UserID == 0 {
				info.UserID = c.SubscriptionID
			}
		}
		info.PersonLastSentAt = s.lastDigestSent(src, info)
		rows = append(rows, db.ListDeliveryCandidatesRow(info))
	}
	return rows, nil
}

func (s *fakeStore) ExpireStaleSubscriptionMatches(_ context.Context, a db.ExpireStaleSubscriptionMatchesParams) (int64, error) {
	s.calls = append(s.calls, "expire")
	s.expireCalls = append(s.expireCalls, a)
	if s.expireErr != nil {
		return 0, s.expireErr
	}
	i := len(s.expireCalls) - 1
	if i < len(s.expireResults) {
		return s.expireResults[i], nil
	}
	return 0, nil
}

func (s *fakeStore) CountCoveredMatches(_ context.Context, a db.CountCoveredMatchesParams) (int64, error) {
	s.coveredCounts = append(s.coveredCounts, a)
	return s.coveredCount, s.coveredErr
}

func (s *fakeStore) MarkCoveredMatchesNotified(_ context.Context, a db.MarkCoveredMatchesNotifiedParams) (int64, error) {
	s.coveredMarked = append(s.coveredMarked, a)
	return s.coveredCount, nil
}

func (s *fakeStore) GetSubscriptionForDelivery(_ context.Context, id int64) (db.GetSubscriptionForDeliveryRow, error) {
	return s.delivery[id], s.deliveryErr
}

func (s *fakeStore) GetJobsForDigest(_ context.Context, ids []int64) ([]db.GetJobsForDigestRow, error) {
	if s.digestJobsErr != nil {
		return nil, s.digestJobsErr
	}
	out := make([]db.GetJobsForDigestRow, 0, len(ids))
	for _, id := range ids {
		if j, ok := s.digestJobs[id]; ok {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *fakeStore) MarkMatchesNotified(_ context.Context, a db.MarkMatchesNotifiedParams) (int64, error) {
	s.notified = append(s.notified, a)
	return int64(len(a.JobIds)), nil
}

func (s *fakeStore) RecordMatchDeliveryFailure(_ context.Context, a db.RecordMatchDeliveryFailureParams) error {
	s.failures = append(s.failures, a)
	return nil
}

func (s *fakeStore) ReleaseMatchClaim(_ context.Context, a db.ReleaseMatchClaimParams) error {
	s.released = append(s.released, a)
	return nil
}

func (s *fakeStore) MarkDigestSent(_ context.Context, id int64) error {
	s.digestSent = append(s.digestSent, id)
	return nil
}

func (s *fakeStore) RecordNotification(_ context.Context, a db.RecordNotificationParams) (int64, error) {
	s.recordedNotifications = append(s.recordedNotifications, a)
	if s.recordNotificationErr != nil {
		return 0, s.recordNotificationErr
	}
	if s.recordNotificationErrOnce != nil {
		err := s.recordNotificationErrOnce
		s.recordNotificationErrOnce = nil
		return 0, err
	}
	// ON CONFLICT ... DO NOTHING RETURNING id: a key already recorded inserts
	// nothing and yields no row.
	if _, taken := s.notificationsByDedupKey[a.DedupKey.String]; taken && a.DedupKey.Valid {
		return 0, pgx.ErrNoRows
	}
	if s.nextNotificationID == 0 {
		s.nextNotificationID = 1
	}
	id := s.nextNotificationID
	s.nextNotificationID++
	if a.DedupKey.Valid {
		if s.notificationsByDedupKey == nil {
			s.notificationsByDedupKey = make(map[string]int64)
		}
		s.notificationsByDedupKey[a.DedupKey.String] = id
	}
	return id, nil
}

func (s *fakeStore) GetNotificationIDByDedupKey(_ context.Context, a db.GetNotificationIDByDedupKeyParams) (int64, error) {
	if s.beforeDedupRead != nil {
		s.beforeDedupRead(s)
	}
	id, ok := s.notificationsByDedupKey[a.DedupKey.String]
	if !ok {
		return 0, pgx.ErrNoRows
	}
	return id, nil
}

func (s *fakeStore) DeleteNotification(_ context.Context, id int64) error {
	s.deletedNotifications = append(s.deletedNotifications, id)
	// A withdrawn row frees its key, so the retry that follows records afresh
	// rather than reading back an id that no longer exists.
	for key, recorded := range s.notificationsByDedupKey {
		if recorded == id {
			delete(s.notificationsByDedupKey, key)
		}
	}
	return nil
}

func (s *fakeStore) DeleteTelegramLink(_ context.Context, userID int64) (int64, error) {
	s.unlinkedTelegram = append(s.unlinkedTelegram, userID)
	if s.unlinkTelegramErr != nil {
		return 0, s.unlinkTelegramErr
	}
	return 1, nil
}

func (s *fakeStore) DisableWebhookConfig(_ context.Context, userID int64) (int64, error) {
	s.disabledWebhooks = append(s.disabledWebhooks, userID)
	if s.disableWebhookErr != nil {
		return 0, s.disableWebhookErr
	}
	return 1, nil
}

func (s *fakeStore) RecordWebhookDeliverySuccess(_ context.Context, userID int64) error {
	s.recordedWebhookSuccesses = append(s.recordedWebhookSuccesses, userID)
	return nil
}

func (s *fakeStore) RecordWebhookDeliveryFailure(_ context.Context, a db.RecordWebhookDeliveryFailureParams) (db.RecordWebhookDeliveryFailureRow, error) {
	s.recordedWebhookFailures = append(s.recordedWebhookFailures, a)
	if s.webhookFailureErr != nil {
		return db.RecordWebhookDeliveryFailureRow{}, s.webhookFailureErr
	}
	return s.webhookFailureResult, nil
}

type fakeNotifier struct {
	err  error
	sent []Digest
	// messages is every send as the channel received it; sent flattens their digests.
	messages []Message
}

// Send records the message even when it fails, so a test can assert what the
// channel was handed on the failure path too.
func (n *fakeNotifier) Send(_ context.Context, _, _ string, m Message) error {
	n.messages = append(n.messages, m)
	n.sent = append(n.sent, m.Digests...)
	return n.err
}

// failAfterNotifier lets the first `succeed` sends through and fails every one
// after, so a test can put one channel's success and the next channel's failure
// in the same pass — which is what makes "who may withdraw the shared row"
// observable.
type failAfterNotifier struct {
	succeed int
	err     error
	sent    []Digest
}

func (n *failAfterNotifier) Send(_ context.Context, _, _ string, m Message) error {
	n.sent = append(n.sent, m.Digests...)
	if len(n.sent) > n.succeed {
		return n.err
	}
	return nil
}

// --- helpers -------------------------------------------------------------

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

func rfc(t time.Time) *string { s := t.Format(time.RFC3339); return &s }

func hit(id int64, created time.Time) search.JobDocument {
	return search.JobDocument{ID: id, Job: jobview.Job{CreatedAt: rfc(created)}}
}

func hitWithSkills(id int64, created time.Time, skills ...string) search.JobDocument {
	return search.JobDocument{ID: id, Job: jobview.Job{CreatedAt: rfc(created), Skills: skills}}
}
