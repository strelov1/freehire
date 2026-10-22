package notify

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fullClaim builds a claim of n matches for subscription 1, the newest first, each
// with a job row, and the cap that makes it a FULL claim.
func fullClaim(n int, base time.Time) ([]db.ClaimSubscriptionMatchesRow, map[int64]db.GetJobsForDigestRow) {
	claimed := make([]db.ClaimSubscriptionMatchesRow, 0, n)
	jobs := make(map[int64]db.GetJobsForDigestRow, n)
	for i := range n {
		id := int64(100 + i)
		claimed = append(claimed, db.ClaimSubscriptionMatchesRow{
			SubscriptionID: 1,
			JobID:          id,
			MatchedAt:      ts(base.Add(-time.Duration(i) * time.Minute)),
		})
		jobs[id] = db.GetJobsForDigestRow{ID: id, Title: "Job", PublicSlug: "job"}
	}
	return claimed, jobs
}

// The bug: a `daily` subscription matching more than one digest holds sent the
// oldest SnapshotCap matches every morning and re-queued the rest, so it fell
// further behind every day (subscription 256, 2026-10-22, was being mailed
// matches from three weeks earlier). Now what does not fit is COUNTED by the
// digest and retired with it.
func TestDeliver_FullClaimCountsAndRetiresWhatDidNotFit(t *testing.T) {
	base := time.Date(2026, 10, 22, 9, 0, 0, 0, time.UTC)
	claimed, jobs := fullClaim(3, base)
	// The claim's rows are not ordered (UPDATE ... RETURNING promises no order):
	// the cutoff must be computed, not read off the last row.
	claimed[0], claimed[2] = claimed[2], claimed[0]
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs, coveredCount: 597}
	notifier := &fakeNotifier{}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 3
	r := New(store, &fakeSearcher{}, notifier, cfg)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	d := notifier.sent[0]
	if d.Total != 600 || len(d.Jobs) != 3 {
		t.Fatalf("digest Total=%d Jobs=%d, want 600/3 — it carries the newest 3 and counts the rest", d.Total, len(d.Jobs))
	}
	if !d.Overflowed() {
		t.Error("Overflowed() = false for a digest that counts more than it carries")
	}
	if len(store.released) != 0 {
		t.Errorf("released = %+v, want nothing re-queued", store.released)
	}
	wantCutoff := db.CountCoveredMatchesParams{
		SubscriptionID:  1,
		BeforeMatchedAt: ts(base.Add(-2 * time.Minute)),
		BeforeJobID:     102,
	}
	if len(store.coveredCounts) != 1 || store.coveredCounts[0] != wantCutoff {
		t.Fatalf("CountCoveredMatches = %+v, want %+v (the OLDEST claimed row)", store.coveredCounts, wantCutoff)
	}
	if len(store.coveredMarked) != 1 || db.CountCoveredMatchesParams(store.coveredMarked[0]) != wantCutoff {
		t.Fatalf("MarkCoveredMatchesNotified = %+v, want the same cutoff the count used", store.coveredMarked)
	}
}

// The notification's own page lists only the recorded snapshot, under the
// recorded body. For an overflowed digest that body must say the list is the
// newest part of the count, or the page reads as 600 promised and 3 shown.
func TestDeliver_OverflowedDigestRecordsWhatItsPageLists(t *testing.T) {
	claimed, jobs := fullClaim(3, time.Now())
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs, coveredCount: 597}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 3
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, cfg)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got, want := store.recordedNotifications[0].Body, "600 new jobs — the newest 3 are listed"; got != want {
		t.Errorf("recorded body = %q, want %q", got, want)
	}
}

// A claim that came back short has nothing behind it to cover — no count, no mark.
func TestDeliver_ShortClaimCoversNothing(t *testing.T) {
	claimed, jobs := fullClaim(2, time.Now())
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs, coveredCount: 50}
	notifier := &fakeNotifier{}
	r := New(store, &fakeSearcher{}, notifier, DefaultConfig())

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := notifier.sent[0].Total; got != 2 {
		t.Errorf("Total = %d, want 2", got)
	}
	if len(store.coveredCounts) != 0 || len(store.coveredMarked) != 0 {
		t.Errorf("count=%v mark=%v, want neither for a short claim", store.coveredCounts, store.coveredMarked)
	}
}

// A digest that did not go out told nobody about the covered matches either, so
// they stay pending for the retry.
func TestDeliver_FailedSendRetiresNothingItCovered(t *testing.T) {
	claimed, jobs := fullClaim(3, time.Now())
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs, coveredCount: 10}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 3
	r := New(store, &fakeSearcher{}, &fakeNotifier{err: errors.New("telegram down")}, cfg)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if len(store.coveredMarked) != 0 {
		t.Errorf("covered marked = %+v, want none after a failed send", store.coveredMarked)
	}
}

// If the count cannot be read, the digest announces only what it carries and
// retires nothing it did not count — the two numbers must never disagree.
func TestDeliver_UncountableCoverAnnouncesOnlyWhatItCarries(t *testing.T) {
	claimed, jobs := fullClaim(3, time.Now())
	store := &fakeStore{claimed: claimed, delivery: deliverySubscription(), digestJobs: jobs, coveredErr: errors.New("timeout")}
	notifier := &fakeNotifier{}
	cfg := DefaultConfig()
	cfg.SnapshotCap = 3
	r := New(store, &fakeSearcher{}, notifier, cfg)

	if _, err := r.Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := notifier.sent[0].Total; got != 3 {
		t.Errorf("Total = %d, want 3", got)
	}
	if len(store.coveredMarked) != 0 {
		t.Errorf("covered marked = %+v, want none when the count failed", store.coveredMarked)
	}
}

// Stale matches leave the pending set BEFORE the claim reads it, in bounded
// statements repeated until one comes back short.
func TestDeliver_ExpiresStaleMatchesInBatchesBeforeClaiming(t *testing.T) {
	store := &fakeStore{expireResults: []int64{expireBatch, expireBatch, 7}}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	stats, err := r.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"expire", "expire", "expire", "claim"}; !reflect.DeepEqual(store.calls, want) {
		t.Errorf("calls = %v, want %v", store.calls, want)
	}
	if want := 2*expireBatch + 7; stats.Expired != want {
		t.Errorf("Expired = %d, want %d", stats.Expired, want)
	}
	want := db.ExpireStaleSubscriptionMatchesParams{
		LeaseSeconds:     DefaultConfig().LeaseSeconds,
		FreshnessSeconds: int32((72 * time.Hour).Seconds()),
		BatchSize:        expireBatch,
	}
	if store.expireCalls[0] != want {
		t.Errorf("expire params = %+v, want %+v", store.expireCalls[0], want)
	}
}

// Delivering without expiring first would hand stale postings to a digest, so a
// failed expiry stops the delivery stage rather than being stepped over.
func TestDeliver_ExpiryFailureStopsDelivery(t *testing.T) {
	store := &fakeStore{expireErr: errors.New("connection reset")}
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, DefaultConfig())

	if _, err := r.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded, want the expiry failure")
	}
	if want := []string{"expire"}; !reflect.DeepEqual(store.calls, want) {
		t.Errorf("calls = %v, want %v — nothing may be claimed", store.calls, want)
	}
}

// A window under a second becomes a zero-second interval in SQL, and expiry is
// terminal: every pending match would go stale before it could be claimed. A
// misconfigured window fails the delivery stage instead.
func TestDeliver_SubSecondFreshnessWindowRefusesToExpire(t *testing.T) {
	store := &fakeStore{}
	cfg := DefaultConfig()
	cfg.FreshnessWindow = 0
	r := New(store, &fakeSearcher{}, &fakeNotifier{}, cfg)

	if _, err := r.Run(context.Background()); err == nil {
		t.Fatal("Run succeeded, want a refusal")
	}
	if len(store.calls) != 0 {
		t.Errorf("calls = %v, want nothing expired or claimed", store.calls)
	}
}

func TestDigestSearchPath(t *testing.T) {
	cases := []struct{ query, want string }{
		{"", "/jobs?utm_source=email"},
		{"seniority=senior&skills=go,rust", "/jobs?seniority=senior&skills=go,rust&utm_source=email"},
	}
	for _, c := range cases {
		if got := (Digest{SearchQuery: c.query}).SearchPath("email"); got != c.want {
			t.Errorf("SearchPath(%q) = %q, want %q", c.query, got, c.want)
		}
	}
}

// A digest counting more than it carries still records the jobs it carries.
func TestDigestJobsSnapshot_OverflowedDigestRecordsWhatItCarries(t *testing.T) {
	d := digestOf(2)
	d.Total = 500
	var jobs []SnapshotJob
	if err := json.Unmarshal(digestJobsSnapshot(d), &jobs); err != nil {
		t.Fatal(err)
	}
	if len(jobs) != 2 {
		t.Errorf("snapshot carries %d jobs, want 2", len(jobs))
	}

	one := digestOf(1)
	one.Total = 500
	if raw := digestJobsSnapshot(one); raw == nil {
		t.Error("a one-job digest that COUNTS more must still record its job list — it is not a single-job notification")
	}
}
