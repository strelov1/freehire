//go:build integration

// Integration tests for the SQL the CV lifecycle letters stand on, against a real Postgres:
// enrollment, the holdout, the offsets, the eligibility re-check inside the claim, the
// paying observation and the shared daily claim. Run with:
// go test -tags=integration ./internal/engage/cvlifecycle/
package cvlifecycle

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/maildaily"
	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

const day = 24 * time.Hour

// newUser inserts a verified free account whose CV parsed `uploadedAgo` ago.
func newUser(t *testing.T, pool *pgxpool.Pool, label string, uploadedAgo time.Duration) int64 {
	t.Helper()
	at := time.Now().Add(-uploadedAgo)
	var id int64
	err := pool.QueryRow(context.Background(), `
		INSERT INTO users (email, email_verified, resume_uploaded_at, resume_extract_status, resume_structured_uploaded_at)
		VALUES ($1, true, $2, 'ok', $2) RETURNING id`,
		fmt.Sprintf("%s@example.test", label), at).Scan(&id)
	if err != nil {
		t.Fatalf("insert user %s: %v", label, err)
	}
	return id
}

func exec(t *testing.T, pool *pgxpool.Pool, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(context.Background(), sql, args...); err != nil {
		t.Fatalf("%s: %v", sql, err)
	}
}

func isHoldout(t *testing.T, pool *pgxpool.Pool, id int64) bool {
	t.Helper()
	var h bool
	if err := pool.QueryRow(context.Background(), `SELECT cv_lifecycle_holdout($1)`, id).Scan(&h); err != nil {
		t.Fatalf("holdout: %v", err)
	}
	return h
}

// mailable inserts accounts until it has one that is, and one that is not, in the holdout,
// returning (mailed, held) ids. The split is a hash, so which ids land where is not chosen.
func mailable(t *testing.T, pool *pgxpool.Pool, prefix string, uploadedAgo time.Duration) (mailed, held int64) {
	t.Helper()
	for i := 0; mailed == 0 || held == 0; i++ {
		if i > 200 {
			t.Fatal("no holdout/non-holdout pair in 200 accounts")
		}
		id := newUser(t, pool, fmt.Sprintf("%s-%d", prefix, i), uploadedAgo)
		if isHoldout(t, pool, id) {
			if held == 0 {
				held = id
			}
		} else if mailed == 0 {
			mailed = id
		}
	}
	return mailed, held
}

func newRunner(t *testing.T, q *db.Queries) (*Runner, *fakeSender) {
	t.Helper()
	store := newFakeStore()
	sender := &fakeSender{store: store, err: map[int64]error{}}
	r, err := New(q, &fakeComposer{err: map[int64]error{}}, sender, DefaultConfig())
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return r, sender
}

func sentTo(s *fakeSender, id int64) bool {
	for _, line := range s.sent {
		var step int
		var to int64
		if _, err := fmt.Sscanf(line, "%d:%d:", &step, &to); err == nil && to == id {
			return true
		}
	}
	return false
}

func sentSteps(t *testing.T, pool *pgxpool.Pool, id int64) []int {
	t.Helper()
	rows, err := pool.Query(context.Background(), `SELECT step FROM cv_lifecycle_sends WHERE user_id = $1 AND error = '' ORDER BY step`, id)
	if err != nil {
		t.Fatalf("sends: %v", err)
	}
	defer rows.Close()
	var steps []int
	for rows.Next() {
		var s int16
		if err := rows.Scan(&s); err != nil {
			t.Fatal(err)
		}
		steps = append(steps, int(s))
	}
	return steps
}

func enrollment(t *testing.T, pool *pgxpool.Pool, id int64) (status string, paid, exited bool, holdout bool) {
	t.Helper()
	err := pool.QueryRow(context.Background(),
		`SELECT status, paid_at IS NOT NULL, exited_at IS NOT NULL, holdout FROM cv_lifecycle_enrollments WHERE user_id = $1`, id,
	).Scan(&status, &paid, &exited, &holdout)
	if err != nil {
		t.Fatalf("enrollment of %d: %v", id, err)
	}
	return status, paid, exited, holdout
}

func TestEnrollment_AtMostOnceAndTheUploadDateNeverMoves(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	id := newUser(t, pool, "enroll-once", 2*day)

	enroll := func() int64 {
		n, err := q.EnrollCVLifecycleUsers(context.Background(), db.EnrollCVLifecycleUsersParams{WindowDays: 14, MaxRows: 100})
		if err != nil {
			t.Fatalf("enroll: %v", err)
		}
		return n
	}
	if n := enroll(); n != 1 {
		t.Fatalf("first enroll = %d, want 1", n)
	}
	var anchor time.Time
	if err := pool.QueryRow(context.Background(), `SELECT uploaded_at FROM cv_lifecycle_enrollments WHERE user_id = $1`, id).Scan(&anchor); err != nil {
		t.Fatal(err)
	}

	// A re-upload, parsed again, is not a new enrollment and does not move the anchor.
	exec(t, pool, `UPDATE users SET resume_uploaded_at = now(), resume_structured_uploaded_at = now() WHERE id = $1`, id)
	if n := enroll(); n != 0 {
		t.Fatalf("second enroll = %d, want 0", n)
	}
	var after time.Time
	var rowsN int
	if err := pool.QueryRow(context.Background(),
		`SELECT uploaded_at, (SELECT count(*) FROM cv_lifecycle_enrollments WHERE user_id = $1) FROM cv_lifecycle_enrollments WHERE user_id = $1`, id,
	).Scan(&after, &rowsN); err != nil {
		t.Fatal(err)
	}
	if !after.Equal(anchor) || rowsN != 1 {
		t.Fatalf("anchor moved or duplicated: %v -> %v, rows %d", anchor, after, rowsN)
	}
}

func TestEnrollment_OnlyMailableParsedFreeRecentAccounts(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ok := newUser(t, pool, "ok", day)
	unverified := newUser(t, pool, "unverified", day)
	exec(t, pool, `UPDATE users SET email_verified = false WHERE id = $1`, unverified)
	unparsed := newUser(t, pool, "unparsed", day)
	exec(t, pool, `UPDATE users SET resume_extract_status = 'failed' WHERE id = $1`, unparsed)
	reuploaded := newUser(t, pool, "reuploaded", day)
	exec(t, pool, `UPDATE users SET resume_uploaded_at = now() WHERE id = $1`, reuploaded)
	paying := newUser(t, pool, "paying", day)
	exec(t, pool, `UPDATE users SET pro_until_stripe = now() + interval '10 days' WHERE id = $1`, paying)
	old := newUser(t, pool, "old", 30*day)

	if n, err := q.CountCVLifecycleEnrollable(context.Background(), 14); err != nil || n != 1 {
		t.Fatalf("enrollable = %d, %v; want 1", n, err)
	}
	if n, err := q.EnrollCVLifecycleUsers(context.Background(), db.EnrollCVLifecycleUsersParams{WindowDays: 14, MaxRows: 100}); err != nil || n != 1 {
		t.Fatalf("enrolled = %d, %v; want 1", n, err)
	}
	for id, want := range map[int64]bool{ok: true, unverified: false, unparsed: false, reuploaded: false, paying: false, old: false} {
		var got bool
		if err := pool.QueryRow(context.Background(), `SELECT EXISTS (SELECT 1 FROM cv_lifecycle_enrollments WHERE user_id = $1)`, id).Scan(&got); err != nil {
			t.Fatal(err)
		}
		if got != want {
			t.Errorf("user %d enrolled = %v, want %v", id, got, want)
		}
	}
}

func TestHoldout_IsStableAndAboutATenth(t *testing.T) {
	pool := testdb.Pool(t)
	var ids []int64
	for i := 0; i < 400; i++ {
		ids = append(ids, newUser(t, pool, fmt.Sprintf("split-%d", i), day))
	}
	held := 0
	for _, id := range ids {
		first := isHoldout(t, pool, id)
		if first != isHoldout(t, pool, id) {
			t.Fatalf("holdout of %d changed between two reads", id)
		}
		if first {
			held++
		}
	}
	if held < 15 || held > 70 { // 10% of 400 is 40; the bounds only catch a broken mask or salt
		t.Fatalf("holdout = %d of 400, want roughly 10%%", held)
	}

	q := db.New(pool)
	if _, err := q.EnrollCVLifecycleUsers(context.Background(), db.EnrollCVLifecycleUsersParams{WindowDays: 14, MaxRows: 1000}); err != nil {
		t.Fatal(err)
	}
	for _, id := range ids {
		if _, _, _, stored := enrollment(t, pool, id); stored != isHoldout(t, pool, id) {
			t.Fatalf("stored holdout of %d disagrees with the function", id)
		}
	}
}

func TestRun_OffsetsAreFromTheUploadAndAStepWhoseSuccessorIsDueIsDropped(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	r, sender := newRunner(t, q)

	cases := []struct {
		label string
		ago   time.Duration
		want  []int
	}{
		{"hours", 6 * time.Hour, nil},
		{"day1", day + time.Hour, []int{1}},
		{"day3", 3*day + time.Hour, []int{1}},
		{"day7", 7*day + time.Hour, []int{2}},
		{"day13", 13*day + time.Hour, []int{3}},
	}
	ids := map[string]int64{}
	for _, c := range cases {
		id, _ := mailable(t, pool, c.label, c.ago)
		ids[c.label] = id
	}
	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Enrolled < len(cases) {
		t.Fatalf("enrolled %d, want at least %d", stats.Enrolled, len(cases))
	}
	for _, c := range cases {
		got := sentSteps(t, pool, ids[c.label])
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: sent steps %v, want %v", c.label, got, c.want)
		}
	}
	if len(sender.sent) == 0 {
		t.Fatal("nothing reached the sender")
	}
}

func TestRun_ASecondRunSendsNothingMoreToday(t *testing.T) {
	pool := testdb.Pool(t)
	r, sender := newRunner(t, db.New(pool))
	id, _ := mailable(t, pool, "again", day+time.Hour)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	first := len(sender.sent)
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(sender.sent) != first {
		t.Fatalf("second run sent %d more", len(sender.sent)-first)
	}
	if got := sentSteps(t, pool, id); fmt.Sprint(got) != "[1]" {
		t.Fatalf("steps = %v, want [1]", got)
	}
}

func TestRun_HoldoutIsEnrolledAndNeverMailed(t *testing.T) {
	pool := testdb.Pool(t)
	r, _ := newRunner(t, db.New(pool))
	mailed, held := mailable(t, pool, "hold", 2*day)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sentSteps(t, pool, held); len(got) != 0 {
		t.Fatalf("holdout account got steps %v", got)
	}
	if status, _, _, h := enrollment(t, pool, held); status != "active" || !h {
		t.Fatalf("holdout enrollment = %s holdout=%v, want active holdout", status, h)
	}
	if got := sentSteps(t, pool, mailed); fmt.Sprint(got) != "[1]" {
		t.Fatalf("mailed account steps = %v, want [1]", got)
	}
}

func TestClaim_RechecksEligibilityInsideTheStatement(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	claim := func(id int64) error {
		_, err := q.ClaimCVLifecycleSend(ctx, db.ClaimCVLifecycleSendParams{UserID: id, Step: int16(StepATS), AfterDays: 1})
		return err
	}
	// Each account is enrolled and due; then one thing changes between list and claim.
	setup := func(label string) int64 {
		id, _ := mailable(t, pool, label, 2*day)
		exec(t, pool, `INSERT INTO cv_lifecycle_enrollments (user_id, uploaded_at, holdout)
			SELECT id, resume_uploaded_at, false FROM users WHERE id = $1`, id)
		return id
	}

	ok := setup("claim-ok")
	if err := claim(ok); err != nil {
		t.Fatalf("eligible claim: %v", err)
	}
	if err := claim(ok); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("second claim of the same step = %v, want ErrNoRows", err)
	}

	changes := map[string]string{
		"unsubscribed": `INSERT INTO notification_settings (user_id, news_email_enabled) VALUES ($1, false)`,
		"paid":         `UPDATE users SET pro_until_stripe = now() + interval '5 days' WHERE id = $1`,
		"reuploaded":   `UPDATE users SET resume_uploaded_at = now() WHERE id = $1`,
		"unverified":   `UPDATE users SET email_verified = false WHERE id = $1`,
		"holdout":      `UPDATE cv_lifecycle_enrollments SET holdout = true WHERE user_id = $1`,
		"exited":       `UPDATE cv_lifecycle_enrollments SET status = 'unsubscribed' WHERE user_id = $1`,
	}
	for name, sql := range changes {
		id := setup("claim-" + name)
		exec(t, pool, sql, id)
		if err := claim(id); !errors.Is(err, pgx.ErrNoRows) {
			t.Errorf("%s: claim = %v, want ErrNoRows", name, err)
		}
	}
}

func TestObserve_PaidForStripeAndRevenueCatEntitledForGrantedOnly(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()

	users := map[string]int64{}
	for _, label := range []string{"stripe", "revenuecat", "ultra", "granted", "none", "late"} {
		id := newUser(t, pool, "obs-"+label, 2*day)
		users[label] = id
	}
	if _, err := q.EnrollCVLifecycleUsers(ctx, db.EnrollCVLifecycleUsersParams{WindowDays: 14, MaxRows: 100}); err != nil {
		t.Fatal(err)
	}
	exec(t, pool, `UPDATE users SET pro_until_stripe = now() + interval '30 days' WHERE id = $1`, users["stripe"])
	exec(t, pool, `UPDATE users SET pro_until_revenuecat = now() + interval '30 days' WHERE id = $1`, users["revenuecat"])
	exec(t, pool, `UPDATE users SET ultra_until_stripe = now() + interval '30 days' WHERE id = $1`, users["ultra"])
	exec(t, pool, `UPDATE users SET pro_until_granted = now() + interval '30 days' WHERE id = $1`, users["granted"])
	// Paid, but past the observation window: not counted as a conversion of this cohort.
	exec(t, pool, `UPDATE cv_lifecycle_enrollments SET enrolled_at = now() - interval '100 days' WHERE user_id = $1`, users["late"])
	exec(t, pool, `UPDATE users SET pro_until_stripe = now() + interval '30 days' WHERE id = $1`, users["late"])

	if n, err := q.ObserveCVLifecyclePaid(ctx, 90); err != nil || n != 3 {
		t.Fatalf("paid observed = %d, %v; want 3", n, err)
	}
	if n, err := q.ObserveCVLifecycleEntitled(ctx, 90); err != nil || n != 1 {
		t.Fatalf("entitled observed = %d, %v; want 1", n, err)
	}

	for label, want := range map[string]struct {
		status string
		paid   bool
	}{
		"stripe": {"paid", true}, "revenuecat": {"paid", true}, "ultra": {"paid", true},
		"granted": {"entitled", false}, "none": {"active", false}, "late": {"active", false},
	} {
		status, paid, _, _ := enrollment(t, pool, users[label])
		if status != want.status || paid != want.paid {
			t.Errorf("%s: status=%s paid_at set=%v, want %s/%v", label, status, paid, want.status, want.paid)
		}
	}

	// A granted account that later pays moves on to paid; paid_at is set once.
	exec(t, pool, `UPDATE users SET pro_until_stripe = now() + interval '30 days' WHERE id = $1`, users["granted"])
	if _, err := q.ObserveCVLifecyclePaid(ctx, 90); err != nil {
		t.Fatal(err)
	}
	if status, paid, _, _ := enrollment(t, pool, users["granted"]); status != "paid" || !paid {
		t.Errorf("granted then paying: status=%s paid_at set=%v", status, paid)
	}
}

func TestObserve_HoldoutsAndUnsubscribedAreObservedToo(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	mailed, held := mailable(t, pool, "watch", 2*day)
	if _, err := q.EnrollCVLifecycleUsers(context.Background(), db.EnrollCVLifecycleUsersParams{WindowDays: 14, MaxRows: 100}); err != nil {
		t.Fatal(err)
	}
	exec(t, pool, `INSERT INTO notification_settings (user_id, news_email_enabled) VALUES ($1, false)`, mailed)
	if n, err := q.ExitCVLifecycleUnsubscribed(context.Background()); err != nil || n != 1 {
		t.Fatalf("unsubscribed = %d, %v; want 1", n, err)
	}
	exec(t, pool, `UPDATE users SET pro_until_revenuecat = now() + interval '30 days' WHERE id IN ($1, $2)`, mailed, held)
	if n, err := q.ObserveCVLifecyclePaid(context.Background(), 90); err != nil || n != 2 {
		t.Fatalf("paid observed = %d, %v; want 2", n, err)
	}
	for _, id := range []int64{mailed, held} {
		if status, paid, exited, _ := enrollment(t, pool, id); status != "paid" || !paid || !exited {
			t.Errorf("user %d: status=%s paid=%v exited=%v", id, status, paid, exited)
		}
	}
}

func TestRun_UnsubscribeEndsTheSequence(t *testing.T) {
	pool := testdb.Pool(t)
	r, sender := newRunner(t, db.New(pool))
	id, _ := mailable(t, pool, "unsub", 2*day)
	exec(t, pool, `INSERT INTO notification_settings (user_id, news_email_enabled) VALUES ($1, false)`, id)

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if stats.Unsubscribed != 1 || len(sentSteps(t, pool, id)) != 0 || sentTo(sender, id) {
		t.Fatalf("stats = %+v; want one unsubscribe and no letter to %d", stats, id)
	}
	if status, _, exited, _ := enrollment(t, pool, id); status != "unsubscribed" || !exited {
		t.Fatalf("status = %s exited = %v", status, exited)
	}
}

func TestDailyClaim_OneSenderPerAccountPerDayAcrossAllFour(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()
	sources := []string{maildaily.SourceOnboarding, maildaily.SourceLimitNudge, maildaily.SourceBroadcast, maildaily.SourceCVLifecycle}

	for i, first := range sources {
		id := newUser(t, pool, fmt.Sprintf("claim-%d", i), day)
		if _, err := q.ClaimMailDay(ctx, db.ClaimMailDayParams{UserID: id, Source: first}); err != nil {
			t.Fatalf("%s first claim: %v", first, err)
		}
		for _, other := range sources {
			if _, err := q.ClaimMailDay(ctx, db.ClaimMailDayParams{UserID: id, Source: other}); !errors.Is(err, pgx.ErrNoRows) {
				t.Errorf("%s claimed after %s held the day: %v", other, first, err)
			}
		}
	}
}

func TestDailyClaim_ReleaseIsTheTakersOwnAndFreesTheDay(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	ctx := context.Background()
	id := newUser(t, pool, "release", day)

	claimed, err := q.ClaimMailDay(ctx, db.ClaimMailDayParams{UserID: id, Source: maildaily.SourceBroadcast})
	if err != nil {
		t.Fatal(err)
	}
	if err := q.ReleaseMailDay(ctx, db.ReleaseMailDayParams{UserID: id, Day: claimed, Source: maildaily.SourceCVLifecycle}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimMailDay(ctx, db.ClaimMailDayParams{UserID: id, Source: maildaily.SourceCVLifecycle}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a foreign release freed the day: %v", err)
	}
	if err := q.ReleaseMailDay(ctx, db.ReleaseMailDayParams{UserID: id, Day: claimed, Source: maildaily.SourceBroadcast}); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ClaimMailDay(ctx, db.ClaimMailDayParams{UserID: id, Source: maildaily.SourceCVLifecycle}); err != nil {
		t.Fatalf("the taker's own release did not free the day: %v", err)
	}
}

func TestDailyClaim_AnotherSendersDayKeepsTheLetterPendingThenSendsTomorrow(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	r, sender := newRunner(t, q)
	id, _ := mailable(t, pool, "busy", 2*day)
	if _, err := q.ClaimMailDay(context.Background(), db.ClaimMailDayParams{UserID: id, Source: maildaily.SourceOnboarding}); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if sentTo(sender, id) || len(sentSteps(t, pool, id)) != 0 {
		t.Fatalf("a letter went out to %d on another sender's day", id)
	}

	exec(t, pool, `UPDATE mail_day_claims SET day = mail_today() - 1 WHERE user_id = $1`, id)
	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sentSteps(t, pool, id); fmt.Sprint(got) != "[1]" {
		t.Fatalf("steps after the day passed = %v, want [1]", got)
	}
}

func TestPrune_DropsOnlyOldClaims(t *testing.T) {
	pool := testdb.Pool(t)
	q := db.New(pool)
	id := newUser(t, pool, "prune", day)
	exec(t, pool, `INSERT INTO mail_day_claims (user_id, day, source) VALUES ($1, mail_today() - 30, 'onboarding'), ($1, mail_today(), 'onboarding')`, id)
	n, err := q.PruneMailDayClaims(context.Background())
	if err != nil || n != 1 {
		t.Fatalf("pruned = %d, %v; want 1", n, err)
	}
}
