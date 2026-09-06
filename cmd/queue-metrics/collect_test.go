package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
)

// fakeQueries answers each query from a canned value, so collect's assembly and error
// handling are testable without a database. A non-nil err field fails that one query.
type fakeQueries struct {
	search       db.SearchOutboxMetricsRow
	searchDelete db.SearchDeleteOutboxMetricsRow
	enrich       db.EnrichmentOutboxMetricsRow
	semantic     db.SemanticOutboxMetricsRow
	mail         db.MailClassificationOutboxMetricsRow
	applyForms   db.ApplyFormOutboxMetricsRow
	adzuna       db.AdzunaDescriptionOutboxMetricsRow
	autoApply    db.AutoApplyQueueMetricsRow
	pushTickets  db.PushTicketOutboxMetricsRow
	apple        db.AppleRevocationJobMetricsRow
	boards       db.BoardHealthMetricsRow
	newest       pgtype.Timestamptz
	health       []db.ProviderIngestHealthRow
	notify       db.NotifyBacklogMetricsRow
	newestErr    error
	searchErr    error
}

func (f fakeQueries) NotifyBacklogMetrics(context.Context) (db.NotifyBacklogMetricsRow, error) {
	return f.notify, nil
}

func (f fakeQueries) SearchOutboxMetrics(context.Context) (db.SearchOutboxMetricsRow, error) {
	return f.search, f.searchErr
}

func (f fakeQueries) SearchDeleteOutboxMetrics(context.Context) (db.SearchDeleteOutboxMetricsRow, error) {
	return f.searchDelete, nil
}

func (f fakeQueries) ApplyFormOutboxMetrics(context.Context) (db.ApplyFormOutboxMetricsRow, error) {
	return f.applyForms, nil
}

func (f fakeQueries) AdzunaDescriptionOutboxMetrics(context.Context) (db.AdzunaDescriptionOutboxMetricsRow, error) {
	return f.adzuna, nil
}

func (f fakeQueries) AutoApplyQueueMetrics(context.Context) (db.AutoApplyQueueMetricsRow, error) {
	return f.autoApply, nil
}

func (f fakeQueries) PushTicketOutboxMetrics(context.Context) (db.PushTicketOutboxMetricsRow, error) {
	return f.pushTickets, nil
}

func (f fakeQueries) EnrichmentOutboxMetrics(context.Context) (db.EnrichmentOutboxMetricsRow, error) {
	return f.enrich, nil
}

func (f fakeQueries) SemanticOutboxMetrics(context.Context) (db.SemanticOutboxMetricsRow, error) {
	return f.semantic, nil
}

func (f fakeQueries) MailClassificationOutboxMetrics(context.Context) (db.MailClassificationOutboxMetricsRow, error) {
	return f.mail, nil
}

func (f fakeQueries) AppleRevocationJobMetrics(context.Context) (db.AppleRevocationJobMetricsRow, error) {
	return f.apple, nil
}

func (f fakeQueries) BoardHealthMetrics(context.Context) (db.BoardHealthMetricsRow, error) {
	return f.boards, nil
}

func (f fakeQueries) NewestOpenJobCreatedAt(context.Context) (pgtype.Timestamptz, error) {
	return f.newest, f.newestErr
}

func (f fakeQueries) ProviderIngestHealth(context.Context) ([]db.ProviderIngestHealthRow, error) {
	return f.health, nil
}

func populatedQueries() fakeQueries {
	return fakeQueries{
		search: db.SearchOutboxMetricsRow{Depth: 3, DeadLetters: 2, OldestAgeSeconds: 21600.5},
		// A removal this queue gave up on leaves a closed posting live in the facet index,
		// and its UNIQUE (job_id) then blocks that posting from ever being re-queued.
		searchDelete: db.SearchDeleteOutboxMetricsRow{Depth: 7, DeadLetters: 5, OldestAgeSeconds: 1800},
		enrich:       db.EnrichmentOutboxMetricsRow{Depth: 1049297, DeadLetters: 41, OldestAgeSeconds: 5529600},
		semantic:     db.SemanticOutboxMetricsRow{Depth: 0, DeadLetters: 0, OldestAgeSeconds: 0},
		// The shape prod was actually in: nothing live and every entry dead-lettered, which
		// the worker's own log reported as "done failed=0 dead-lettered=0" for five weeks.
		mail: db.MailClassificationOutboxMetricsRow{Depth: 0, DeadLetters: 2726, OldestAgeSeconds: 0},
		// The queue cmd/apple-revoke drains. A `failed` job is never claimed again and
		// leaves its identity stuck in revocation_pending, so its count is the one that
		// needs watching.
		applyForms: db.ApplyFormOutboxMetricsRow{Depth: 185432, DeadLetters: 13, OldestAgeSeconds: 604800},
		adzuna:     db.AdzunaDescriptionOutboxMetricsRow{Depth: 611, DeadLetters: 3, OldestAgeSeconds: 7200},
		// The one queue with a third state: parked attempts are neither owed work nor
		// work retried to exhaustion, so they need their own count.
		autoApply: db.AutoApplyQueueMetricsRow{Depth: 6, DeadLetters: 1, Blocked: 22, OldestAgeSeconds: 300},
		// No attempts and no failed_at column, so no dead-letter measurement exists.
		pushTickets: db.PushTicketOutboxMetricsRow{Depth: 48, OldestAgeSeconds: 1200},
		apple:       db.AppleRevocationJobMetricsRow{Depth: 4, DeadLetters: 9, OldestAgeSeconds: 900},
		boards:      db.BoardHealthMetricsRow{Healthy: 74894, Failing: 7002, Cooled: 1882},
		notify:      db.NotifyBacklogMetricsRow{PendingSubscriptions: 12, OldestAgeSeconds: 184.25},
		newest:      pgtype.Timestamptz{Time: time.Unix(1786821346, 0), Valid: true},
		health: []db.ProviderIngestHealthRow{
			{
				Provider:      "greenhouse",
				LastSuccessAt: pgtype.Timestamptz{Time: time.Unix(1786821000, 0), Valid: true},
				Healthy:       9312, Failing: 604, Cooled: 71,
			},
			// The shape that started this: no success ever, so no timestamp — and one
			// failing board, which is the only thing left that can name it.
			{Provider: "gulftalent", LastSuccessAt: pgtype.Timestamptz{}, Failing: 1},
		},
	}
}

// A provider whose boards have never succeeded answers with a NULL max(), and that NULL
// must survive collection as "no measurement" rather than collapsing to the zero instant
// — render turns the two into an absent sample and a 1970 timestamp respectively, and
// only one of those is honest.
func TestCollectKeepsNeverSucceededProviderDistinctFromZero(t *testing.T) {
	snap, err := collect(context.Background(), populatedQueries())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	if len(snap.providers) != 2 {
		t.Fatalf("providers = %d, want 2", len(snap.providers))
	}
	if got := snap.providers[0]; got.name != "greenhouse" || !got.lastSuccess.Equal(time.Unix(1786821000, 0)) {
		t.Errorf("first provider = %+v, want greenhouse at 1786821000", got)
	}
	if got := snap.providers[1]; got.name != "gulftalent" || !got.lastSuccess.IsZero() {
		t.Errorf("second provider = %+v, want gulftalent with no measurement", got)
	}
	// The board counts are what that provider is left with once the timestamp drops out,
	// so they must arrive alongside the NULL rather than be lost with it.
	if got := snap.providers[1]; got.failing != 1 || got.healthy != 0 || got.cooled != 0 {
		t.Errorf("gulftalent boards = %d healthy/%d failing/%d cooled, want 0/1/0",
			got.healthy, got.failing, got.cooled)
	}
	if got := snap.providers[0]; got.healthy != 9312 || got.failing != 604 || got.cooled != 71 {
		t.Errorf("greenhouse boards = %d/%d/%d, want 9312/604/71", got.healthy, got.failing, got.cooled)
	}
}

func TestCollectAssemblesEveryQueueInOrder(t *testing.T) {
	got, err := collect(context.Background(), populatedQueries())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}

	want := []queueMetrics{
		{name: "search_outbox", depth: 3, deadLetters: ptr(int64(2)), oldestAgeSeconds: 21600.5},
		{name: "search_delete_outbox", depth: 7, deadLetters: ptr(int64(5)), oldestAgeSeconds: 1800},
		{name: "enrichment_outbox", depth: 1049297, deadLetters: ptr(int64(41)), oldestAgeSeconds: 5529600},
		{name: "semantic_outbox", depth: 0, deadLetters: ptr(int64(0)), oldestAgeSeconds: 0},
		{name: "email_classification_outbox", depth: 0, deadLetters: ptr(int64(2726)), oldestAgeSeconds: 0},
		{name: "apply_form_outbox", depth: 185432, deadLetters: ptr(int64(13)), oldestAgeSeconds: 604800},
		{name: "adzuna_description_outbox", depth: 611, deadLetters: ptr(int64(3)), oldestAgeSeconds: 7200},
		{name: "auto_apply_queue", depth: 6, deadLetters: ptr(int64(1)), blocked: ptr(int64(22)), oldestAgeSeconds: 300},
		{name: "push_ticket_outbox", depth: 48, oldestAgeSeconds: 1200},
		{name: "apple_revocation_jobs", depth: 4, deadLetters: ptr(int64(9)), oldestAgeSeconds: 900},
	}
	if len(got.queues) != len(want) {
		t.Fatalf("collected %d queues, want %d", len(got.queues), len(want))
	}
	for i := range want {
		if !sameQueue(got.queues[i], want[i]) {
			t.Errorf("queue %d = %s, want %s", i, showQueue(got.queues[i]), showQueue(want[i]))
		}
	}
	if got.healthyBoards != 74894 || got.failingBoards != 7002 || got.cooledBoards != 1882 {
		t.Errorf("boards = %d/%d/%d, want 74894/7002/1882", got.healthyBoards, got.failingBoards, got.cooledBoards)
	}
	if !got.newestJob.Equal(time.Unix(1786821346, 0)) {
		t.Errorf("newestJob = %v, want %v", got.newestJob, time.Unix(1786821346, 0))
	}
}

func TestCollectTreatsAnEmptyCatalogueAsAbsentNotAsAFailure(t *testing.T) {
	q := populatedQueries()
	q.newest = pgtype.Timestamptz{}
	q.newestErr = pgx.ErrNoRows

	got, err := collect(context.Background(), q)

	// An empty catalogue is a fresh-install state, not an incident: the run must
	// still publish every other family rather than failing outright.
	if err != nil {
		t.Fatalf("collect on an empty catalogue: %v", err)
	}
	if !got.newestJob.IsZero() {
		t.Errorf("newestJob = %v, want the zero time so render omits the sample", got.newestJob)
	}
	if len(got.queues) != 10 {
		t.Errorf("collected %d queues, want all 10 despite the empty catalogue", len(got.queues))
	}
}

func TestCollectPropagatesAQueryFailure(t *testing.T) {
	q := populatedQueries()
	q.searchErr = errors.New("connection reset")

	_, err := collect(context.Background(), q)

	if err == nil {
		t.Fatal("collect succeeded, want the query error propagated so the run exits non-zero")
	}
	if !errors.Is(err, q.searchErr) {
		t.Errorf("collect error = %v, want it to wrap the underlying query error", err)
	}
}

// ptr addresses a literal so a test can express "this queue HAS that state, and the
// count is zero" separately from "this queue has no such state at all" — the
// distinction queueMetrics' optional counts exist to carry.
func ptr[T any](v T) *T { return &v }

// sameQueue compares two measurements by value rather than by pointer identity, which is
// what struct equality would compare now that two counts are optional.
func sameQueue(a, b queueMetrics) bool {
	return a.name == b.name && a.depth == b.depth &&
		sameCount(a.deadLetters, b.deadLetters) && sameCount(a.blocked, b.blocked) &&
		a.oldestAgeSeconds == b.oldestAgeSeconds
}

func sameCount(a, b *int64) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

func showQueue(q queueMetrics) string {
	return fmt.Sprintf("{name:%s depth:%d dead:%s blocked:%s age:%v}",
		q.name, q.depth, showCount(q.deadLetters), showCount(q.blocked), q.oldestAgeSeconds)
}

func showCount(n *int64) string {
	if n == nil {
		return "none"
	}
	return strconv.FormatInt(*n, 10)
}

// push_ticket_outbox carries neither attempts nor failed_at, so there is no give-up state
// to measure. It must arrive as an ABSENT count rather than a zero: a zero would publish
// "this queue has given up on nothing", which is a claim about a measurement nobody took,
// and would sit in the dead-letter family looking permanently healthy.
func TestCollectLeavesAQueueWithNoGiveUpStateUnmeasuredForDeadLetters(t *testing.T) {
	got, err := collect(context.Background(), populatedQueries())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	push, ok := queueByName(got.queues, "push_ticket_outbox")
	if !ok {
		t.Fatal("push_ticket_outbox was not measured at all")
	}
	if push.deadLetters != nil {
		t.Errorf("push_ticket_outbox dead letters = %d, want no measurement", *push.deadLetters)
	}
	if push.depth != 48 || push.oldestAgeSeconds != 1200 {
		t.Errorf("push_ticket_outbox = depth %d age %v, want 48/1200 — the age is its whole signal",
			push.depth, push.oldestAgeSeconds)
	}
}

// A parked auto-apply attempt needs new data, not another try, so it is neither depth nor
// a dead letter. Folding it into either would misreport a population no run will ever
// claim again as either owed work or exhausted retries.
func TestCollectKeepsParkedAutoApplyAttemptsOutOfDepthAndDeadLetters(t *testing.T) {
	got, err := collect(context.Background(), populatedQueries())
	if err != nil {
		t.Fatalf("collect: %v", err)
	}
	q, ok := queueByName(got.queues, "auto_apply_queue")
	if !ok {
		t.Fatal("auto_apply_queue was not measured at all")
	}
	if q.blocked == nil {
		t.Fatal("auto_apply_queue published no parked count, so parked attempts are invisible")
	}
	if *q.blocked != 22 || q.depth != 6 || q.deadLetters == nil || *q.deadLetters != 1 {
		t.Errorf("auto_apply_queue = depth %d dead %s blocked %d, want 6/1/22 as three distinct states",
			q.depth, showCount(q.deadLetters), *q.blocked)
	}
}

func queueByName(queues []queueMetrics, name string) (queueMetrics, bool) {
	for _, q := range queues {
		if q.name == name {
			return q, true
		}
	}
	return queueMetrics{}, false
}
