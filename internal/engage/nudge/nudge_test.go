package nudge

import (
	"context"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeStore is a DB-free Store. It serves canned candidate rows and delivery
// context, and records every write the engine makes.
type fakeStore struct {
	// unlinkedTelegram records the user ids a delivery forgot the Telegram chat for.
	unlinkedTelegram []int64

	followUp        []db.ListFollowUpCandidatesRow
	interviewPrep   []db.ListInterviewPrepCandidatesRow
	jobClosed       int64
	autoApplySubmit []db.ListAutoApplySubmittedCandidatesRow
	autoApplyBlock  []db.ListAutoApplyBlockedCandidatesRow
	autoApplyFail   []db.ListAutoApplyFailedCandidatesRow
	autoApplyReady  []db.ListAutoApplyReviewReadyCandidatesRow
	recorded        []db.RecordNudgeParams

	due []int64
	row db.GetNudgeForDeliveryRow
	// rows serves a distinct delivery context per claimed id, for the tests that
	// put several nudges in one pass. When nil every id gets `row`.
	rows map[int64]db.GetNudgeForDeliveryRow

	delivered []int64
	cancelled []int64
	failed    []int64
	released  []int64

	recordedNotifications []db.RecordNotificationParams
	notifyErr             error
}

func (s *fakeStore) ListFollowUpCandidates(context.Context, int32) ([]db.ListFollowUpCandidatesRow, error) {
	return s.followUp, nil
}
func (s *fakeStore) ListInterviewPrepCandidates(context.Context, int32) ([]db.ListInterviewPrepCandidatesRow, error) {
	return s.interviewPrep, nil
}
func (s *fakeStore) SettleClosedApplications(context.Context, int32) (int64, error) {
	return s.jobClosed, nil
}
func (s *fakeStore) ListAutoApplySubmittedCandidates(context.Context, int32) ([]db.ListAutoApplySubmittedCandidatesRow, error) {
	return s.autoApplySubmit, nil
}
func (s *fakeStore) ListAutoApplyBlockedCandidates(context.Context, int32) ([]db.ListAutoApplyBlockedCandidatesRow, error) {
	return s.autoApplyBlock, nil
}
func (s *fakeStore) ListAutoApplyFailedCandidates(context.Context, int32) ([]db.ListAutoApplyFailedCandidatesRow, error) {
	return s.autoApplyFail, nil
}
func (s *fakeStore) ListAutoApplyReviewReadyCandidates(context.Context, int32) ([]db.ListAutoApplyReviewReadyCandidatesRow, error) {
	return s.autoApplyReady, nil
}
func (s *fakeStore) RecordNudge(_ context.Context, arg db.RecordNudgeParams) (int64, error) {
	for _, r := range s.recorded {
		if r.UserID == arg.UserID && r.JobID == arg.JobID && r.Kind == arg.Kind && r.EpisodeKey.Time.Equal(arg.EpisodeKey.Time) {
			return 0, nil // already recorded -> idempotent no-op
		}
	}
	s.recorded = append(s.recorded, arg)
	return 1, nil
}
func (s *fakeStore) ClaimDueNudges(context.Context, db.ClaimDueNudgesParams) ([]int64, error) {
	return s.due, nil
}
func (s *fakeStore) GetNudgeForDelivery(_ context.Context, id int64) (db.GetNudgeForDeliveryRow, error) {
	if s.rows != nil {
		return s.rows[id], nil
	}
	return s.row, nil
}
func (s *fakeStore) MarkNudgeDelivered(_ context.Context, id int64) (int64, error) {
	s.delivered = append(s.delivered, id)
	return 1, nil
}
func (s *fakeStore) CancelNudgeAtFire(_ context.Context, id int64) (int64, error) {
	s.cancelled = append(s.cancelled, id)
	return 1, nil
}
func (s *fakeStore) RecordNudgeDeliveryFailure(_ context.Context, arg db.RecordNudgeDeliveryFailureParams) error {
	s.failed = append(s.failed, arg.ID)
	return nil
}
func (s *fakeStore) ReleaseNudgeClaim(_ context.Context, id int64) error {
	s.released = append(s.released, id)
	return nil
}
func (s *fakeStore) DeleteTelegramLink(_ context.Context, userID int64) (int64, error) {
	s.unlinkedTelegram = append(s.unlinkedTelegram, userID)
	return 1, nil
}
func (s *fakeStore) RecordNotification(_ context.Context, arg db.RecordNotificationParams) (int64, error) {
	if s.notifyErr != nil {
		return 0, s.notifyErr
	}
	s.recordedNotifications = append(s.recordedNotifications, arg)
	return int64(len(s.recordedNotifications)), nil
}

type fakeNotifier struct {
	sent []string // "channel:dest"
	// groups records the kind and message list of each send, so a test can assert
	// what a delivery carried and not only that it happened.
	groups []sentGroup
	err    error
	// errByChannel fails only the named channels, so a test can model one channel
	// dying while another lands — which a blanket err cannot express.
	errByChannel map[string]error
}

type sentGroup struct {
	kind string
	msgs []Message
}

func (n *fakeNotifier) Send(_ context.Context, channel, dest, kind string, ms []Message) error {
	if err, ok := n.errByChannel[channel]; ok {
		return err
	}
	if n.err != nil {
		return n.err
	}
	n.sent = append(n.sent, channel+":"+dest)
	n.groups = append(n.groups, sentGroup{kind: kind, msgs: ms})
	return nil
}

var fixedNow = time.Date(2026, 8, 9, 12, 0, 0, 0, time.UTC)

func newRunner(store Store, notifier Notifier) *Runner {
	r := New(store, notifier, DefaultConfig())
	r.now = func() time.Time { return fixedNow }
	return r
}

func ts(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }

// --- MATCH: follow-up ------------------------------------------------------

func TestMatch_FollowUp_SilentApplicationIsRecorded(t *testing.T) {
	// "applied" tolerates 21 days; 25 days silent crosses it.
	last := fixedNow.Add(-25 * 24 * time.Hour)
	store := &fakeStore{followUp: []db.ListFollowUpCandidatesRow{
		{UserID: 1, JobID: pgtype.Int8{Int64: 2, Valid: true}, Stage: pgtype.Text{String: "applied", Valid: true}, LastActivityAt: ts(last)},
	}}
	r := newRunner(store, &fakeNotifier{})

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 {
		t.Fatalf("recorded = %d, want 1", len(store.recorded))
	}
	got := store.recorded[0]
	if got.UserID != 1 || got.JobID != 2 || got.Kind != KindFollowUp {
		t.Errorf("recorded = %+v, want (1,2,follow_up)", got)
	}
	if !got.EpisodeKey.Time.Equal(last) {
		t.Errorf("episode key = %v, want last_activity_at %v", got.EpisodeKey.Time, last)
	}
	if stats.Matched != 1 {
		t.Errorf("stats.Matched = %d, want 1", stats.Matched)
	}
}

func TestMatch_FollowUp_ActiveApplicationIsNotRecorded(t *testing.T) {
	// 5 days silent is well within "applied"'s 21-day tolerance.
	last := fixedNow.Add(-5 * 24 * time.Hour)
	store := &fakeStore{followUp: []db.ListFollowUpCandidatesRow{
		{UserID: 1, JobID: pgtype.Int8{Int64: 2, Valid: true}, Stage: pgtype.Text{String: "applied", Valid: true}, LastActivityAt: ts(last)},
	}}
	r := newRunner(store, &fakeNotifier{})

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 0 {
		t.Errorf("recorded = %d, want 0 (not yet silent)", len(store.recorded))
	}
}

func TestMatch_FollowUp_TerminalStageIsNeverRecorded(t *testing.T) {
	last := fixedNow.Add(-90 * 24 * time.Hour)
	store := &fakeStore{followUp: []db.ListFollowUpCandidatesRow{
		{UserID: 1, JobID: pgtype.Int8{Int64: 2, Valid: true}, Stage: pgtype.Text{String: "rejected", Valid: true}, LastActivityAt: ts(last)},
	}}
	r := newRunner(store, &fakeNotifier{})

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 0 {
		t.Errorf("recorded = %d, want 0 (terminal stage never accrues silence)", len(store.recorded))
	}
}

// --- MATCH: interview-prep ---------------------------------------------------

func TestMatch_InterviewPrep_RecordsCandidate(t *testing.T) {
	occurred := fixedNow.Add(-2 * time.Hour)
	store := &fakeStore{interviewPrep: []db.ListInterviewPrepCandidatesRow{
		{UserID: 3, JobID: pgtype.Int8{Int64: 4, Valid: true}, OccurredAt: ts(occurred)},
	}}
	r := newRunner(store, &fakeNotifier{})

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(store.recorded) != 1 {
		t.Fatalf("recorded = %d, want 1", len(store.recorded))
	}
	got := store.recorded[0]
	if got.UserID != 3 || got.JobID != 4 || got.Kind != KindInterviewPrep {
		t.Errorf("recorded = %+v, want (3,4,interview_prep)", got)
	}
	if !got.EpisodeKey.Time.Equal(occurred) {
		t.Errorf("episode key = %v, want occurred_at %v", got.EpisodeKey.Time, occurred)
	}
	if stats.Matched != 1 {
		t.Errorf("stats.Matched = %d, want 1", stats.Matched)
	}
}

// --- MATCH: job-closed ---------------------------------------------------------

func TestMatch_JobClosed_CountsAtomicallyRecordedNotifications(t *testing.T) {
	store := &fakeStore{jobClosed: 1}
	stats, err := newRunner(store, &fakeNotifier{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Matched != 1 {
		t.Fatalf("matched = %d, want 1", stats.Matched)
	}
	if len(store.recorded) != 0 {
		t.Fatal("closure notifications must be recorded by the settlement query")
	}
}

func TestDeliver_JobClosed_DeliversAfterAutomaticExpiry(t *testing.T) {
	chat := int64(42)
	store := &fakeStore{due: []int64{1}, row: deliveryRow(KindJobClosed, "expired", 0, true, false, []string{"telegram"}, &chat, "")}
	stats, err := newRunner(store, &fakeNotifier{}).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Delivered != 1 {
		t.Fatalf("delivered = %d, want 1", stats.Delivered)
	}
}

// --- DELIVER -----------------------------------------------------------------

func deliveryRow(kind string, stage string, daysSilent int, notificationsEnabled, jobOpen bool, channels []string, chatID *int64, email string) db.GetNudgeForDeliveryRow {
	row := db.GetNudgeForDeliveryRow{
		ID: 1, JobID: 2, Kind: kind,
		Title: "Go Dev", Company: "Acme", PublicSlug: "go-dev-acme", URL: "https://ats/x",
		JobOpen: jobOpen, NotificationsEnabled: notificationsEnabled, Channels: channels,
		Stage:                   pgtype.Text{String: stage, Valid: stage != ""},
		ApplicationExists:       true,
		JobClosedEpisodeCurrent: true,
		LastActivityAt: pgtype.Timestamptz{
			Time: fixedNow.Add(-time.Duration(daysSilent) * 24 * time.Hour), Valid: true,
		},
		AccountEmail: email,
	}
	if chatID != nil {
		row.TelegramChatID = pgtype.Int8{Int64: *chatID, Valid: true}
	}
	return row
}

// groupRow is deliveryRow's multi-nudge variant: a deliverable row of `kind` for
// `userID`, naming its own job, so a test can put several nudges in one pass.
func groupRow(id, userID int64, kind string) db.GetNudgeForDeliveryRow {
	row := deliveryRow(kind, stageFor(kind), 25, true, true, []string{"email"}, nil, "a@b.c")
	row.ID, row.UserID, row.JobID = id, userID, id
	row.Title = "Job " + strconv.FormatInt(id, 10)
	row.PublicSlug = "job-" + strconv.FormatInt(id, 10)
	return row
}

// stageFor is the stage each kind's actionable() check needs to pass.
func stageFor(kind string) string {
	if kind == KindInterviewPrep {
		return "interview"
	}
	return "applied"
}

// Five silent applications used to be five emails in one 30-minute pass.
func TestDeliver_GroupsOneAccountsSameKindNudgesIntoOneSend(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2, 3, 4},
		rows: map[int64]db.GetNudgeForDeliveryRow{
			1: groupRow(1, 42, KindFollowUp),
			2: groupRow(2, 42, KindFollowUp),
			3: groupRow(3, 42, KindFollowUp),
			4: groupRow(4, 42, KindFollowUp),
		},
	}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.groups) != 1 {
		t.Fatalf("groups = %v, want one message for one account and one kind", notifier.groups)
	}
	if got := notifier.groups[0]; got.kind != KindFollowUp || len(got.msgs) != 4 {
		t.Errorf("group = (%q, %d jobs), want (follow_up, 4)", got.kind, len(got.msgs))
	}
	if len(store.delivered) != 4 || stats.Delivered != 4 {
		t.Errorf("delivered = %v (stats %d), want all four stamped", store.delivered, stats.Delivered)
	}
}

// The kinds are different conversations and must not share a message.
func TestDeliver_DifferentKindsStayInSeparateSends(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2, 3},
		rows: map[int64]db.GetNudgeForDeliveryRow{
			1: groupRow(1, 42, KindFollowUp),
			2: groupRow(2, 42, KindFollowUp),
			3: groupRow(3, 42, KindInterviewPrep),
		},
	}
	notifier := &fakeNotifier{}
	r := newRunner(store, notifier)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(notifier.groups) != 2 {
		t.Fatalf("groups = %v, want one message per kind", notifier.groups)
	}
	byKind := map[string]int{}
	for _, g := range notifier.groups {
		byKind[g.kind] = len(g.msgs)
	}
	if byKind[KindFollowUp] != 2 || byKind[KindInterviewPrep] != 1 {
		t.Errorf("group sizes = %v, want follow_up=2 interview_prep=1", byKind)
	}
}

// Each group records its own notification, keeping the per-kind vocabulary the
// notification center already filters on.
func TestDeliver_EachGroupRecordsItsOwnKindedNotification(t *testing.T) {
	store := &fakeStore{
		due: []int64{1, 2, 3},
		rows: map[int64]db.GetNudgeForDeliveryRow{
			1: groupRow(1, 42, KindFollowUp),
			2: groupRow(2, 42, KindFollowUp),
			3: groupRow(3, 42, KindInterviewPrep),
		},
	}
	r := newRunner(store, &fakeNotifier{})

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.recordedNotifications) != 2 {
		t.Fatalf("recorded = %v, want one notification per group", store.recordedNotifications)
	}
	byKind := map[string]db.RecordNotificationParams{}
	for _, rec := range store.recordedNotifications {
		byKind[rec.Kind] = rec
	}
	multi, ok := byKind["nudge_"+KindFollowUp]
	if !ok {
		t.Fatalf("kinds recorded = %v, want a nudge_follow_up row", byKind)
	}
	if multi.PublicSlug.Valid {
		t.Errorf("PublicSlug = %+v, want unset for a two-job group", multi.PublicSlug)
	}
	if len(multi.Jobs) == 0 {
		t.Errorf("Jobs is empty, want the group's job list")
	}
	single, ok := byKind["nudge_"+KindInterviewPrep]
	if !ok {
		t.Fatalf("kinds recorded = %v, want a nudge_interview_prep row", byKind)
	}
	if !single.PublicSlug.Valid || single.PublicSlug.String != "job-3" {
		t.Errorf("PublicSlug = %+v, want the single job's slug", single.PublicSlug)
	}
}

// Beyond SnapshotCap the excess is RELEASED, never stamped: a nudge marked delivered
// while appearing in no message is gone for good.
func TestDeliver_BatchOverflowIsReleasedNotDelivered(t *testing.T) {
	cfg := DefaultConfig()
	cfg.SnapshotCap = 2
	store := &fakeStore{
		due: []int64{1, 2, 3},
		rows: map[int64]db.GetNudgeForDeliveryRow{
			1: groupRow(1, 42, KindFollowUp),
			2: groupRow(2, 42, KindFollowUp),
			3: groupRow(3, 42, KindFollowUp),
		},
	}
	notifier := &fakeNotifier{}
	r := New(store, notifier, cfg)
	r.now = func() time.Time { return fixedNow }

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(notifier.groups) != 1 || len(notifier.groups[0].msgs) != 2 {
		t.Fatalf("groups = %v, want one message carrying the cap", notifier.groups)
	}
	if len(store.delivered) != 2 {
		t.Errorf("delivered = %v, want only what the message carried", store.delivered)
	}
	if len(store.released) != 1 || len(store.failed) != 0 {
		t.Errorf("released = %v failed = %v, want the overflow released and no attempt burnt", store.released, store.failed)
	}
	if stats.Deferred != 1 {
		t.Errorf("stats.Deferred = %d, want the overflow nudge counted", stats.Deferred)
	}
}

// A hand-built Config with no SnapshotCap would make every batch full at zero
// members: nothing delivered, nothing failed, forever.
func TestNew_ZeroSnapshotCapDoesNotStallDelivery(t *testing.T) {
	store := &fakeStore{
		due:  []int64{1},
		rows: map[int64]db.GetNudgeForDeliveryRow{1: groupRow(1, 42, KindFollowUp)},
	}
	r := New(store, &fakeNotifier{}, Config{FollowUpWindowDays: 30, LeaseSeconds: 600, ClaimBatch: 500, MaxAttempts: 5})
	r.now = func() time.Time { return fixedNow }
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.delivered) != 1 {
		t.Errorf("delivered = %v, want the nudge delivered despite an unset SnapshotCap", store.delivered)
	}
}

func TestRecipient_Push_ResolvesUserIDWhenDeviceRegistered(t *testing.T) {
	info := db.GetNudgeForDeliveryRow{UserID: 42, HasPushDevice: true}

	dest, ok := recipient(notify.ChannelPush, info)
	if !ok {
		t.Fatal("ok = false, want true (a registered device is deliverable)")
	}
	if dest != "42" {
		t.Errorf("dest = %q, want %q", dest, "42")
	}
}

func pgTime(hh, mm int) pgtype.Time {
	return pgtype.Time{Microseconds: int64(hh)*int64(time.Hour/time.Microsecond) + int64(mm)*int64(time.Minute/time.Microsecond), Valid: true}
}

func TestJobClosedRejectsStaleEpisode(t *testing.T) {
	for _, stage := range []string{"applied", "expired"} {
		t.Run(stage, func(t *testing.T) {
			row := deliveryRow(KindJobClosed, stage, 0, true, false, nil, nil, "")
			row.JobClosedEpisodeCurrent = false
			if newRunner(&fakeStore{}, &fakeNotifier{}).actionable(row) {
				t.Fatal("stale closure episode must not be delivered")
			}
		})
	}
}

type ctxCheckingStore struct {
	*fakeStore
	liveWithDeadline bool
}

func (s *ctxCheckingStore) ReleaseNudgeClaim(ctx context.Context, id int64) error {
	_, ok := ctx.Deadline()
	s.liveWithDeadline = ctx.Err() == nil && ok
	return s.fakeStore.ReleaseNudgeClaim(ctx, id)
}

func TestRelease_SurvivesCancelledRunContext(t *testing.T) {
	store := &ctxCheckingStore{fakeStore: &fakeStore{}}
	r := New(store, &fakeNotifier{}, DefaultConfig())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	r.release(ctx, 7)
	if !store.liveWithDeadline || len(store.released) != 1 {
		t.Fatalf("release ran on a dead or unbounded context (live+deadline=%v, released=%v)", store.liveWithDeadline, store.released)
	}
}
