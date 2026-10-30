# Buddy Mock Interviews Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Let two interview-buddy members book a 60-minute peer mock interview with each other from published availability, with database-enforced non-overlap, cancellation (including by leave and block), 24h/1h reminders, handle-only notifications and a two-way blind review.

**Architecture:** The pure slot engine moves out of `internal/engage/mentorship` into a new `internal/engage/slots` package that both mentorship and buddy import. `internal/engage/buddy` gains a `MockService` (settings, availability, slots, booking, cancellation, review, reminders) over a `MockRepository` implemented on the existing `QueriesRepository`, plus a `MockMailNotifier`. Postgres owns the hard guarantees: a per-person `EXCLUDE` on `buddy_mock_participants`, a single-statement booking guard, the leave cancellation inside `LeaveBuddyMember`, and a trigger on `user_blocks` (the messaging core never imports a feature, so the buddy migration reacts to its block). HTTP lives in `internal/api/handler/buddy_mocks.go`; the reminder pass runs inside `cmd/mentorship-remind`; the web reuses the mentor schedule editor and slot picker with their API calls passed as props.

**Tech Stack:** Go 1.26, Fiber v2, PostgreSQL (btree_gist), sqlc (pgx/v5), testcontainers via `internal/platform/testdb`, SvelteKit 3 / Svelte 5 runes, vitest + @testing-library/svelte.

## Global Constraints

- Mocks are **60 minutes**, **no buffers**, **12 hours** minimum notice, **30-day** horizon. Constants in `internal/engage/buddy`, never columns.
- Topics are exactly **`coding`**, **`system_design`**, **`behavioral`**.
- A member holds **at most 3 future confirmed mocks as guest**.
- Reviews: rating **1-5**, comment **≤ 1000** characters, written once, from the mock's end until **7 days** after it; revealed when the counterpart has reviewed too or the **7-day** window has closed.
- The meeting link must be **`https://`-only**, and reaches only the two participants of a confirmed mock.
- Migration: the proposal and design name **`0210_buddy_mocks.sql`**, but `migrations/0210_onboarding_buddies_step.sql` already exists and the newest file on `origin/main` is `0213_source_stats_quality.sql`. Use **`migrations/0214_buddy_mocks.sql`**; before writing it, run `git fetch origin main && git ls-tree --name-only origin/main migrations/ | tail -3` and take the next free number if `0214` has been taken meanwhile (rename every `0214` reference in this plan with it).
- **English only** in code, comments, identifiers, docs and commits.
- Before every commit that touches Go: `gofmt -l .` prints nothing (run `gofmt -w` on the touched paths), then `go vet ./...` and `go test ./...` pass. Before every push: `go vet -tags=integration ./...`.
- After any edit under `internal/platform/db/queries/`: `make sqlc`, and commit the regenerated `internal/platform/db/*.go` with it.
- A new Go package goes into `internal/platform/arch/layering/blocks.go` in the same commit.
- Run `/simplify` on the changed code right before each code commit (not for docs- or test-only commits).
- Integration tests need Docker (testcontainers). Every `go test -tags=integration` command below assumes it is running.

## File map

| Path | Task | Responsibility |
|---|---|---|
| `internal/engage/slots/*.go` | 1 | The pure engine (moved): rules, expansion, overrides, subtraction, slicing, `Slots`, `Calendar` |
| `internal/platform/arch/layering/blocks.go` | 1 | `slots` in the engage block |
| `migrations/0214_buddy_mocks.sql` | 2 | Six tables, the participants `EXCLUDE`, the block trigger |
| `internal/platform/db/queries/buddy_mocks.sql` | 2 | Every mock query |
| `internal/platform/db/queries/buddies.sql` | 2 | `LeaveBuddyMember` also cancels the member's future mocks |
| `internal/platform/db/buddy_mocks_integration_test.go` | 2 | Schema guarantees |
| `internal/engage/buddy/mock.go` | 3, 5, 7 | Constants, sentinels, `MockSettings`, `Mock`, `MockEntry`, pure helpers |
| `internal/engage/buddy/mock_service.go` | 3-7, 9 | `MockService` use cases, `MockRepository`, `MockNotifier` |
| `internal/engage/buddy/mock_repository.go` | 3-7, 9 | `MockRepository` on `*QueriesRepository` |
| `internal/engage/buddy/mock_notify.go` | 8 | `MockMailNotifier` (email + notification center) |
| `internal/engage/buddy/mock_test.go`, `mock_notify_test.go` | 3, 7, 8 | Unit tests |
| `internal/engage/buddy/mock_*_integration_test.go` | 3-9 | Service tests against Postgres |
| `cmd/mentorship-remind/main.go` | 9 | Runs the buddy reminder pass beside the mentor one |
| `internal/api/handler/buddy_mocks.go` (+ `buddies.go`, `conversations.go`, `mentorship.go`, `handler.go`) | 10 | Routes, error mapping, composition |
| `web/src/lib/components/MentorScheduleEditor.svelte`, `MentorBookingView.svelte`, `MentorBookingForm.svelte` | 11 | Shared editor/picker, API as props |
| `web/src/lib/buddyMocks.ts`, `api.ts`, `types.ts`, `components/BuddyMock*.svelte`, `routes/my/buddies/mocks/+page.svelte` | 12 | The buddy surfaces |
| `internal/engage/buddy/AGENTS.md`, `internal/engage/mentorship/AGENTS.md`, `internal/engage/AGENTS.md`, `AGENTS.md` | 13 | Docs |

## 1. Extract the slot engine into internal/engage/slots

**Files:**
- Move (git mv) from `internal/engage/mentorship/` to `internal/engage/slots/`: `slots.go`, `slots_test.go`, `slice.go`, `slice_test.go`, `override.go`, `override_test.go`, `expand.go`, `expand_test.go`, `subtract.go`, `subtract_test.go`, `schedule.go`, `schedule_test.go`, `calendar.go`, `calendar_test.go`, `anchor_test.go`, `determinism_test.go`, `midnight_test.go`, `repeated_hour_test.go`.
  `calendar.go` moves too although the proposal lists six files: `Calendar` calls the engine's unexported `expandSchedule`, `subtractBusy`, `sliceSlots`, `mergeOverlapping`, `compareIntervals` and `clip`, and is just as pure.
- Create: `internal/engage/slots/doc.go`, `internal/engage/mentorship/engine_helpers_test.go`
- Modify: `internal/platform/arch/layering/blocks.go:244`; every remaining `internal/engage/mentorship/*.go` that names an engine type (`booking.go`, `booking_repository.go`, `profile.go`, `repository.go`, `schedule_service.go`, `service.go`, `slot_cache.go:89-113`, `booking_test.go`, `fake_repo_test.go`, `profile_test.go`, `remind_test.go`, `service_calendar_test.go`, `slot_cache_test.go`, `googlemeet*.go`, `invite_test.go`); `internal/api/handler/mentorship.go`, `mentorship_availability.go`, `mentorship_write.go`, `mentorship_moderation_photo_integration_test.go`.

**Interfaces:**
- Produces (package `slots`, unchanged apart from the renames): `type Rule`, `func NewWeeklyRule(time.Weekday, TimeOfDay, TimeOfDay) (Rule, error)`, `func NewDatedRule(Date, TimeOfDay, TimeOfDay) (Rule, error)`, `func (Rule) WithID(int64) Rule` (was unexported `withID`), `func (Rule) ID() int64`, `type TimeOfDay`, `func NewTimeOfDay(int, int) (TimeOfDay, error)`, `type Date`, `func NewDate(int, time.Month, int) (Date, error)`, `type Interval struct{ Start, End time.Time }`, `type SessionParams struct{ Duration, BufferBefore, BufferAfter, MinimumNotice, Horizon time.Duration }`, `type SlotRequest struct{ Rules []Rule; OwnerZone *time.Location; Params SessionParams; Busy []Interval; From, To, Now time.Time; ViewerZone string }`, `type SlotResult struct{ Slots []Interval; Zone string }`, `func Slots(SlotRequest) (SlotResult, error)`, `type CalendarRequest` (field `OwnerZone`), `func Calendar(CalendarRequest) (CalendarResult, error)`, `StatusBooked/StatusBusy/StatusFree/StatusClosed`, sentinels `ErrNoOwnerZone`, `ErrInvalidTimeOfDay`, `ErrInvalidDate`, `ErrInvalidWeekday`, `ErrInvalidSpan`, `ErrInvalidSessionParams` (messages now prefixed `slots:` instead of `mentorship:`; no test or client reads the text).
- Consumes: nothing new. Mentorship behaviour does not change.

- [x] 1.1 Move the engine and its tests with history, and rename the package and the mentor-flavoured identifiers:

```bash
cd /Users/i_strelov/Projects/hire-wt-mocks
mkdir -p internal/engage/slots
for f in slots slice override expand subtract schedule calendar; do
  git mv internal/engage/mentorship/$f.go internal/engage/slots/$f.go
  git mv internal/engage/mentorship/${f}_test.go internal/engage/slots/${f}_test.go
done
for f in anchor determinism midnight repeated_hour; do
  git mv internal/engage/mentorship/${f}_test.go internal/engage/slots/${f}_test.go
done
perl -pi -e 's/^package mentorship$/package slots/; s/\bErrNoMentorZone\b/ErrNoOwnerZone/g; s/\bMentorZone\b/OwnerZone/g; s/"mentorship: /"slots: /g; s/the mentor has no timezone/the owner has no timezone/; s/\bwithID\b/WithID/g' internal/engage/slots/*.go
```

- [x] 1.2 Fix the two doc comments the rename made wrong. In `internal/engage/slots/slots.go` replace

```go
// ErrNoOwnerZone → 500. A mentor row reached the engine without the IANA zone that
```
with
```go
// ErrNoOwnerZone → 500. An owner's row reached the engine without the IANA zone that
```
and in `internal/engage/slots/schedule.go` replace
```go
// WithID labels a rule with the row it came from. Unexported: an id is something the
// repository knows and a caller cannot invent.
```
with
```go
// WithID labels a rule with the row it came from. Only a repository calls it: an id is
// something storage knows and a caller cannot invent.
```

- [x] 1.3 Create `internal/engage/slots/doc.go`:

```go
// Package slots is the availability engine shared by mentorship sessions and buddy mock
// interviews: an owner's weekly rules and dated overrides, stated in wall-clock time in
// their own IANA zone, expanded per date, with busy time subtracted, cut into sessions
// and filtered by the notice period and the horizon.
//
// It is a pure function of its inputs — no database, no clock, no owner id — which is what
// makes the daylight-saving and boundary cases testable without Postgres, and what lets a
// listed window and the re-derivation a booking runs agree. It moved here unchanged from
// internal/engage/mentorship, whose AGENTS.md still states the rules a change here keeps.
package slots
```

- [x] 1.4 Run the moved tests: `go test ./internal/engage/slots/ -count=1`. Expected: `ok`. Run the layering guard: `go test ./internal/platform/arch/layering/ -count=1`. Expected: FAIL, naming `internal/engage/slots` as a package in no block.

- [x] 1.5 Add `slots` to the engage block. In `internal/platform/arch/layering/blocks.go` replace line 244

```go
		"referral", "reminder", "report", "socialdigest", "subscription",
```
with
```go
		"referral", "reminder", "report",
		// slots is the pure availability engine (rules, expansion, slicing) that mentorship
		// and buddy mock interviews both compute with. It imports nothing but the standard
		// library; it sits in engage because both of its callers do.
		"slots",
		"socialdigest", "subscription",
```
Run `go test ./internal/platform/arch/layering/ -count=1`. Expected: `ok`.

- [x] 1.6 Run `go build ./internal/engage/mentorship/`. Expected: FAIL with `undefined: Rule`, `undefined: Interval`, `undefined: SlotRequest`, … . Rename the local variable that would shadow the new package import: in `internal/engage/mentorship/slot_cache.go` replace

```go
	slots := make([]Interval, 0, len(c.Slots))
	for i := range c.Slots {
		slots = append(slots, Interval{Start: c.Slots[i].In(zone), End: c.Ends[i].In(zone)})
	}
	return SlotResult{Slots: slots, Zone: c.Zone}, true
```
with
```go
	intervals := make([]Interval, 0, len(c.Slots))
	for i := range c.Slots {
		intervals = append(intervals, Interval{Start: c.Slots[i].In(zone), End: c.Ends[i].In(zone)})
	}
	return SlotResult{Slots: intervals, Zone: c.Zone}, true
```

- [x] 1.7 Qualify every engine name left in mentorship (comment lines are skipped; the only string literal touched is a `t.Fatalf("NewWeeklyRule: …")` label, which then reads `slots.NewWeeklyRule`) and in the mentorship handlers, then let goimports add the import:

```bash
cd /Users/i_strelov/Projects/hire-wt-mocks
perl -pi -e 'next if /^\s*\/\//;
  s/(?<![\w.])(SlotRequest|SlotResult|CalendarRequest|CalendarResult|CalendarInterval|CalendarStatus|SessionParams|TimeOfDay|Interval|Rule|Date|NewTimeOfDay|NewDate|NewWeeklyRule|NewDatedRule|ErrInvalidTimeOfDay|ErrInvalidDate|ErrInvalidWeekday|ErrInvalidSpan|ErrInvalidSessionParams|StatusBooked|StatusBusy|StatusFree|StatusClosed)(?!\w)/slots.$1/g;
  s/(?<![\w.])(Slots|Calendar)\(/slots.$1(/g;
  s/\bErrNoMentorZone\b/slots.ErrNoOwnerZone/g;
  s/\bMentorZone:/OwnerZone:/g;
  s/\.withID\(/.WithID(/g;' internal/engage/mentorship/*.go
perl -pi -e 's/\bmentorship\.(SlotRequest|SlotResult|CalendarRequest|CalendarResult|CalendarInterval|CalendarStatus|SessionParams|TimeOfDay|Interval|Rule|Date|NewTimeOfDay|NewDate|NewWeeklyRule|NewDatedRule|ErrInvalidTimeOfDay|ErrInvalidDate|ErrInvalidWeekday|ErrInvalidSpan|ErrInvalidSessionParams|StatusBooked|StatusBusy|StatusFree|StatusClosed)\b/slots.$1/g; s/\bmentorship\.ErrNoMentorZone\b/slots.ErrNoOwnerZone/g' internal/api/handler/mentorship*.go
command -v "$(go env GOPATH)/bin/goimports" >/dev/null || go install golang.org/x/tools/cmd/goimports@latest
"$(go env GOPATH)/bin/goimports" -w internal/engage/mentorship/*.go internal/api/handler/mentorship*.go
```

- [x] 1.8 Give the mentorship tests that stayed behind the two fixture helpers they shared with the moved files. Create `internal/engage/mentorship/engine_helpers_test.go`:

```go
package mentorship

import (
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// berlin is the zone the booking and cache tests state their mentor in. The engine's own
// copy moved to internal/engage/slots with the engine tests.
func berlin(t *testing.T) *time.Location {
	t.Helper()
	loc, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatalf("LoadLocation(Europe/Berlin): %v", err)
	}
	return loc
}

func mustTimeOfDay(t *testing.T, hour, minute int) slots.TimeOfDay {
	t.Helper()
	got, err := slots.NewTimeOfDay(hour, minute)
	if err != nil {
		t.Fatalf("NewTimeOfDay(%d, %d): %v", hour, minute, err)
	}
	return got
}
```

- [x] 1.9 Verify the move changed nothing:

```bash
gofmt -l internal/engage/slots internal/engage/mentorship internal/api/handler   # prints nothing
go build ./... && go vet ./...
go vet -tags=integration ./internal/engage/mentorship/... ./internal/api/handler/ ./internal/engage/slots/
go test ./internal/engage/slots/ ./internal/engage/mentorship/... ./internal/api/handler/ ./internal/platform/arch/layering/ -count=1
```
Expected: every command succeeds; `go test` prints `ok` for each package. Then the mentorship integration suite: `go test -tags=integration ./internal/engage/mentorship/ ./internal/api/handler/ -run 'Mentor|Mentorship' -count=1`. Expected: `ok`.

- [x] 1.10 Commit:

```bash
git add -A internal/engage/slots internal/engage/mentorship internal/api/handler internal/platform/arch/layering/blocks.go
git commit -m "Move the slot engine into internal/engage/slots" -m "Pure move: the engine, its calendar partition and their tests leave mentorship so buddy mock interviews can share them. MentorZone becomes OwnerZone, ErrNoMentorZone becomes ErrNoOwnerZone and withID is exported as WithID for the repositories."
```

## 2. Schema and queries (migration 0214_buddy_mocks.sql)

**Files:**
- Create: `migrations/0214_buddy_mocks.sql`, `internal/platform/db/queries/buddy_mocks.sql`, `internal/platform/db/buddy_mocks_integration_test.go`
- Modify: `internal/platform/db/queries/buddies.sql:20-40` (`LeaveBuddyMember`)
- Regenerate: `internal/platform/db/models.go`, `internal/platform/db/buddy_mocks.sql.go`, `internal/platform/db/buddies.sql.go`, `internal/platform/db/querier.go`

**Interfaces (generated by sqlc; verify after `make sqlc`):**
- Models: `db.BuddyMockSetting{UserID int64; Timezone string; MeetingUrl string; Topics []string; Paused bool; CreatedAt, UpdatedAt pgtype.Timestamptz}`, `db.BuddyMockAvailability{ID int64; UserID int64; Weekday pgtype.Int2; OnDate pgtype.Date; StartTime, EndTime pgtype.Time; CreatedAt pgtype.Timestamptz}`.
- `GetBuddyMockSettings(ctx, userID int64) (BuddyMockSetting, error)`
- `UpsertBuddyMockSettings(ctx, UpsertBuddyMockSettingsParams{UserID int64; Timezone string; MeetingUrl string; Topics []string; Paused bool}) (BuddyMockSetting, error)`
- `ListBuddyMockAvailability(ctx, userID int64) ([]BuddyMockAvailability, error)`
- `ReplaceBuddyMockWeekly(ctx, ReplaceBuddyMockWeeklyParams{UserID int64; Weekdays []int16; StartTimes, EndTimes []pgtype.Time}) error`
- `CreateBuddyMockAvailabilityRule(ctx, CreateBuddyMockAvailabilityRuleParams{UserID int64; Weekday pgtype.Int2; OnDate pgtype.Date; StartTime, EndTime pgtype.Time}) (BuddyMockAvailability, error)`
- `DeleteBuddyMockAvailabilityRule(ctx, DeleteBuddyMockAvailabilityRuleParams{ID, UserID int64}) (int64, error)`
- `ListBuddyMockBusy(ctx, ListBuddyMockBusyParams{UserID int64; WindowStart, WindowEnd pgtype.Timestamptz}) ([]ListBuddyMockBusyRow{StartsAt, EndsAt pgtype.Timestamptz}, error)`
- `CountFutureBuddyMocksAsGuest(ctx, guestUserID int64) (int64, error)`
- `CreateBuddyMock(ctx, CreateBuddyMockParams{HostUserID, GuestUserID int64; StartsAt, EndsAt pgtype.Timestamptz; Topic, GuestTimezone string}) (pgtype.UUID, error)` — `pgx.ErrNoRows` when the pair cannot meet; SQLSTATE 23P01 on `buddy_mock_participants_no_overlap`.
- `GetBuddyMock(ctx, id pgtype.UUID) (GetBuddyMockRow, error)` with `ID pgtype.UUID; HostUserID, GuestUserID int64; StartsAt, EndsAt pgtype.Timestamptz; Topic, Status, GuestTimezone string; CancelReason pgtype.Text; HostHandle, GuestHandle, HostTimezone, MeetingUrl, HostEmail, GuestEmail string`.
- `ListBuddyMocksForUser(ctx, ListBuddyMocksForUserParams{UserID int64; RowLimit int32}) ([]ListBuddyMocksForUserRow, error)` with `ID pgtype.UUID; HostUserID, GuestUserID int64; StartsAt, EndsAt pgtype.Timestamptz; Topic, Status string; CancelReason pgtype.Text; HostHandle, GuestHandle string; HostMeetingUrl pgtype.Text; CounterpartVisible bool; MyRating pgtype.Int2; MyComment pgtype.Text; TheirRating pgtype.Int2; TheirComment pgtype.Text`.
- `CancelBuddyMock(ctx, CancelBuddyMockParams{UserID int64; ID pgtype.UUID}) (pgtype.UUID, error)`
- `ClaimBuddyMockCancelNotices(ctx, actorID int64) ([]pgtype.UUID, error)`
- `CreateBuddyMockReview(ctx, CreateBuddyMockReviewParams{AuthorUserID int64; Rating int16; Comment string; MockID pgtype.UUID}) (int64, error)`
- `GetBuddyMemberMockSummary(ctx, subjectUserID int64) (GetBuddyMemberMockSummaryRow{Completed, RatingCount int64; RatingAvg float64}, error)`
- `ListBuddyMemberRevealedReviews(ctx, ListBuddyMemberRevealedReviewsParams{SubjectUserID int64; RowLimit int32}) ([]ListBuddyMemberRevealedReviewsRow{Rating int16; Comment string; CreatedAt pgtype.Timestamptz; Topic string}, error)`
- `ListBuddyMocksDueForReminder(ctx, ListBuddyMocksDueForReminderParams{OffsetMinutes, FloorMinutes, RowLimit int32}) ([]pgtype.UUID, error)`
- `RecordBuddyMockReminder(ctx, RecordBuddyMockReminderParams{MockID pgtype.UUID; OffsetMinutes int32}) (int64, error)`, `DeleteBuddyMockReminder(ctx, DeleteBuddyMockReminderParams{MockID pgtype.UUID; OffsetMinutes int32}) (int64, error)`
- `LeaveBuddyMember(ctx, userID int64) (BuddyMember, error)` — signature unchanged.

Design deviations recorded here (design.md D3 lists the columns): `buddy_mocks` gains `guest_timezone` (D9 renders the guest's notices "from the booking", and nothing else stores it), `cancel_reason` (the wording differs for a participant's cancellation and a leave/block) and `cancel_announced_at` (the block cancels inside a trigger, which cannot return rows, so the cancellation notice is claimed from this column afterwards — see Task 6).

- [x] 2.1 Write the failing schema test `internal/platform/db/buddy_mocks_integration_test.go`:

```go
//go:build integration

// Integration tests for the buddy mock-interview schema: the per-person EXCLUDE that keeps
// one member out of two overlapping mocks whatever their role, the single-statement booking
// guard, and the two cancellations that live in SQL — leaving the network and a block.
// Run with: go test -tags=integration ./internal/platform/db/
package db

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/platform/pgerr"
)

// mockMembers seeds n accounts and joins each to the buddy network.
func mockMembers(t *testing.T, pool *pgxpool.Pool, q *Queries, n int) []int64 {
	t.Helper()
	ids := make([]int64, n)
	for i := range ids {
		ids[i] = seedAccount(t, pool, fmt.Sprintf("mock-%d@example.test", i))
		if _, err := q.JoinBuddyMember(context.Background(), JoinBuddyMemberParams{
			UserID: ids[i], Handle: fmt.Sprintf("mock-member-%d", i),
		}); err != nil {
			t.Fatalf("join %d: %v", ids[i], err)
		}
	}
	return ids
}

// mockAt is a whole hour two days ahead plus offset: far from now and from any clock edge.
func mockAt(offset time.Duration) time.Time {
	return time.Now().UTC().Truncate(time.Hour).Add(48*time.Hour + offset)
}

func bookMock(ctx context.Context, q *Queries, host, guest int64, start time.Time) (pgtype.UUID, error) {
	return q.CreateBuddyMock(ctx, CreateBuddyMockParams{
		HostUserID:    host,
		GuestUserID:   guest,
		StartsAt:      pgtype.Timestamptz{Time: start, Valid: true},
		EndsAt:        pgtype.Timestamptz{Time: start.Add(time.Hour), Valid: true},
		Topic:         "coding",
		GuestTimezone: "UTC",
	})
}

func mustBookMock(t *testing.T, q *Queries, host, guest int64, start time.Time) pgtype.UUID {
	t.Helper()
	id, err := bookMock(context.Background(), q, host, guest, start)
	if err != nil {
		t.Fatalf("book %d with %d at %v: %v", host, guest, start, err)
	}
	return id
}

// mockState reads a mock's status, why it was cancelled, and how many of its two
// participant rows still hold time.
func mockState(t *testing.T, pool *pgxpool.Pool, id pgtype.UUID) (status, reason string, active int64) {
	t.Helper()
	if err := pool.QueryRow(context.Background(), `
		SELECT m.status, coalesce(m.cancel_reason, ''),
		       (SELECT count(*) FROM buddy_mock_participants p WHERE p.mock_id = m.id AND p.active)
		FROM buddy_mocks m WHERE m.id = $1`, id).Scan(&status, &reason, &active); err != nil {
		t.Fatalf("read mock: %v", err)
	}
	return status, reason, active
}

func TestBuddyMockParticipantsCannotOverlapInEitherRole(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	mustBookMock(t, q, a, b, mockAt(0))
	// a hosts at 0; being c's guest at +30m overlaps a's own row although the role differs.
	_, err := bookMock(ctx, q, c, a, mockAt(30*time.Minute))
	if name, ok := pgerr.ExclusionViolationConstraint(err); !ok || name != "buddy_mock_participants_no_overlap" {
		t.Fatalf("overlap as guest: err = %v, want the participants exclusion", err)
	}
	// Half-open bounds, as the slot engine's: back to back is not an overlap.
	mustBookMock(t, q, c, a, mockAt(time.Hour))

	// The refused insert was one statement and left no orphan mock row behind.
	var n int64
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM buddy_mocks`).Scan(&n); err != nil || n != 2 {
		t.Fatalf("mock rows = %d (err %v), want 2", n, err)
	}
}

func TestCancelBuddyMockFreesBothParticipants(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	id := mustBookMock(t, q, a, b, mockAt(0))
	got, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: id, UserID: b})
	if err != nil || got != id {
		t.Fatalf("cancel = %v, %v", got, err)
	}
	if status, reason, active := mockState(t, pool, id); status != "cancelled" || reason != "participant" || active != 0 {
		t.Fatalf("after cancel: %s/%s/%d active", status, reason, active)
	}
	// Both people's time is free again, in either role.
	mustBookMock(t, q, b, a, mockAt(0))

	other := mustBookMock(t, q, a, b, mockAt(4*time.Hour))
	if _, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: other, UserID: c}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("a stranger's cancel: err = %v, want no rows", err)
	}
	if _, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: id, UserID: a}); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("cancelling twice: err = %v, want no rows", err)
	}
}

func TestBlockCancelsThePairsFutureMocks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	pair := mustBookMock(t, q, a, b, mockAt(0))
	unrelated := mustBookMock(t, q, a, c, mockAt(2*time.Hour))
	// A mock already under way is history rather than a plan, and the block leaves it alone.
	var started pgtype.UUID
	if err := pool.QueryRow(ctx, `
		INSERT INTO buddy_mocks (host_user_id, guest_user_id, starts_at, ends_at, topic, guest_timezone)
		VALUES ($1, $2, now() - interval '10 minutes', now() + interval '50 minutes', 'coding', 'UTC')
		RETURNING id`, b, a).Scan(&started); err != nil {
		t.Fatalf("seed started mock: %v", err)
	}

	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, b, a); err != nil {
		t.Fatalf("block: %v", err)
	}

	if status, reason, active := mockState(t, pool, pair); status != "cancelled" || reason != "blocked" || active != 0 {
		t.Fatalf("pair mock after block: %s/%s/%d active", status, reason, active)
	}
	var by int64
	if err := pool.QueryRow(ctx, `SELECT cancelled_by FROM buddy_mocks WHERE id = $1`, pair).Scan(&by); err != nil || by != b {
		t.Fatalf("cancelled_by = %d (err %v), want the blocker %d", by, err, b)
	}
	if status, _, _ := mockState(t, pool, unrelated); status != "confirmed" {
		t.Fatalf("a mock with someone else was cancelled: %s", status)
	}
	if status, _, _ := mockState(t, pool, started); status != "confirmed" {
		t.Fatalf("a started mock was cancelled: %s", status)
	}
}

func TestLeaveBuddyMemberCancelsTheirFutureMocks(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	asHost := mustBookMock(t, q, a, b, mockAt(0))
	asGuest := mustBookMock(t, q, c, a, mockAt(2*time.Hour))
	unrelated := mustBookMock(t, q, b, c, mockAt(4*time.Hour))

	if _, err := q.LeaveBuddyMember(ctx, a); err != nil {
		t.Fatalf("leave: %v", err)
	}
	for name, id := range map[string]pgtype.UUID{"as host": asHost, "as guest": asGuest} {
		if status, reason, active := mockState(t, pool, id); status != "cancelled" || reason != "left" || active != 0 {
			t.Fatalf("%s after leave: %s/%s/%d active", name, status, reason, active)
		}
	}
	if status, _, _ := mockState(t, pool, unrelated); status != "confirmed" {
		t.Fatalf("someone else's mock was cancelled by the leave: %s", status)
	}
}

func TestCreateBuddyMockRefusesAPairThatCannotMeet(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 3)
	a, b, c := m[0], m[1], m[2]

	if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, b, a); err != nil {
		t.Fatalf("block: %v", err)
	}
	if _, err := bookMock(ctx, q, a, b, mockAt(0)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("booking across a block: err = %v, want no rows", err)
	}
	if _, err := q.LeaveBuddyMember(ctx, c); err != nil {
		t.Fatalf("leave: %v", err)
	}
	if _, err := bookMock(ctx, q, a, c, mockAt(0)); !errors.Is(err, pgx.ErrNoRows) {
		t.Fatalf("booking a member who left: err = %v, want no rows", err)
	}
}

func TestClaimBuddyMockCancelNoticesOnce(t *testing.T) {
	pool := startPostgres(t)
	q := New(pool)
	ctx := context.Background()
	m := mockMembers(t, pool, q, 2)
	a, b := m[0], m[1]

	id := mustBookMock(t, q, a, b, mockAt(0))
	if _, err := q.CancelBuddyMock(ctx, CancelBuddyMockParams{ID: id, UserID: a}); err != nil {
		t.Fatalf("cancel: %v", err)
	}
	first, err := q.ClaimBuddyMockCancelNotices(ctx, a)
	if err != nil || len(first) != 1 || first[0] != id {
		t.Fatalf("first claim = %v, %v", first, err)
	}
	if again, err := q.ClaimBuddyMockCancelNotices(ctx, a); err != nil || len(again) != 0 {
		t.Fatalf("second claim = %v, %v, want nothing", again, err)
	}
}
```

- [x] 2.2 Run it: `go vet -tags=integration ./internal/platform/db/`. Expected: FAIL — `undefined: CreateBuddyMockParams`, `q.CreateBuddyMock undefined`, … .

- [x] 2.3 Create `migrations/0214_buddy_mocks.sql`:

```sql
-- Buddy mock interviews (openspec/changes/buddy-mock-interviews): one interview-buddy member
-- books a 60-minute peer mock from another's published availability. Every reference to
-- users cascades, and every one is indexed on this side (TestEveryUserForeignKeyIsIndexed).

-- btree_gist backs the participants EXCLUDE below (a scalar = beside a range &&). 0145
-- already created it; repeated so this file stands on its own.
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- A member's mock-host settings. timezone is an IANA name, validated where it is read, for
-- the reason 0145 gives for mentors.timezone. The link is the host's own call room; only
-- the two participants of a confirmed mock ever read it, and it is read live rather than
-- snapshotted, so a host who fixes a typo reaches guests who already booked.
CREATE TABLE IF NOT EXISTS buddy_mock_settings (
    user_id     bigint      PRIMARY KEY REFERENCES buddy_members (user_id) ON DELETE CASCADE,
    timezone    text        NOT NULL,
    meeting_url text        NOT NULL,
    topics      text[]      NOT NULL,
    paused      boolean     NOT NULL DEFAULT false,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buddy_mock_settings_url_check
        CHECK (meeting_url LIKE 'https://%' AND char_length(meeting_url) <= 500),
    CONSTRAINT buddy_mock_settings_topics_check
        CHECK (cardinality(topics) > 0
               AND topics <@ ARRAY['coding', 'system_design', 'behavioral']::text[])
);

-- Availability in the two row shapes mentor_availability has (0145): a weekly row names a
-- weekday, a dated row names one date and replaces that date's weekly rows; an empty dated
-- row closes its date. Times carry no zone: they resolve through the host's settings zone.
CREATE TABLE IF NOT EXISTS buddy_mock_availability (
    id         bigint      GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id    bigint      NOT NULL REFERENCES buddy_members (user_id) ON DELETE CASCADE,
    -- A day of the week, 0 through 6.
    -- squawk-ignore prefer-bigint-over-smallint
    weekday    smallint,
    on_date    date,
    start_time time        NOT NULL,
    end_time   time        NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buddy_mock_availability_shape_check
        CHECK ((weekday IS NULL) <> (on_date IS NULL)),
    CONSTRAINT buddy_mock_availability_weekday_check
        CHECK (weekday IS NULL OR weekday BETWEEN 0 AND 6),
    CONSTRAINT buddy_mock_availability_span_check
        CHECK (end_time >= start_time AND (on_date IS NOT NULL OR end_time > start_time))
);

CREATE INDEX IF NOT EXISTS buddy_mock_availability_user_idx ON buddy_mock_availability (user_id);

-- One booked mock. The id is a random uuid because two accounts read it. There is no
-- 'completed' status: completion is "confirmed and ended". guest_timezone is what the
-- guest's notices are rendered in; the host's is read from their settings. cancel_reason
-- picks the wording of the cancellation notice, and cancel_announced_at is that notice's
-- claim: a block cancels inside a trigger, which cannot hand rows back to the caller.
CREATE TABLE IF NOT EXISTS buddy_mocks (
    id                  uuid        PRIMARY KEY DEFAULT gen_random_uuid(),
    host_user_id        bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    guest_user_id       bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    starts_at           timestamptz NOT NULL,
    ends_at             timestamptz NOT NULL,
    topic               text        NOT NULL,
    guest_timezone      text        NOT NULL,
    status              text        NOT NULL DEFAULT 'confirmed',
    cancelled_by        bigint      REFERENCES users (id) ON DELETE SET NULL,
    cancelled_at        timestamptz,
    cancel_reason       text,
    cancel_announced_at timestamptz,
    created_at          timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT buddy_mocks_pair_check   CHECK (host_user_id <> guest_user_id),
    CONSTRAINT buddy_mocks_span_check   CHECK (ends_at > starts_at),
    CONSTRAINT buddy_mocks_topic_check  CHECK (topic IN ('coding', 'system_design', 'behavioral')),
    CONSTRAINT buddy_mocks_status_check CHECK (status IN ('confirmed', 'cancelled')),
    CONSTRAINT buddy_mocks_cancelled_check
        CHECK ((status = 'cancelled') = (cancelled_at IS NOT NULL)
               AND (status = 'cancelled') = (cancel_reason IS NOT NULL)),
    CONSTRAINT buddy_mocks_cancel_reason_check
        CHECK (cancel_reason IN ('participant', 'left', 'blocked'))
);

-- Each member's own list, either role, and account deletion.
CREATE INDEX IF NOT EXISTS buddy_mocks_host_starts_at_idx ON buddy_mocks (host_user_id, starts_at);
CREATE INDEX IF NOT EXISTS buddy_mocks_guest_starts_at_idx ON buddy_mocks (guest_user_id, starts_at);
-- Account deletion and the cancellation-notice claim. Partial: a live mock has nobody who
-- cancelled it.
CREATE INDEX IF NOT EXISTS buddy_mocks_cancelled_by_idx
    ON buddy_mocks (cancelled_by) WHERE cancelled_by IS NOT NULL;
-- The reminder worker's scan.
CREATE INDEX IF NOT EXISTS buddy_mocks_upcoming_idx
    ON buddy_mocks (starts_at) WHERE status = 'confirmed';

-- One row per person per mock, so ONE constraint can refuse "host at 19:00 and guest at
-- 19:00": a mentor booking only constrains the mentor, while here either person may hold
-- either role. Half-open '[)' bounds match slots.Interval.Overlaps, so back-to-back mocks
-- are legal. The WHERE is load-bearing: a cancelled mock sets active = false in the same
-- statement as its status, and frees both people's time.
CREATE TABLE IF NOT EXISTS buddy_mock_participants (
    mock_id   uuid        NOT NULL REFERENCES buddy_mocks (id) ON DELETE CASCADE,
    user_id   bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    starts_at timestamptz NOT NULL,
    ends_at   timestamptz NOT NULL,
    active    boolean     NOT NULL DEFAULT true,

    PRIMARY KEY (mock_id, user_id),
    CONSTRAINT buddy_mock_participants_span_check CHECK (ends_at > starts_at),
    CONSTRAINT buddy_mock_participants_no_overlap
        EXCLUDE USING gist (
            user_id WITH =,
            tstzrange(starts_at, ends_at, '[)') WITH &&
        ) WHERE (active)
);

-- A member's busy time for the slot engine, and account deletion (the EXCLUDE's index is
-- partial on active and cannot serve the cascade).
CREATE INDEX IF NOT EXISTS buddy_mock_participants_user_idx
    ON buddy_mock_participants (user_id, starts_at);

-- One review per participant per mock; the key is what makes a second one impossible.
-- Revealed when the counterpart has reviewed too or seven days after the end — computed in
-- the read queries, never stored.
CREATE TABLE IF NOT EXISTS buddy_mock_reviews (
    mock_id         uuid        NOT NULL REFERENCES buddy_mocks (id) ON DELETE CASCADE,
    author_user_id  bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    subject_user_id bigint      NOT NULL REFERENCES users (id) ON DELETE CASCADE,
    -- One to five stars.
    -- squawk-ignore prefer-bigint-over-smallint
    rating          smallint    NOT NULL,
    comment         text        NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (mock_id, author_user_id),
    CONSTRAINT buddy_mock_reviews_rating_check  CHECK (rating BETWEEN 1 AND 5),
    CONSTRAINT buddy_mock_reviews_comment_check CHECK (char_length(comment) <= 1000),
    CONSTRAINT buddy_mock_reviews_pair_check    CHECK (author_user_id <> subject_user_id)
);

CREATE INDEX IF NOT EXISTS buddy_mock_reviews_author_idx ON buddy_mock_reviews (author_user_id);
-- The member page's aggregate and recent reviews.
CREATE INDEX IF NOT EXISTS buddy_mock_reviews_subject_idx
    ON buddy_mock_reviews (subject_user_id, created_at DESC);

-- What cmd/mentorship-remind already sent for a mock; the composite key is the claim, as
-- in mentor_booking_reminders.
CREATE TABLE IF NOT EXISTS buddy_mock_reminders (
    mock_id        uuid        NOT NULL REFERENCES buddy_mocks (id) ON DELETE CASCADE,
    -- Minutes before the mock: 1440 and 60 today. A label, not a counter.
    -- squawk-ignore prefer-bigint-over-int
    offset_minutes integer     NOT NULL,
    sent_at        timestamptz NOT NULL DEFAULT now(),

    PRIMARY KEY (mock_id, offset_minutes),
    CONSTRAINT buddy_mock_reminders_offset_check CHECK (offset_minutes > 0)
);

-- A block cancels the pair's future mocks inside the block's own statement. The block is
-- written by the messaging core (BlockFromConversation), which never imports a feature, so
-- the feature reacts here; every path that ever inserts a block is covered by construction.
-- cancelled_by is the blocker, which is how the caller claims the notice afterwards.
CREATE OR REPLACE FUNCTION public.buddy_mocks_cancel_on_block() RETURNS trigger
LANGUAGE plpgsql AS $$
BEGIN
    WITH cancelled AS (
        UPDATE buddy_mocks m
        SET status = 'cancelled', cancelled_at = now(), cancelled_by = NEW.blocker_id,
            cancel_reason = 'blocked'
        WHERE m.status = 'confirmed'
          AND m.starts_at > now()
          AND ((m.host_user_id = NEW.blocker_id AND m.guest_user_id = NEW.blocked_id)
            OR (m.host_user_id = NEW.blocked_id AND m.guest_user_id = NEW.blocker_id))
        RETURNING m.id
    )
    UPDATE buddy_mock_participants p
    SET active = false
    WHERE p.mock_id IN (SELECT id FROM cancelled);
    RETURN NULL;
END;
$$;

CREATE OR REPLACE TRIGGER buddy_mocks_cancel_on_block
    AFTER INSERT ON user_blocks
    FOR EACH ROW EXECUTE FUNCTION public.buddy_mocks_cancel_on_block();
```

- [x] 2.4 Create `internal/platform/db/queries/buddy_mocks.sql`:

```sql
-- name: GetBuddyMockSettings :one
-- A member's mock-host settings, or no row when they never saved any.
SELECT * FROM buddy_mock_settings WHERE user_id = $1;

-- name: UpsertBuddyMockSettings :one
-- Save the whole settings block. The service validates zone, link and topics first; the
-- CHECKs repeat the link and topic rules so no other writer can skip them.
INSERT INTO buddy_mock_settings (user_id, timezone, meeting_url, topics, paused)
VALUES (sqlc.arg(user_id), sqlc.arg(timezone), sqlc.arg(meeting_url), sqlc.arg(topics)::text[], sqlc.arg(paused))
ON CONFLICT (user_id) DO UPDATE
SET timezone = EXCLUDED.timezone, meeting_url = EXCLUDED.meeting_url, topics = EXCLUDED.topics,
    paused = EXCLUDED.paused, updated_at = now()
RETURNING *;

-- name: ListBuddyMockAvailability :many
-- Every availability row of a member, both shapes: an override only means anything beside
-- the weekly rules it replaces.
SELECT * FROM buddy_mock_availability WHERE user_id = $1 ORDER BY id;

-- name: ReplaceBuddyMockWeekly :exec
-- Swap the recurring week in one statement, leaving dated overrides alone: half of the
-- edit applied is a member bookable at hours they just removed. The three arrays are one
-- row each, zipped back by position (WITH ORDINALITY: sqlc's analyzer does not resolve the
-- multi-array form of unnest).
WITH cleared AS (
    DELETE FROM buddy_mock_availability
    WHERE user_id = sqlc.arg(user_id)::bigint AND weekday IS NOT NULL
)
INSERT INTO buddy_mock_availability (user_id, weekday, start_time, end_time)
SELECT sqlc.arg(user_id)::bigint, w.weekday, s.start_time, e.end_time
FROM unnest(sqlc.arg(weekdays)::smallint[]) WITH ORDINALITY AS w (weekday, ord)
JOIN unnest(sqlc.arg(start_times)::time[]) WITH ORDINALITY AS s (start_time, ord) ON s.ord = w.ord
JOIN unnest(sqlc.arg(end_times)::time[]) WITH ORDINALITY AS e (end_time, ord) ON e.ord = w.ord;

-- name: CreateBuddyMockAvailabilityRule :one
-- One row, weekly or dated; which one is whichever of weekday and on_date is set.
INSERT INTO buddy_mock_availability (user_id, weekday, on_date, start_time, end_time)
VALUES (sqlc.arg(user_id), sqlc.narg(weekday), sqlc.narg(on_date), sqlc.arg(start_time), sqlc.arg(end_time))
RETURNING *;

-- name: DeleteBuddyMockAvailabilityRule :execrows
-- The user_id guard scopes the delete to the owner's own schedule.
DELETE FROM buddy_mock_availability WHERE id = sqlc.arg(id) AND user_id = sqlc.arg(user_id);

-- name: ListBuddyMockBusy :many
-- A member's occupied time in a window: every mock they are in, as host or as guest. The
-- slot engine subtracts it from their availability.
SELECT starts_at, ends_at
FROM buddy_mock_participants
WHERE user_id = sqlc.arg(user_id)
  AND active
  AND starts_at < sqlc.arg(window_end)
  AND ends_at > sqlc.arg(window_start)
ORDER BY starts_at;

-- name: CountFutureBuddyMocksAsGuest :one
-- The spam bound's count: confirmed mocks the member is a guest in that have not started.
SELECT count(*) FROM buddy_mocks
WHERE guest_user_id = $1 AND status = 'confirmed' AND starts_at > now();

-- name: CreateBuddyMock :one
-- Book a mock and both participant rows in ONE statement. The guard repeats, inside the
-- write, what the service read before it: both people are active members and no block
-- stands between them; a pair that cannot meet inserts nothing and returns no row. The
-- buddy_mock_participants_no_overlap EXCLUDE refuses a person already in an overlapping
-- mock in either role; the repository maps that 23P01 to the same "slot unavailable" a
-- stale page gets.
WITH mock AS (
    INSERT INTO buddy_mocks (host_user_id, guest_user_id, starts_at, ends_at, topic, guest_timezone)
    SELECT sqlc.arg(host_user_id)::bigint, sqlc.arg(guest_user_id)::bigint,
           sqlc.arg(starts_at)::timestamptz, sqlc.arg(ends_at)::timestamptz,
           sqlc.arg(topic)::text, sqlc.arg(guest_timezone)::text
    WHERE EXISTS (SELECT 1 FROM buddy_members
                  WHERE user_id = sqlc.arg(host_user_id)::bigint AND left_at IS NULL)
      AND EXISTS (SELECT 1 FROM buddy_members
                  WHERE user_id = sqlc.arg(guest_user_id)::bigint AND left_at IS NULL)
      AND NOT EXISTS (
          SELECT 1 FROM user_blocks b
          WHERE (b.blocker_id = sqlc.arg(host_user_id)::bigint AND b.blocked_id = sqlc.arg(guest_user_id)::bigint)
             OR (b.blocker_id = sqlc.arg(guest_user_id)::bigint AND b.blocked_id = sqlc.arg(host_user_id)::bigint))
    RETURNING id, host_user_id, guest_user_id, starts_at, ends_at
), seats AS (
    INSERT INTO buddy_mock_participants (mock_id, user_id, starts_at, ends_at)
    SELECT mock.id, mock.host_user_id, mock.starts_at, mock.ends_at FROM mock
    UNION ALL
    SELECT mock.id, mock.guest_user_id, mock.starts_at, mock.ends_at FROM mock
)
SELECT id FROM mock;

-- name: GetBuddyMock :one
-- One mock with what both participants' notices need: both handles, the host's CURRENT
-- zone and link, and both addresses — each address only ever addresses its own owner's
-- notice. Authorisation is the caller's: this answers for any id.
SELECT m.id, m.host_user_id, m.guest_user_id, m.starts_at, m.ends_at, m.topic, m.status,
       m.guest_timezone, m.cancel_reason,
       hm.handle AS host_handle, gm.handle AS guest_handle,
       coalesce(s.timezone, '')::text AS host_timezone,
       coalesce(s.meeting_url, '')::text AS meeting_url,
       hu.email AS host_email, gu.email AS guest_email
FROM buddy_mocks m
JOIN buddy_members hm ON hm.user_id = m.host_user_id
JOIN buddy_members gm ON gm.user_id = m.guest_user_id
JOIN users hu ON hu.id = m.host_user_id
JOIN users gu ON gu.id = m.guest_user_id
LEFT JOIN buddy_mock_settings s ON s.user_id = m.host_user_id
WHERE m.id = $1;

-- name: ListBuddyMocksForUser :many
-- A member's own mocks, newest first, with the host's current link, whether the other side
-- may still be drawn (both active, no block either way — the ListBuddyCounterparts rule),
-- the member's own review, and the review ABOUT them only once revealed: they have written
-- theirs, or seven days have passed since the end. The same reveal rule is restated in
-- GetBuddyMemberMockSummary and ListBuddyMemberRevealedReviews; change all three together.
SELECT m.id, m.host_user_id, m.guest_user_id, m.starts_at, m.ends_at, m.topic, m.status,
       m.cancel_reason,
       hm.handle AS host_handle, gm.handle AS guest_handle,
       s.meeting_url AS host_meeting_url,
       (cm.left_at IS NULL
        AND NOT EXISTS (
            SELECT 1 FROM user_blocks b
            WHERE (b.blocker_id = sqlc.arg(user_id)::bigint AND b.blocked_id = cm.user_id)
               OR (b.blocker_id = cm.user_id AND b.blocked_id = sqlc.arg(user_id)::bigint)))::boolean AS counterpart_visible,
       mine.rating AS my_rating, mine.comment AS my_comment,
       theirs.rating AS their_rating, theirs.comment AS their_comment
FROM buddy_mocks m
JOIN buddy_members hm ON hm.user_id = m.host_user_id
JOIN buddy_members gm ON gm.user_id = m.guest_user_id
JOIN buddy_members cm ON cm.user_id = CASE WHEN m.host_user_id = sqlc.arg(user_id)::bigint
                                           THEN m.guest_user_id ELSE m.host_user_id END
LEFT JOIN buddy_mock_settings s ON s.user_id = m.host_user_id
LEFT JOIN buddy_mock_reviews mine
       ON mine.mock_id = m.id AND mine.author_user_id = sqlc.arg(user_id)::bigint
LEFT JOIN buddy_mock_reviews theirs
       ON theirs.mock_id = m.id AND theirs.subject_user_id = sqlc.arg(user_id)::bigint
      AND (mine.mock_id IS NOT NULL OR m.ends_at + interval '7 days' <= now())
WHERE m.host_user_id = sqlc.arg(user_id)::bigint OR m.guest_user_id = sqlc.arg(user_id)::bigint
ORDER BY m.starts_at DESC
LIMIT sqlc.arg(row_limit);

-- name: CancelBuddyMock :one
-- A participant cancels a confirmed mock before it starts, freeing both people's time in
-- the same statement. No row: not a participant, already cancelled, or already started.
WITH cancelled AS (
    UPDATE buddy_mocks m
    SET status = 'cancelled', cancelled_at = now(), cancelled_by = sqlc.arg(user_id)::bigint,
        cancel_reason = 'participant'
    WHERE m.id = sqlc.arg(id)
      AND m.status = 'confirmed'
      AND m.starts_at > now()
      AND (m.host_user_id = sqlc.arg(user_id)::bigint OR m.guest_user_id = sqlc.arg(user_id)::bigint)
    RETURNING m.id
), freed AS (
    UPDATE buddy_mock_participants p
    SET active = false
    WHERE p.mock_id IN (SELECT id FROM cancelled)
)
SELECT id FROM cancelled;

-- name: ClaimBuddyMockCancelNotices :many
-- Claim the cancellations an actor caused that nobody has announced yet — by their own
-- cancel, their leave, or their block (whose trigger cannot return rows). A concurrent
-- claim waits on the row lock and then matches nothing, so each notice goes out once.
-- Bounded to future mocks cancelled in the last day: an announcement lost to a crash is not
-- worth sending weeks later.
UPDATE buddy_mocks
SET cancel_announced_at = now()
WHERE cancelled_by = sqlc.arg(actor_id)::bigint
  AND status = 'cancelled'
  AND cancel_announced_at IS NULL
  AND cancelled_at > now() - interval '1 day'
  AND starts_at > now()
RETURNING id;

-- name: CreateBuddyMockReview :execrows
-- One review of the other participant, once, between the mock's end and seven days after
-- it, and only of a confirmed mock. Zero rows: already reviewed (the key) or outside the
-- rules (the WHERE); the service reads the mock first to say which.
INSERT INTO buddy_mock_reviews (mock_id, author_user_id, subject_user_id, rating, comment)
SELECT m.id, sqlc.arg(author_user_id)::bigint,
       CASE WHEN m.host_user_id = sqlc.arg(author_user_id)::bigint THEN m.guest_user_id ELSE m.host_user_id END,
       sqlc.arg(rating)::smallint, sqlc.arg(comment)::text
FROM buddy_mocks m
WHERE m.id = sqlc.arg(mock_id)
  AND m.status = 'confirmed'
  AND m.ends_at <= now()
  AND m.ends_at + interval '7 days' > now()
  AND (m.host_user_id = sqlc.arg(author_user_id)::bigint OR m.guest_user_id = sqlc.arg(author_user_id)::bigint)
ON CONFLICT (mock_id, author_user_id) DO NOTHING;

-- name: GetBuddyMemberMockSummary :one
-- What a member page shows: completed mocks (confirmed and ended, either role) and the
-- count and average of REVEALED reviews about the member. Zeroes, never NULLs.
SELECT
    (SELECT count(*) FROM buddy_mocks c
     WHERE c.status = 'confirmed' AND c.ends_at <= now()
       AND (c.host_user_id = sqlc.arg(subject_user_id)::bigint
            OR c.guest_user_id = sqlc.arg(subject_user_id)::bigint))::bigint AS completed,
    count(r.mock_id)::bigint AS rating_count,
    coalesce(avg(r.rating), 0)::float8 AS rating_avg
FROM buddy_mock_reviews r
JOIN buddy_mocks m ON m.id = r.mock_id
WHERE r.subject_user_id = sqlc.arg(subject_user_id)::bigint
  AND (EXISTS (SELECT 1 FROM buddy_mock_reviews o
               WHERE o.mock_id = r.mock_id AND o.author_user_id = r.subject_user_id)
       OR m.ends_at + interval '7 days' <= now());

-- name: ListBuddyMemberRevealedReviews :many
-- The most recent revealed reviews about a member, without their authors: who mocked with
-- whom is a social graph no member page needs to publish.
SELECT r.rating, r.comment, r.created_at, m.topic
FROM buddy_mock_reviews r
JOIN buddy_mocks m ON m.id = r.mock_id
WHERE r.subject_user_id = sqlc.arg(subject_user_id)::bigint
  AND (EXISTS (SELECT 1 FROM buddy_mock_reviews o
               WHERE o.mock_id = r.mock_id AND o.author_user_id = r.subject_user_id)
       OR m.ends_at + interval '7 days' <= now())
ORDER BY r.created_at DESC
LIMIT sqlc.arg(row_limit);

-- name: ListBuddyMocksDueForReminder :many
-- Confirmed mocks starting inside one offset's window (floor, offset] and not yet reminded
-- at it. starts_at > now() keeps a late run from reminding about a mock already under way.
SELECT m.id
FROM buddy_mocks m
WHERE m.status = 'confirmed'
  AND m.starts_at > now()
  AND m.starts_at <= now() + make_interval(mins => sqlc.arg(offset_minutes)::int)
  AND m.starts_at > now() + make_interval(mins => sqlc.arg(floor_minutes)::int)
  AND NOT EXISTS (
      SELECT 1 FROM buddy_mock_reminders r
      WHERE r.mock_id = m.id AND r.offset_minutes = sqlc.arg(offset_minutes)::int)
ORDER BY m.starts_at
LIMIT sqlc.arg(row_limit);

-- name: RecordBuddyMockReminder :execrows
-- Claim one reminder. Zero rows means another run holds it: send only after this returns 1.
INSERT INTO buddy_mock_reminders (mock_id, offset_minutes)
VALUES (sqlc.arg(mock_id), sqlc.arg(offset_minutes))
ON CONFLICT DO NOTHING;

-- name: DeleteBuddyMockReminder :execrows
-- Give a claim back after a failed delivery, so the next run retries.
DELETE FROM buddy_mock_reminders
WHERE mock_id = sqlc.arg(mock_id) AND offset_minutes = sqlc.arg(offset_minutes);
```

- [x] 2.5 Make the leave cancel the member's future mocks in its own statement. In `internal/platform/db/queries/buddies.sql` replace lines 20-40 (`-- name: LeaveBuddyMember :one` through `RETURNING bm.*;`) with:

```sql
-- name: LeaveBuddyMember :one
-- Opt a member out: stamp left_at, close every pending or open buddy conversation they
-- are in, and cancel every future mock they are in (either role, freeing both people's
-- time), in one statement so a leave cannot half-apply. The conversations are the
-- messaging core's (kind 'buddy'); closing them here rather than through the core is what
-- keeps the writes atomic. The row is kept, so rejoining restores the handle. The member
-- update is guarded by left_at IS NULL, so for a member who is not active it returns no
-- row; the close and the cancel are not guarded, which is harmless because a left member
-- has no pending or open buddy conversation and no future mock. cancelled_by is the
-- leaver: the caller claims the cancellation notices by it afterwards.
WITH closed AS (
    UPDATE conversations c
    SET status = 'closed', closed_at = now()
    WHERE c.kind = 'buddy'
      AND c.status IN ('pending', 'open')
      AND EXISTS (
          SELECT 1 FROM conversation_participants p
          WHERE p.conversation_id = c.id AND p.user_id = sqlc.arg(user_id))
), cancelled AS (
    UPDATE buddy_mocks m
    SET status = 'cancelled', cancelled_at = now(), cancelled_by = sqlc.arg(user_id)::bigint,
        cancel_reason = 'left'
    WHERE m.status = 'confirmed'
      AND m.starts_at > now()
      AND (m.host_user_id = sqlc.arg(user_id)::bigint OR m.guest_user_id = sqlc.arg(user_id)::bigint)
    RETURNING m.id
), freed AS (
    UPDATE buddy_mock_participants p
    SET active = false
    WHERE p.mock_id IN (SELECT id FROM cancelled)
)
UPDATE buddy_members bm
SET left_at = now()
WHERE bm.user_id = sqlc.arg(user_id) AND bm.left_at IS NULL
RETURNING bm.*;
```

- [x] 2.6 Generate and check the signatures the later tasks rely on:

```bash
make sqlc
grep -n 'func (q \*Queries) LeaveBuddyMember(ctx context.Context, userID int64)' internal/platform/db/buddies.sql.go
grep -n 'type BuddyMockSetting struct\|type BuddyMockAvailability struct' internal/platform/db/models.go
grep -n '^func (q \*Queries) \(CreateBuddyMock\|GetBuddyMock\|ClaimBuddyMockCancelNotices\|CountFutureBuddyMocksAsGuest\|GetBuddyMemberMockSummary\)(' internal/platform/db/buddy_mocks.sql.go
grep -n 'HostTimezone \|MeetingUrl \|HostEmail \|CounterpartVisible \|RatingAvg ' internal/platform/db/buddy_mocks.sql.go
```
Expected: every grep prints a line; `HostTimezone`, `MeetingUrl` and `HostEmail` are `string`, `CounterpartVisible` is `bool`, `RatingAvg` is `float64`, `ClaimBuddyMockCancelNotices(ctx context.Context, actorID int64) ([]pgtype.UUID, error)`. If sqlc typed `host_timezone`/`meeting_url` as `pgtype.Text` despite the `coalesce(...)::text`, change the mapping in Task 5 (`row.HostTimezone.String`, `row.MeetingUrl.String`) rather than the query.

- [x] 2.7 Run the schema tests and the account-deletion guard:

```bash
go test -tags=integration ./internal/platform/db/ -run 'BuddyMock|TestBlockCancelsThePairsFutureMocks|TestLeaveBuddyMemberCancelsTheirFutureMocks|TestCreateBuddyMockRefusesAPairThatCannotMeet|TestEveryUserForeignKeyIsIndexed|TestDeletingUserCascadesBuddyData|TestBuddyMembershipQueries' -count=1
```
Expected: `ok`.

- [x] 2.8 Lint the new migration: `pnpm check:sql`. Expected: no findings for `0214_buddy_mocks.sql` (fix a finding by the rule's own advice, or with a `-- squawk-ignore <rule>` line before the statement and the reason beside it).

- [x] 2.9 Verify and commit:

```bash
gofmt -l internal/platform/db && go vet ./... && go test ./... && go vet -tags=integration ./internal/platform/db/
git add migrations/0214_buddy_mocks.sql internal/platform/db/queries/buddy_mocks.sql internal/platform/db/queries/buddies.sql internal/platform/db/*.go
git commit -m "Add the buddy mock-interview schema and queries" -m "Six tables, a per-person EXCLUDE over the participant rows, a single-statement booking guard, the leave cancelling the member's future mocks, and a user_blocks trigger cancelling the pair's."
```

## 3. Mock settings and availability

**Files:**
- Create: `internal/engage/buddy/mock.go`, `internal/engage/buddy/mock_service.go`, `internal/engage/buddy/mock_repository.go`, `internal/engage/buddy/mock_test.go`, `internal/engage/buddy/mock_helpers_integration_test.go`, `internal/engage/buddy/mock_settings_integration_test.go`

**Interfaces:**
- Consumes: `slots.SessionParams`, `slots.Slots`, `slots.SlotRequest`, `slots.SlotResult`, `slots.Rule` (+ `WithID`, `NewWeeklyRule`, `NewDatedRule`, `NewDate`, `NewTimeOfDay`), `slots.Interval`, `slots.ErrInvalidSpan` (Task 1); `db.Queries.GetBuddyMockSettings`, `UpsertBuddyMockSettings`, `ListBuddyMockAvailability`, `ReplaceBuddyMockWeekly`, `CreateBuddyMockAvailabilityRule`, `DeleteBuddyMockAvailabilityRule` (Task 2); `(*Service).requireActive(ctx, userID int64) error` (`feed.go:208`), `ErrNotFound`, `ErrNotMember` (`buddy.go`).
- Produces:
  - `const MockDuration, MockMinimumNotice, MockHorizon time.Duration`; `const TopicCoding, TopicSystemDesign, TopicBehavioral = "coding", "system_design", "behavioral"`; `var MockTopics []string`; `const MockConfirmed, MockCancelled`; `const RoleHost, RoleGuest`; `const CancelByParticipant, CancelByLeave, CancelByBlock = "participant", "left", "blocked"`
  - `var ErrInvalidMockSettings error`
  - `type MockSettings struct{ Timezone, MeetingURL string; Topics []string; Paused bool }`
  - `type Mock struct{ ID uuid.UUID; HostUserID, GuestUserID int64; HostHandle, GuestHandle string; StartsAt, EndsAt time.Time; Topic, Status, CancelReason, HostTimezone, GuestTimezone, MeetingURL, HostEmail, GuestEmail string }`
  - `type MockNotifier interface{ MockBooked(ctx, Mock) error; MockCancelled(ctx, Mock, actorID int64) error; MockReminder(ctx, Mock, before time.Duration) error }`
  - `type MockRepository interface{ MockSettings; SaveMockSettings; MockAvailability; ReplaceMockWeekly; AddMockRule; DeleteMockRule }` (grows in Tasks 4-7 and 9)
  - `func NewMockService(members *Service, repo MockRepository, notifier MockNotifier) *MockService`
  - `func (*MockService) Settings(ctx, userID int64) (MockSettings, bool, error)`, `SaveSettings(ctx, userID int64, in MockSettings) (MockSettings, error)`, `Availability(ctx, userID int64) ([]slots.Rule, error)`, `ReplaceWeekly(ctx, userID int64, rules []slots.Rule) error`, `AddOverride(ctx, userID int64, rule slots.Rule) (slots.Rule, error)`, `DeleteRule(ctx, userID, ruleID int64) error`
  - unexported: `validateMockSettings(MockSettings) (MockSettings, error)`, `ianaZone(string) (*time.Location, bool)`, `mockSlots(rules []slots.Rule, hostZone *time.Location, busy []slots.Interval, from, to, now time.Time, viewerZone string) (slots.SlotResult, error)`

- [ ] 3.1 Write the failing unit tests `internal/engage/buddy/mock_test.go` (settings validation, and the availability scenarios "Weekly range across a DST change" and "Closed date", which are the engine's answer under the mock shape):

```go
package buddy

import (
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

func TestValidateMockSettings(t *testing.T) {
	got, err := validateMockSettings(MockSettings{
		Timezone:   "Europe/Berlin",
		MeetingURL: "  https://meet.example.test/abc  ",
		Topics:     []string{TopicBehavioral, TopicCoding, TopicCoding},
	})
	if err != nil {
		t.Fatalf("valid settings refused: %v", err)
	}
	if got.MeetingURL != "https://meet.example.test/abc" {
		t.Errorf("link = %q, want it trimmed", got.MeetingURL)
	}
	if want := []string{TopicCoding, TopicBehavioral}; !slices.Equal(got.Topics, want) {
		t.Errorf("topics = %v, want %v (vocabulary order, no duplicates)", got.Topics, want)
	}

	link := "https://meet.example.test/abc"
	for name, in := range map[string]MockSettings{
		"unknown zone":    {Timezone: "Mars/Olympus", MeetingURL: link, Topics: []string{TopicCoding}},
		"server zone":     {Timezone: "Local", MeetingURL: link, Topics: []string{TopicCoding}},
		"abbreviation":    {Timezone: "EST", MeetingURL: link, Topics: []string{TopicCoding}},
		"no zone":         {Timezone: "", MeetingURL: link, Topics: []string{TopicCoding}},
		"plain http link": {Timezone: "UTC", MeetingURL: "http://meet.example.test/abc", Topics: []string{TopicCoding}},
		"no host":         {Timezone: "UTC", MeetingURL: "https://", Topics: []string{TopicCoding}},
		"too long":        {Timezone: "UTC", MeetingURL: "https://meet.example.test/" + strings.Repeat("a", 500), Topics: []string{TopicCoding}},
		"no topics":       {Timezone: "UTC", MeetingURL: link},
		"unknown topic":   {Timezone: "UTC", MeetingURL: link, Topics: []string{TopicCoding, "karaoke"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := validateMockSettings(in); !errors.Is(err, ErrInvalidMockSettings) {
				t.Fatalf("err = %v, want ErrInvalidMockSettings", err)
			}
		})
	}
}

func weeklyRule(t *testing.T, day time.Weekday, fromHour, toHour int) slots.Rule {
	t.Helper()
	start, err := slots.NewTimeOfDay(fromHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	end, err := slots.NewTimeOfDay(toHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := slots.NewWeeklyRule(day, start, end)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

// Tuesdays 19:00-21:00 in Berlin, listed by a viewer in New York from 15 October to
// 5 November 2026: Berlin leaves summer time on 25 October, New York on 1 November, and
// every slot must still begin at 19:00 or 20:00 on the HOST's wall clock.
func TestMockSlotsKeepTheHostsWallClockAcrossDST(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC)

	res, err := mockSlots([]slots.Rule{weeklyRule(t, time.Tuesday, 19, 21)}, berlin, nil, now, to, now, "America/New_York")
	if err != nil {
		t.Fatal(err)
	}
	if res.Zone != "America/New_York" {
		t.Errorf("zone = %q, want the viewer's", res.Zone)
	}
	var days []string
	for _, s := range res.Slots {
		local := s.Start.In(berlin)
		if h := local.Hour(); (h != 19 && h != 20) || local.Minute() != 0 {
			t.Errorf("slot %v starts at %02d:%02d in Berlin", s.Start, h, local.Minute())
		}
		if s.End.Sub(s.Start) != MockDuration {
			t.Errorf("slot %v lasts %v", s.Start, s.End.Sub(s.Start))
		}
		days = append(days, local.Format(time.DateOnly))
	}
	want := []string{"2026-10-20", "2026-10-20", "2026-10-27", "2026-10-27", "2026-11-03", "2026-11-03"}
	if !slices.Equal(days, want) {
		t.Errorf("slot days = %v, want %v", days, want)
	}
}

func TestMockSlotsOfferNothingOnAClosedDate(t *testing.T) {
	berlin, err := time.LoadLocation("Europe/Berlin")
	if err != nil {
		t.Fatal(err)
	}
	day, err := slots.NewDate(2026, time.October, 27)
	if err != nil {
		t.Fatal(err)
	}
	midnight, err := slots.NewTimeOfDay(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := slots.NewDatedRule(day, midnight, midnight)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, time.October, 15, 12, 0, 0, 0, time.UTC)
	to := time.Date(2026, time.November, 5, 0, 0, 0, 0, time.UTC)

	res, err := mockSlots([]slots.Rule{weeklyRule(t, time.Tuesday, 19, 21), closure}, berlin, nil, now, to, now, "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Slots {
		if s.Start.In(berlin).Format(time.DateOnly) == "2026-10-27" {
			t.Errorf("slot %v offered on the closed date", s.Start)
		}
	}
	if len(res.Slots) != 4 {
		t.Errorf("slots = %d, want 4 (two Tuesdays, two hours each)", len(res.Slots))
	}
}
```

- [ ] 3.2 Run: `go test ./internal/engage/buddy/ -run 'TestValidateMockSettings|TestMockSlots' -count=1`. Expected: FAIL — `undefined: validateMockSettings`, `undefined: MockSettings`, `undefined: mockSlots`, … .

- [ ] 3.3 Create `internal/engage/buddy/mock.go`:

```go
package buddy

import (
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// The shape of every mock interview (design D2). Constants rather than columns: a peer
// mock has one shape, and per-member tuning is a mentor feature.
const (
	// MockDuration is how long a mock runs. The participants swap roles halfway themselves.
	MockDuration = 60 * time.Minute
	// MockMinimumNotice is how far ahead of now the earliest bookable slot begins.
	MockMinimumNotice = 12 * time.Hour
	// MockHorizon is how far ahead of now the latest bookable slot begins.
	MockHorizon = 30 * 24 * time.Hour
	// maxMeetingURLRunes bounds a meeting link, matching buddy_mock_settings_url_check.
	maxMeetingURLRunes = 500
)

// mockSession is that shape as the engine takes it. No buffers: mocks may run back to back.
var mockSession = slots.SessionParams{Duration: MockDuration, MinimumNotice: MockMinimumNotice, Horizon: MockHorizon}

// The topics a host may offer; they match buddy_mock_settings_topics_check and
// buddy_mocks_topic_check.
const (
	TopicCoding       = "coding"
	TopicSystemDesign = "system_design"
	TopicBehavioral   = "behavioral"
)

// MockTopics is the topic vocabulary in display order; saved settings keep this order.
var MockTopics = []string{TopicCoding, TopicSystemDesign, TopicBehavioral}

// The statuses a mock holds. There is no "completed": that is confirmed and ended.
const (
	MockConfirmed = "confirmed"
	MockCancelled = "cancelled"
)

// The two roles a participant holds in one mock.
const (
	RoleHost  = "host"
	RoleGuest = "guest"
)

// Why a mock was cancelled; buddy_mocks.cancel_reason. A leave or a block is told to the
// other side only as "the mock was cancelled".
const (
	CancelByParticipant = "participant"
	CancelByLeave       = "left"
	CancelByBlock       = "blocked"
)

// ErrInvalidMockSettings → 422: a zone that is not an IANA place, a link that is not
// https://, or no known topic.
var ErrInvalidMockSettings = errors.New("buddy: invalid mock settings")

// MockSettings is what a member offers as a mock host.
type MockSettings struct {
	// Timezone is the IANA zone their availability is stated in.
	Timezone string
	// MeetingURL is their own call room. Only the two participants of a confirmed mock
	// ever read it, and they read the current one.
	MeetingURL string
	// Topics is a non-empty subset of MockTopics, in its order.
	Topics []string
	// Paused withdraws every slot and leaves booked mocks standing.
	Paused bool
}

// Mock is one booked mock interview as the service reads it. The two addresses exist to
// address each participant's OWN notice; nothing that reaches the other participant
// carries either of them, or either account id.
type Mock struct {
	ID            uuid.UUID
	HostUserID    int64
	GuestUserID   int64
	HostHandle    string
	GuestHandle   string
	StartsAt      time.Time
	EndsAt        time.Time
	Topic         string
	Status        string
	CancelReason  string
	HostTimezone  string
	GuestTimezone string
	MeetingURL    string
	HostEmail     string
	GuestEmail    string
}

// validateMockSettings refuses settings no slot or notice could be computed from, and
// normalises the rest: the link trimmed, the topics deduplicated into vocabulary order.
func validateMockSettings(in MockSettings) (MockSettings, error) {
	if _, ok := ianaZone(in.Timezone); !ok {
		return MockSettings{}, fmt.Errorf("%w: %q is not an IANA time zone", ErrInvalidMockSettings, in.Timezone)
	}
	link := strings.TrimSpace(in.MeetingURL)
	parsed, err := url.Parse(link)
	if !strings.HasPrefix(link, "https://") || err != nil || parsed.Host == "" ||
		utf8.RuneCountInString(link) > maxMeetingURLRunes {
		return MockSettings{}, fmt.Errorf("%w: the meeting link must be an https:// address", ErrInvalidMockSettings)
	}
	for _, t := range in.Topics {
		if !slices.Contains(MockTopics, t) {
			return MockSettings{}, fmt.Errorf("%w: unknown topic %q", ErrInvalidMockSettings, t)
		}
	}
	topics := make([]string, 0, len(MockTopics))
	for _, t := range MockTopics {
		if slices.Contains(in.Topics, t) {
			topics = append(topics, t)
		}
	}
	if len(topics) == 0 {
		return MockSettings{}, fmt.Errorf("%w: choose at least one topic", ErrInvalidMockSettings)
	}
	return MockSettings{Timezone: in.Timezone, MeetingURL: link, Topics: topics, Paused: in.Paused}, nil
}

// ianaZone loads an IANA zone name, refusing what time.LoadLocation accepts but is not a
// place: "Local" is the SERVER's zone, and "EST" or "Factory" are not where anybody lives.
// Requiring a slash, or exactly "UTC", is the rule mentorship's validateMentorZone states.
func ianaZone(name string) (*time.Location, bool) {
	if name != "UTC" && !strings.Contains(name, "/") {
		return nil, false
	}
	loc, err := time.LoadLocation(name)
	if err != nil {
		return nil, false
	}
	return loc, true
}

// mockSlots runs the shared engine with the mock shape.
func mockSlots(rules []slots.Rule, hostZone *time.Location, busy []slots.Interval, from, to, now time.Time, viewerZone string) (slots.SlotResult, error) {
	return slots.Slots(slots.SlotRequest{
		Rules:      rules,
		OwnerZone:  hostZone,
		Params:     mockSession,
		Busy:       busy,
		From:       from,
		To:         to,
		Now:        now,
		ViewerZone: viewerZone,
	})
}
```

- [ ] 3.4 Run: `go test ./internal/engage/buddy/ -run 'TestValidateMockSettings|TestMockSlots' -count=1`. Expected: `ok`.

- [ ] 3.5 Write the failing integration tests. First the shared helpers, `internal/engage/buddy/mock_helpers_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
)

// recordingNotifier is a MockNotifier that keeps what it was asked to send. failReminders
// makes that many reminder deliveries fail first.
type recordingNotifier struct {
	mu            sync.Mutex
	booked        []buddy.Mock
	cancelled     []buddy.Mock
	reminded      []string
	failReminders int
}

func (n *recordingNotifier) MockBooked(_ context.Context, m buddy.Mock) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.booked = append(n.booked, m)
	return nil
}

func (n *recordingNotifier) MockCancelled(_ context.Context, m buddy.Mock, _ int64) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.cancelled = append(n.cancelled, m)
	return nil
}

func (n *recordingNotifier) MockReminder(_ context.Context, m buddy.Mock, before time.Duration) error {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.failReminders > 0 {
		n.failReminders--
		return errors.New("mail server down")
	}
	n.reminded = append(n.reminded, fmt.Sprintf("%s:%v", m.ID, before))
	return nil
}

// mockSetup is setup plus a MockService over the same database with a recording notifier.
func mockSetup(t *testing.T) (*pgxpool.Pool, *buddy.Service, *buddy.MockService, *recordingNotifier) {
	t.Helper()
	pool, svc := setup(t)
	n := &recordingNotifier{}
	return pool, svc, buddy.NewMockService(svc, buddy.NewQueriesRepository(db.New(pool)), n), n
}

// joined seeds an account, joins it, and returns its id and handle.
func joined(t *testing.T, pool *pgxpool.Pool, svc *buddy.Service, email string) (int64, string) {
	t.Helper()
	id := seedUser(t, pool, email)
	m, err := svc.Join(context.Background(), id)
	if err != nil {
		t.Fatalf("join %s: %v", email, err)
	}
	return id, m.Handle
}

func weekly(t *testing.T, day time.Weekday, fromHour, toHour int) slots.Rule {
	t.Helper()
	start, err := slots.NewTimeOfDay(fromHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	end, err := slots.NewTimeOfDay(toHour, 0)
	if err != nil {
		t.Fatal(err)
	}
	rule, err := slots.NewWeeklyRule(day, start, end)
	if err != nil {
		t.Fatal(err)
	}
	return rule
}

// hostLink is the meeting link openAllWeek saves for a member.
func hostLink(userID int64) string {
	return "https://meet.example.test/room-" + strconv.FormatInt(userID, 10)
}

// openAllWeek saves settings in zone and offers every day 00:00-24:00, so a slot exists at
// every whole hour past the notice period. topics defaults to the whole vocabulary.
func openAllWeek(t *testing.T, mocks *buddy.MockService, userID int64, zone string, topics ...string) {
	t.Helper()
	ctx := context.Background()
	if len(topics) == 0 {
		topics = buddy.MockTopics
	}
	if _, err := mocks.SaveSettings(ctx, userID, buddy.MockSettings{
		Timezone: zone, MeetingURL: hostLink(userID), Topics: topics,
	}); err != nil {
		t.Fatalf("save settings: %v", err)
	}
	week := make([]slots.Rule, 0, 7)
	for d := time.Sunday; d <= time.Saturday; d++ {
		week = append(week, weekly(t, d, 0, 24))
	}
	if err := mocks.ReplaceWeekly(ctx, userID, week); err != nil {
		t.Fatalf("replace week: %v", err)
	}
}
```

Then `internal/engage/buddy/mock_settings_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

func TestMockSettingsRefuseAnUnknownZoneAndStoreNothing(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	u, _ := joined(t, pool, svc, "a@example.test")

	_, err := mocks.SaveSettings(ctx, u, buddy.MockSettings{
		Timezone: "Mars/Olympus", MeetingURL: "https://meet.example.test/a", Topics: []string{buddy.TopicCoding},
	})
	if !errors.Is(err, buddy.ErrInvalidMockSettings) {
		t.Fatalf("err = %v, want ErrInvalidMockSettings", err)
	}
	if _, ok, err := mocks.Settings(ctx, u); err != nil || ok {
		t.Fatalf("settings stored after a refusal: ok=%v err=%v", ok, err)
	}
}

func TestMockSettingsRefuseAPlainHTTPLink(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	u, _ := joined(t, pool, svc, "a@example.test")

	_, err := mocks.SaveSettings(context.Background(), u, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: "http://meet.example.test/a", Topics: []string{buddy.TopicCoding},
	})
	if !errors.Is(err, buddy.ErrInvalidMockSettings) {
		t.Fatalf("err = %v, want ErrInvalidMockSettings", err)
	}
}

func TestMockSettingsRoundTrip(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	u, _ := joined(t, pool, svc, "a@example.test")

	if _, err := mocks.SaveSettings(ctx, u, buddy.MockSettings{
		Timezone: "Europe/Berlin", MeetingURL: "https://meet.example.test/a",
		Topics: []string{buddy.TopicBehavioral, buddy.TopicCoding}, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}
	got, ok, err := mocks.Settings(ctx, u)
	if err != nil || !ok {
		t.Fatalf("settings = %v, %v", ok, err)
	}
	if got.Timezone != "Europe/Berlin" || !got.Paused ||
		!slices.Equal(got.Topics, []string{buddy.TopicCoding, buddy.TopicBehavioral}) {
		t.Fatalf("settings = %+v", got)
	}
}

func TestMockSettingsNeedAnActiveMember(t *testing.T) {
	pool, _, mocks, _ := mockSetup(t)
	outsider := seedUser(t, pool, "outsider@example.test")

	_, err := mocks.SaveSettings(context.Background(), outsider, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: "https://meet.example.test/a", Topics: []string{buddy.TopicCoding},
	})
	if !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("err = %v, want ErrNotMember", err)
	}
}

func TestMockAvailabilityWeekAndOverrides(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	u, _ := joined(t, pool, svc, "a@example.test")
	other, _ := joined(t, pool, svc, "b@example.test")

	evening := weekly(t, time.Tuesday, 19, 21)
	morning := weekly(t, time.Saturday, 9, 12)
	if err := mocks.ReplaceWeekly(ctx, u, []slots.Rule{evening, morning}); err != nil {
		t.Fatal(err)
	}
	day, err := slots.NewDate(2026, time.December, 1)
	if err != nil {
		t.Fatal(err)
	}
	midnight, err := slots.NewTimeOfDay(0, 0)
	if err != nil {
		t.Fatal(err)
	}
	closure, err := slots.NewDatedRule(day, midnight, midnight)
	if err != nil {
		t.Fatal(err)
	}
	stored, err := mocks.AddOverride(ctx, u, closure)
	if err != nil || stored.ID() == 0 {
		t.Fatalf("add override = %+v, %v; want a stored row id", stored, err)
	}

	// Replacing the week leaves the override alone.
	if err := mocks.ReplaceWeekly(ctx, u, []slots.Rule{evening}); err != nil {
		t.Fatal(err)
	}
	rules, err := mocks.Availability(ctx, u)
	if err != nil || len(rules) != 2 {
		t.Fatalf("availability = %d rules, %v; want the evening and the override", len(rules), err)
	}

	if err := mocks.DeleteRule(ctx, other, stored.ID()); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("deleting another member's rule: err = %v, want ErrNotFound", err)
	}
	if err := mocks.DeleteRule(ctx, u, stored.ID()); err != nil {
		t.Fatal(err)
	}
	if err := mocks.ReplaceWeekly(ctx, u, []slots.Rule{closure}); !errors.Is(err, slots.ErrInvalidSpan) {
		t.Fatalf("a dated rule in the week: err = %v, want ErrInvalidSpan", err)
	}
	if _, err := mocks.AddOverride(ctx, u, evening); !errors.Is(err, slots.ErrInvalidSpan) {
		t.Fatalf("a weekly rule as an override: err = %v, want ErrInvalidSpan", err)
	}
}
```

- [ ] 3.6 Run: `go vet -tags=integration ./internal/engage/buddy/`. Expected: FAIL — `undefined: buddy.NewMockService`, `undefined: buddy.MockService`, … .

- [ ] 3.7 Create `internal/engage/buddy/mock_service.go`:

```go
package buddy

import (
	"context"
	"fmt"
	"time"

	"github.com/strelov1/freehire/internal/engage/slots"
)

// MockNotifier delivers what a mock produces. Every message names the other participant
// by buddy handle only and renders the time in its recipient's own zone.
type MockNotifier interface {
	// MockBooked tells both participants.
	MockBooked(ctx context.Context, m Mock) error
	// MockCancelled tells the participant who did not cause it; actorID is who did.
	MockCancelled(ctx context.Context, m Mock, actorID int64) error
	// MockReminder tells both participants the mock starts in `before`.
	MockReminder(ctx context.Context, m Mock, before time.Duration) error
}

// MockRepository is the storage the MockService needs.
type MockRepository interface {
	// MockSettings is a member's saved settings; ok is false when they never saved any.
	MockSettings(ctx context.Context, userID int64) (s MockSettings, ok bool, err error)
	SaveMockSettings(ctx context.Context, userID int64, s MockSettings) (MockSettings, error)
	// MockAvailability is every rule of a member, weekly and dated, each with its row id.
	MockAvailability(ctx context.Context, userID int64) ([]slots.Rule, error)
	// ReplaceMockWeekly swaps the weekly rules in one statement, leaving overrides alone.
	ReplaceMockWeekly(ctx context.Context, userID int64, rules []slots.Rule) error
	// AddMockRule stores one rule and returns it labelled with its row id.
	AddMockRule(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error)
	// DeleteMockRule reports false when the rule is not userID's.
	DeleteMockRule(ctx context.Context, userID, ruleID int64) (bool, error)
}

// MockService is the mock-interview use cases between buddy members: settings,
// availability, the slot listing, booking, cancellation, review and reminders. Membership,
// handles and blocks are the buddy Service's; this asks it rather than keeping a copy.
type MockService struct {
	members  *Service
	repo     MockRepository
	notifier MockNotifier
	now      func() time.Time
}

// NewMockService builds a MockService. A nil notifier sends nothing and claims nothing.
func NewMockService(members *Service, repo MockRepository, notifier MockNotifier) *MockService {
	return &MockService{members: members, repo: repo, notifier: notifier, now: time.Now}
}

// Settings is the caller's own settings; ok is false before the first save.
func (s *MockService) Settings(ctx context.Context, userID int64) (MockSettings, bool, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return MockSettings{}, false, err
	}
	return s.repo.MockSettings(ctx, userID)
}

// SaveSettings validates and stores the caller's settings whole.
func (s *MockService) SaveSettings(ctx context.Context, userID int64, in MockSettings) (MockSettings, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return MockSettings{}, err
	}
	valid, err := validateMockSettings(in)
	if err != nil {
		return MockSettings{}, err
	}
	return s.repo.SaveMockSettings(ctx, userID, valid)
}

// Availability is the caller's whole schedule, both shapes together.
func (s *MockService) Availability(ctx context.Context, userID int64) ([]slots.Rule, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return nil, err
	}
	return s.repo.MockAvailability(ctx, userID)
}

// ReplaceWeekly swaps the caller's recurring week; a dated rule is not part of a week.
func (s *MockService) ReplaceWeekly(ctx context.Context, userID int64, rules []slots.Rule) error {
	for _, r := range rules {
		if r.IsDated() {
			return fmt.Errorf("%w: the weekly schedule takes weekday rules", slots.ErrInvalidSpan)
		}
	}
	if err := s.members.requireActive(ctx, userID); err != nil {
		return err
	}
	return s.repo.ReplaceMockWeekly(ctx, userID, rules)
}

// AddOverride adds one dated rule: a narrowed day, an extra day, or a closure.
func (s *MockService) AddOverride(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error) {
	if !rule.IsDated() {
		return slots.Rule{}, fmt.Errorf("%w: an override names a date", slots.ErrInvalidSpan)
	}
	if err := s.members.requireActive(ctx, userID); err != nil {
		return slots.Rule{}, err
	}
	return s.repo.AddMockRule(ctx, userID, rule)
}

// DeleteRule removes one row of the caller's own schedule; anyone else's is ErrNotFound.
func (s *MockService) DeleteRule(ctx context.Context, userID, ruleID int64) error {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return err
	}
	ok, err := s.repo.DeleteMockRule(ctx, userID, ruleID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrNotFound
	}
	return nil
}
```

- [ ] 3.8 Create `internal/engage/buddy/mock_repository.go`:

```go
package buddy

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/slots"
	"github.com/strelov1/freehire/internal/platform/db"
)

var _ MockRepository = (*QueriesRepository)(nil)

func (r *QueriesRepository) MockSettings(ctx context.Context, userID int64) (MockSettings, bool, error) {
	row, err := r.q.GetBuddyMockSettings(ctx, userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return MockSettings{}, false, nil
	}
	if err != nil {
		return MockSettings{}, false, err
	}
	return mockSettingsFromRow(row), true, nil
}

func (r *QueriesRepository) SaveMockSettings(ctx context.Context, userID int64, s MockSettings) (MockSettings, error) {
	row, err := r.q.UpsertBuddyMockSettings(ctx, db.UpsertBuddyMockSettingsParams{
		UserID: userID, Timezone: s.Timezone, MeetingUrl: s.MeetingURL, Topics: s.Topics, Paused: s.Paused,
	})
	if err != nil {
		return MockSettings{}, err
	}
	return mockSettingsFromRow(row), nil
}

func mockSettingsFromRow(row db.BuddyMockSetting) MockSettings {
	return MockSettings{Timezone: row.Timezone, MeetingURL: row.MeetingUrl, Topics: row.Topics, Paused: row.Paused}
}

// MockAvailability skips a row the domain cannot represent rather than failing the read:
// one corrupt row must not make a member unbookable, and the CHECKs stop one existing.
func (r *QueriesRepository) MockAvailability(ctx context.Context, userID int64) ([]slots.Rule, error) {
	rows, err := r.q.ListBuddyMockAvailability(ctx, userID)
	if err != nil {
		return nil, err
	}
	out := make([]slots.Rule, 0, len(rows))
	for _, row := range rows {
		if rule, ok := mockRuleFromRow(row); ok {
			out = append(out, rule.WithID(row.ID))
		}
	}
	return out, nil
}

func (r *QueriesRepository) ReplaceMockWeekly(ctx context.Context, userID int64, rules []slots.Rule) error {
	p := db.ReplaceBuddyMockWeeklyParams{
		UserID: userID, Weekdays: []int16{}, StartTimes: []pgtype.Time{}, EndTimes: []pgtype.Time{},
	}
	for _, rule := range rules {
		p.Weekdays = append(p.Weekdays, int16(rule.Weekday()))
		p.StartTimes = append(p.StartTimes, pgClock(rule.Start()))
		p.EndTimes = append(p.EndTimes, pgClock(rule.End()))
	}
	return r.q.ReplaceBuddyMockWeekly(ctx, p)
}

func (r *QueriesRepository) AddMockRule(ctx context.Context, userID int64, rule slots.Rule) (slots.Rule, error) {
	p := db.CreateBuddyMockAvailabilityRuleParams{
		UserID: userID, StartTime: pgClock(rule.Start()), EndTime: pgClock(rule.End()),
	}
	if rule.IsDated() {
		year, month, day := rule.Date().Parts()
		p.OnDate = pgtype.Date{Time: time.Date(year, month, day, 0, 0, 0, 0, time.UTC), Valid: true}
	} else {
		p.Weekday = pgtype.Int2{Int16: int16(rule.Weekday()), Valid: true}
	}
	row, err := r.q.CreateBuddyMockAvailabilityRule(ctx, p)
	if err != nil {
		return slots.Rule{}, err
	}
	return rule.WithID(row.ID), nil
}

func (r *QueriesRepository) DeleteMockRule(ctx context.Context, userID, ruleID int64) (bool, error) {
	n, err := r.q.DeleteBuddyMockAvailabilityRule(ctx, db.DeleteBuddyMockAvailabilityRuleParams{ID: ruleID, UserID: userID})
	return n == 1, err
}

// mockRuleFromRow rebuilds a domain rule, reporting false for a row that is neither shape.
// mentorship keeps the same conversion for mentor_availability; the two tables are
// different sqlc types, so each adapter converts its own.
func mockRuleFromRow(row db.BuddyMockAvailability) (slots.Rule, bool) {
	start, startOK := clockFromPg(row.StartTime)
	end, endOK := clockFromPg(row.EndTime)
	if !startOK || !endOK {
		return slots.Rule{}, false
	}
	if row.OnDate.Valid {
		date, err := slots.NewDate(row.OnDate.Time.Year(), row.OnDate.Time.Month(), row.OnDate.Time.Day())
		if err != nil {
			return slots.Rule{}, false
		}
		rule, err := slots.NewDatedRule(date, start, end)
		return rule, err == nil
	}
	if !row.Weekday.Valid {
		return slots.Rule{}, false
	}
	rule, err := slots.NewWeeklyRule(time.Weekday(row.Weekday.Int16), start, end)
	return rule, err == nil
}

// pgClock renders a wall-clock time as microseconds since midnight; 24:00 is a valid
// Postgres time and how a rule reaches the end of its day.
func pgClock(t slots.TimeOfDay) pgtype.Time {
	return pgtype.Time{Microseconds: int64(t.Minutes()) * 60 * 1_000_000, Valid: true}
}

// clockFromPg reads one back, refusing a value finer than a minute rather than rounding it.
func clockFromPg(t pgtype.Time) (slots.TimeOfDay, bool) {
	const microsPerMinute = 60 * 1_000_000
	if !t.Valid || t.Microseconds%microsPerMinute != 0 {
		return slots.TimeOfDay{}, false
	}
	minutes := int(t.Microseconds / microsPerMinute)
	got, err := slots.NewTimeOfDay(minutes/60, minutes%60)
	return got, err == nil
}
```

- [ ] 3.9 Run: `go test ./internal/engage/buddy/ -count=1 && go test -tags=integration ./internal/engage/buddy/ -run 'TestMockSettings|TestMockAvailability' -count=1`. Expected: `ok` for both.

- [ ] 3.10 Verify and commit:

```bash
gofmt -l internal/engage/buddy && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock.go internal/engage/buddy/mock_service.go internal/engage/buddy/mock_repository.go internal/engage/buddy/mock_test.go internal/engage/buddy/mock_helpers_integration_test.go internal/engage/buddy/mock_settings_integration_test.go
git commit -m "Add buddy mock settings and availability"
```

## 4. Slot listing

**Files:**
- Modify: `internal/engage/buddy/mock_service.go` (interface + `OpenSlots`, `hostSlots`), `internal/engage/buddy/mock_repository.go` (`MockBusy`), `internal/engage/buddy/mock_helpers_integration_test.go` (`firstSlot`, `seedMock`)
- Create: `internal/engage/buddy/mock_slots_integration_test.go`

**Interfaces:**
- Consumes: `(*Service).visibleMember(ctx, viewerID int64, handle string) (Member, error)` (`buddy.go`; `ErrNotMember` for an inactive viewer, `ErrNotFound` for an unknown/left/self/blocked handle); `db.Queries.ListBuddyMockBusy`; `mockSlots`, `ianaZone`; `slots.ErrNoOwnerZone`.
- Produces: `MockRepository.MockBusy(ctx, userID int64, from, to time.Time) ([]slots.Interval, error)`; `type MockSlots struct{ Slots []slots.Interval; Zone string; Topics []string }`; `func (*MockService) OpenSlots(ctx, viewerID int64, hostHandle string, from, to time.Time, viewerZone string) (MockSlots, error)`; unexported `(*MockService).hostSlots(ctx, hostID int64, settings MockSettings, from, to time.Time, viewerZone string) (slots.SlotResult, error)`.

- [ ] 4.1 Append the two fixtures the next tasks share to `internal/engage/buddy/mock_helpers_integration_test.go` (add `"github.com/google/uuid"` and `"github.com/jackc/pgx/v5/pgtype"` to its imports):

```go
// firstSlot is the earliest slot hostHandle offers viewerID in the next three days, in UTC.
func firstSlot(t *testing.T, mocks *buddy.MockService, viewerID int64, hostHandle string) time.Time {
	t.Helper()
	now := time.Now()
	res, err := mocks.OpenSlots(context.Background(), viewerID, hostHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatalf("open slots: %v", err)
	}
	if len(res.Slots) == 0 {
		t.Fatal("the host offers no slot")
	}
	return res.Slots[0].Start.UTC()
}

// seedMock writes a confirmed one-hour mock and both participant rows directly, for states
// a booking cannot reach: a mock already under way or long over, or one in the past.
func seedMock(t *testing.T, pool *pgxpool.Pool, host, guest int64, start time.Time) uuid.UUID {
	t.Helper()
	var id pgtype.UUID
	if err := pool.QueryRow(context.Background(), `
		WITH m AS (
			INSERT INTO buddy_mocks (host_user_id, guest_user_id, starts_at, ends_at, topic, guest_timezone)
			VALUES ($1, $2, $3, $3::timestamptz + interval '1 hour', 'coding', 'UTC')
			RETURNING id, starts_at, ends_at
		), p AS (
			INSERT INTO buddy_mock_participants (mock_id, user_id, starts_at, ends_at)
			SELECT id, $1, starts_at, ends_at FROM m
			UNION ALL
			SELECT id, $2, starts_at, ends_at FROM m
		)
		SELECT id FROM m`, host, guest, start).Scan(&id); err != nil {
		t.Fatalf("seed mock: %v", err)
	}
	return uuid.UUID(id.Bytes)
}
```

- [ ] 4.2 Write the failing tests `internal/engage/buddy/mock_slots_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

func TestMockSlotsAreListedInTheViewersZone(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	openAllWeek(t, mocks, host, "Europe/Berlin", buddy.TopicCoding, buddy.TopicBehavioral)

	now := time.Now()
	res, err := mocks.OpenSlots(ctx, viewer, hostHandle, now, now.AddDate(0, 0, 40), "Asia/Tokyo")
	if err != nil {
		t.Fatal(err)
	}
	if res.Zone != "Asia/Tokyo" || len(res.Slots) == 0 {
		t.Fatalf("zone %q, %d slots", res.Zone, len(res.Slots))
	}
	for _, s := range res.Slots {
		if s.Start.Before(now.Add(buddy.MockMinimumNotice)) || s.Start.After(now.Add(buddy.MockHorizon + time.Minute)) {
			t.Fatalf("slot %v outside the notice period and the horizon", s.Start)
		}
		if s.Start.Location().String() != "Asia/Tokyo" {
			t.Fatalf("slot %v not expressed in the viewer's zone", s.Start)
		}
	}
	if len(res.Topics) != 2 || res.Topics[0] != buddy.TopicCoding || res.Topics[1] != buddy.TopicBehavioral {
		t.Fatalf("topics = %v", res.Topics)
	}
}

func TestPausedOrUnsetHostOffersNoSlots(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	_, unsetHandle := joined(t, pool, svc, "unset@example.test")
	paused, pausedHandle := joined(t, pool, svc, "paused@example.test")
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	openAllWeek(t, mocks, paused, "UTC")
	if _, err := mocks.SaveSettings(ctx, paused, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(paused), Topics: buddy.MockTopics, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	for _, handle := range []string{unsetHandle, pausedHandle} {
		res, err := mocks.OpenSlots(ctx, viewer, handle, now, now.AddDate(0, 0, 7), "UTC")
		if err != nil {
			t.Fatal(err)
		}
		if len(res.Slots) != 0 || len(res.Topics) != 0 || res.Slots == nil {
			t.Fatalf("%s offers %d slots and topics %v; want an empty, non-nil list", handle, len(res.Slots), res.Topics)
		}
	}
}

func TestBeingAGuestHidesTheHostsOwnSlot(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	h, hHandle := joined(t, pool, svc, "h@example.test")
	other, _ := joined(t, pool, svc, "other@example.test")
	viewer, _ := joined(t, pool, svc, "viewer@example.test")
	openAllWeek(t, mocks, h, "UTC")

	at := firstSlot(t, mocks, viewer, hHandle)
	seedMock(t, pool, other, h, at) // h is the GUEST of someone else's mock at that hour

	now := time.Now()
	res, err := mocks.OpenSlots(ctx, viewer, hHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Slots {
		if s.Start.Equal(at) {
			t.Fatalf("h's %v slot is offered while h is a guest in another mock", at)
		}
	}
}

func TestSlotListingRefusesStrangersAndBlocks(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	blocker, _ := joined(t, pool, svc, "blocker@example.test")
	blocked, _ := joined(t, pool, svc, "blocked@example.test")
	outsider := seedUser(t, pool, "outsider@example.test")
	openAllWeek(t, mocks, host, "UTC")
	now := time.Now()

	if _, err := mocks.OpenSlots(ctx, outsider, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); !errors.Is(err, buddy.ErrNotMember) {
		t.Fatalf("non-member: err = %v, want ErrNotMember", err)
	}
	for _, stmt := range [][2]int64{{blocker, host}, {host, blocked}} {
		if _, err := pool.Exec(ctx, `INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, stmt[0], stmt[1]); err != nil {
			t.Fatal(err)
		}
	}
	for _, viewer := range []int64{blocker, blocked} {
		if _, err := mocks.OpenSlots(ctx, viewer, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); !errors.Is(err, buddy.ErrNotFound) {
			t.Fatalf("viewer %d across a block: err = %v, want ErrNotFound", viewer, err)
		}
	}
	if _, err := mocks.OpenSlots(ctx, host, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); !errors.Is(err, buddy.ErrNotFound) {
		t.Fatalf("own handle: err = %v, want ErrNotFound", err)
	}
}
```

- [ ] 4.3 Run: `go vet -tags=integration ./internal/engage/buddy/`. Expected: FAIL — `mocks.OpenSlots undefined`.

- [ ] 4.4 Implement. In `internal/engage/buddy/mock_service.go` add to `MockRepository`, after `DeleteMockRule`:

```go
	// MockBusy is the member's occupied time in a window: every active participant row of
	// theirs, as host or as guest.
	MockBusy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error)
```
and append to the file:

```go
// MockSlots is what one member may book of another: the open slots in the viewer's zone
// and the host's topics. The meeting link is deliberately absent — it reaches a guest only
// through a booked mock.
type MockSlots struct {
	Slots  []slots.Interval
	Zone   string
	Topics []string
}

// OpenSlots lists hostHandle's bookable slots for an active viewer, in viewerZone (an
// unknown name answers in UTC and says so in Zone). A host with no settings or paused
// offers nothing; an unknown, departed or blocked host, and the viewer's own handle, are
// ErrNotFound.
func (s *MockService) OpenSlots(ctx context.Context, viewerID int64, hostHandle string, from, to time.Time, viewerZone string) (MockSlots, error) {
	host, err := s.members.visibleMember(ctx, viewerID, hostHandle)
	if err != nil {
		return MockSlots{}, err
	}
	settings, ok, err := s.repo.MockSettings(ctx, host.UserID)
	if err != nil {
		return MockSlots{}, err
	}
	if !ok || settings.Paused {
		// No schedule: the engine over no rules still resolves the viewer's zone, so the
		// empty answer says which zone it would have been in.
		res, err := mockSlots(nil, time.UTC, nil, from, to, s.now(), viewerZone)
		return MockSlots{Slots: res.Slots, Zone: res.Zone, Topics: []string{}}, err
	}
	res, err := s.hostSlots(ctx, host.UserID, settings, from, to, viewerZone)
	if err != nil {
		return MockSlots{}, err
	}
	return MockSlots{Slots: res.Slots, Zone: res.Zone, Topics: settings.Topics}, nil
}

// hostSlots runs the engine over a host's schedule and busy time. Busy is read a day wider
// than the window on both sides, as mentorship's slotsFor does.
func (s *MockService) hostSlots(ctx context.Context, hostID int64, settings MockSettings, from, to time.Time, viewerZone string) (slots.SlotResult, error) {
	zone, ok := ianaZone(settings.Timezone)
	if !ok {
		return slots.SlotResult{}, fmt.Errorf("%w: member %d has timezone %q", slots.ErrNoOwnerZone, hostID, settings.Timezone)
	}
	rules, err := s.repo.MockAvailability(ctx, hostID)
	if err != nil {
		return slots.SlotResult{}, err
	}
	busy, err := s.repo.MockBusy(ctx, hostID, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return slots.SlotResult{}, err
	}
	return mockSlots(rules, zone, busy, from, to, s.now(), viewerZone)
}
```
In `internal/engage/buddy/mock_repository.go` append:

```go
func (r *QueriesRepository) MockBusy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error) {
	rows, err := r.q.ListBuddyMockBusy(ctx, db.ListBuddyMockBusyParams{
		UserID: userID, WindowStart: pgInstant(from), WindowEnd: pgInstant(to),
	})
	if err != nil {
		return nil, err
	}
	out := make([]slots.Interval, 0, len(rows))
	for _, row := range rows {
		out = append(out, slots.Interval{Start: row.StartsAt.Time, End: row.EndsAt.Time})
	}
	return out, nil
}

func pgInstant(t time.Time) pgtype.Timestamptz { return pgtype.Timestamptz{Time: t, Valid: true} }
```

- [ ] 4.5 Run: `go test -tags=integration ./internal/engage/buddy/ -run 'TestMockSlots|TestPausedOrUnset|TestBeingAGuest|TestSlotListing' -count=1`. Expected: `ok`.

- [ ] 4.6 Verify and commit:

```bash
gofmt -l internal/engage/buddy && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock_service.go internal/engage/buddy/mock_repository.go internal/engage/buddy/mock_helpers_integration_test.go internal/engage/buddy/mock_slots_integration_test.go
git commit -m "List another buddy's open mock slots"
```

## 5. Booking

**Files:**
- Modify: `internal/engage/buddy/mock.go` (sentinels), `internal/engage/buddy/mock_service.go` (interface, `BookInput`, `Book`, `offerable`), `internal/engage/buddy/mock_repository.go` (`FutureGuestMocks`, `CreateMock`, `MockByID`)
- Create: `internal/engage/buddy/mock_booking_integration_test.go`

**Interfaces:**
- Consumes: `db.Queries.CountFutureBuddyMocksAsGuest`, `CreateBuddyMock`, `GetBuddyMock`; `pgerr.ExclusionViolationConstraint(err) (name string, ok bool)` (`internal/platform/pgerr/pgerr.go:93`, the helper mentorship's `CreateBooking` uses); `hostSlots`, `visibleMember`, `MockNotifier.MockBooked`.
- Produces: `const MaxGuestMocks = 3`; `var ErrInvalidTopic, ErrSlotUnavailable, ErrGuestMockLimit, ErrMockNotFound error`; `type NewMock struct{ HostUserID, GuestUserID int64; StartsAt, EndsAt time.Time; Topic, GuestTimezone string }`; `type BookInput struct{ HostHandle string; GuestUserID int64; StartsAt time.Time; Topic, GuestZone string }`; `func (*MockService) Book(ctx, BookInput) (Mock, error)`; repository `FutureGuestMocks(ctx, guestID int64) (int, error)`, `CreateMock(ctx, NewMock) (Mock, error)`, `MockByID(ctx, uuid.UUID) (Mock, bool, error)`.

- [ ] 5.1 Write the failing tests `internal/engage/buddy/mock_booking_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

func book(mocks *buddy.MockService, guest int64, hostHandle string, at time.Time, topic string) (buddy.Mock, error) {
	return mocks.Book(context.Background(), buddy.BookInput{
		HostHandle: hostHandle, GuestUserID: guest, StartsAt: at, Topic: topic, GuestZone: "Asia/Tokyo",
	})
}

func TestBookingConfirmsTheMockAndNotifies(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, guestHandle := joined(t, pool, svc, "guest@example.test")
	other, _ := joined(t, pool, svc, "other@example.test")
	openAllWeek(t, mocks, host, "UTC")

	at := firstSlot(t, mocks, guest, hostHandle)
	m, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	if m.Status != buddy.MockConfirmed || !m.StartsAt.Equal(at) || m.EndsAt.Sub(m.StartsAt) != buddy.MockDuration ||
		m.HostHandle != hostHandle || m.GuestHandle != guestHandle || m.MeetingURL != hostLink(host) ||
		m.GuestTimezone != "Asia/Tokyo" || m.HostTimezone != "UTC" {
		t.Fatalf("booked = %+v", m)
	}
	if len(n.booked) != 1 || n.booked[0].ID != m.ID {
		t.Fatalf("booked notices = %v", n.booked)
	}
	now := time.Now()
	res, err := mocks.OpenSlots(ctx, other, hostHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	for _, s := range res.Slots {
		if s.Start.Equal(at) {
			t.Fatalf("the booked %v slot is still offered", at)
		}
	}
}

func TestConcurrentBookingsOfOneSlotConfirmExactlyOne(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	g1, _ := joined(t, pool, svc, "g1@example.test")
	g2, _ := joined(t, pool, svc, "g2@example.test")
	openAllWeek(t, mocks, host, "UTC")
	at := firstSlot(t, mocks, g1, hostHandle)

	errs := make([]error, 2)
	var wg sync.WaitGroup
	for i, g := range []int64{g1, g2} {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, errs[i] = book(mocks, g, hostHandle, at, buddy.TopicCoding)
		}()
	}
	wg.Wait()

	won, refused := 0, 0
	for _, err := range errs {
		switch {
		case err == nil:
			won++
		case errors.Is(err, buddy.ErrSlotUnavailable):
			refused++
		default:
			t.Fatalf("unexpected error: %v", err)
		}
	}
	if won != 1 || refused != 1 {
		t.Fatalf("won %d, refused %d; want one of each", won, refused)
	}
	var confirmed int
	if err := pool.QueryRow(context.Background(), `SELECT count(*) FROM buddy_mocks WHERE status = 'confirmed'`).Scan(&confirmed); err != nil || confirmed != 1 {
		t.Fatalf("confirmed mocks = %d (err %v), want 1", confirmed, err)
	}
}

func TestAGuestAlreadyHostingIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	third, _ := joined(t, pool, svc, "third@example.test")
	openAllWeek(t, mocks, host, "UTC")

	at := firstSlot(t, mocks, guest, hostHandle)
	seedMock(t, pool, guest, third, at) // the guest hosts their own mock at that hour

	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding); !errors.Is(err, buddy.ErrSlotUnavailable) {
		t.Fatalf("err = %v, want ErrSlotUnavailable", err)
	}
}

func TestATopicTheHostDoesNotOfferIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC", buddy.TopicCoding)

	at := firstSlot(t, mocks, guest, hostHandle)
	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicBehavioral); !errors.Is(err, buddy.ErrInvalidTopic) {
		t.Fatalf("err = %v, want ErrInvalidTopic", err)
	}
}

func TestAFourthFutureMockAsGuestIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	other, _ := joined(t, pool, svc, "other@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")

	base := time.Now().UTC().Truncate(time.Hour).Add(5 * 24 * time.Hour)
	for i := range buddy.MaxGuestMocks {
		seedMock(t, pool, other, guest, base.Add(time.Duration(i)*2*time.Hour))
	}
	at := firstSlot(t, mocks, guest, hostHandle)
	if _, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding); !errors.Is(err, buddy.ErrGuestMockLimit) {
		t.Fatalf("err = %v, want ErrGuestMockLimit", err)
	}
}

func TestPausingKeepsBookedMocksConfirmed(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mocks.SaveSettings(ctx, host, buddy.MockSettings{
		Timezone: "UTC", MeetingURL: hostLink(host), Topics: buddy.MockTopics, Paused: true,
	}); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if res, err := mocks.OpenSlots(ctx, guest, hostHandle, now, now.AddDate(0, 0, 3), "UTC"); err != nil || len(res.Slots) != 0 {
		t.Fatalf("paused host offers %d slots (err %v)", len(res.Slots), err)
	}
	var status string
	if err := pool.QueryRow(ctx, `SELECT status FROM buddy_mocks WHERE id = $1::uuid`, m.ID.String()).Scan(&status); err != nil || status != buddy.MockConfirmed {
		t.Fatalf("booked mock after pause = %q (err %v), want confirmed", status, err)
	}
}
```

- [ ] 5.2 Run: `go vet -tags=integration ./internal/engage/buddy/`. Expected: FAIL — `mocks.Book undefined`, `undefined: buddy.BookInput`, `undefined: buddy.ErrSlotUnavailable`, … .

- [ ] 5.3 Implement. Append to the `const` block of tunables in `internal/engage/buddy/mock.go`, after `maxMeetingURLRunes`:

```go
	// MaxGuestMocks bounds the future confirmed mocks one member may hold as guest: a spam
	// brake on one member reserving everybody's evenings, not a quota.
	MaxGuestMocks = 3
```
and after `ErrInvalidMockSettings`:

```go
// The booking sentinels, each with the status the handler maps it to.
var (
	// ErrInvalidTopic → 422. The host does not offer the topic asked for.
	ErrInvalidTopic = errors.New("buddy: the host does not offer that topic")
	// ErrSlotUnavailable → 409. THE one answer for every ordinary way a booking fails to
	// land — the hour was taken, the schedule changed, the host paused, the notice passed,
	// either person is already in an overlapping mock, or another request won the race —
	// for the reason mentorship's ErrSlotUnavailable gives.
	ErrSlotUnavailable = errors.New("buddy: that time is no longer available")
	// ErrGuestMockLimit → 429. The member already holds MaxGuestMocks future mocks as guest.
	ErrGuestMockLimit = errors.New("buddy: too many upcoming mock interviews as a guest")
	// ErrMockNotFound → 404. No mock, or one the caller is not a participant of.
	ErrMockNotFound = errors.New("buddy: mock interview not found")
)

// NewMock is one mock to write.
type NewMock struct {
	HostUserID    int64
	GuestUserID   int64
	StartsAt      time.Time
	EndsAt        time.Time
	Topic         string
	GuestTimezone string
}
```
In `internal/engage/buddy/mock_service.go` add `"log"`, `"slices"` and `"github.com/google/uuid"` to the imports, extend `MockRepository` after `MockBusy`:

```go
	// FutureGuestMocks counts the member's confirmed, not yet started mocks as guest.
	FutureGuestMocks(ctx context.Context, guestID int64) (int, error)
	// CreateMock writes the mock and both participant rows in one statement. It returns
	// ErrSlotUnavailable when either person is already in an overlapping mock, or when the
	// pair can no longer meet (one left, or a block landed since the service's read).
	CreateMock(ctx context.Context, in NewMock) (Mock, error)
	// MockByID reads one mock with both handles, the host's current zone and link and
	// both addresses. Authorisation is the caller's.
	MockByID(ctx context.Context, id uuid.UUID) (Mock, bool, error)
```
and append:

```go
// BookInput is a request to take one of another member's slots.
type BookInput struct {
	HostHandle  string
	GuestUserID int64
	// StartsAt is the slot's start as the guest saw it. The end is MockDuration later and
	// is never taken from the client.
	StartsAt time.Time
	Topic    string
	// GuestZone is the zone the guest booked in; their notices are rendered in it. An
	// unusable name is stored as UTC, which the notices then state.
	GuestZone string
}

// Book takes one of another member's slots. The slot is re-derived from the host's live
// schedule and busy time rather than trusted from the request; the EXCLUDE on
// buddy_mock_participants, not this re-check, is what wins a race, and losing one is the
// same ErrSlotUnavailable a stale page gets. The booked notice is best-effort.
func (s *MockService) Book(ctx context.Context, in BookInput) (Mock, error) {
	host, err := s.members.visibleMember(ctx, in.GuestUserID, in.HostHandle)
	if err != nil {
		return Mock{}, err
	}
	settings, ok, err := s.repo.MockSettings(ctx, host.UserID)
	if err != nil {
		return Mock{}, err
	}
	if !ok || settings.Paused {
		return Mock{}, ErrSlotUnavailable
	}
	if !slices.Contains(settings.Topics, in.Topic) {
		return Mock{}, ErrInvalidTopic
	}
	held, err := s.repo.FutureGuestMocks(ctx, in.GuestUserID)
	if err != nil {
		return Mock{}, err
	}
	// A count followed by an insert: concurrent bookings by one guest can slip one past the
	// bound, as with the invitation cap. The bound is a brake, the EXCLUDE is what holds.
	if held >= MaxGuestMocks {
		return Mock{}, ErrGuestMockLimit
	}
	offerable, err := s.offerable(ctx, host.UserID, settings, in.StartsAt)
	if err != nil {
		return Mock{}, err
	}
	if !offerable {
		return Mock{}, ErrSlotUnavailable
	}
	guestZone := "UTC"
	if _, ok := ianaZone(in.GuestZone); ok {
		guestZone = in.GuestZone
	}
	mock, err := s.repo.CreateMock(ctx, NewMock{
		HostUserID: host.UserID, GuestUserID: in.GuestUserID,
		StartsAt: in.StartsAt, EndsAt: in.StartsAt.Add(MockDuration),
		Topic: in.Topic, GuestTimezone: guestZone,
	})
	if err != nil {
		return Mock{}, err
	}
	if s.notifier != nil {
		if err := s.notifier.MockBooked(ctx, mock); err != nil {
			log.Printf("buddy: booked notice for mock %s: %v", mock.ID, err)
		}
	}
	return mock, nil
}

// offerable re-derives the host's slots a day either side of start and reports whether
// that exact instant is one of them.
func (s *MockService) offerable(ctx context.Context, hostID int64, settings MockSettings, start time.Time) (bool, error) {
	res, err := s.hostSlots(ctx, hostID, settings, start.AddDate(0, 0, -1), start.AddDate(0, 0, 1), "UTC")
	if err != nil {
		return false, err
	}
	return slices.ContainsFunc(res.Slots, func(iv slots.Interval) bool { return iv.Start.Equal(start) }), nil
}
```
In `internal/engage/buddy/mock_repository.go` add `"github.com/google/uuid"` and `"github.com/strelov1/freehire/internal/platform/pgerr"` to the imports and append:

```go
// constraintMockNoOverlap is the EXCLUDE that keeps one person out of two overlapping mocks
// in either role. Matched by name, so another exclusion could never be answered with this
// one's meaning.
const constraintMockNoOverlap = "buddy_mock_participants_no_overlap"

func (r *QueriesRepository) FutureGuestMocks(ctx context.Context, guestID int64) (int, error) {
	n, err := r.q.CountFutureBuddyMocksAsGuest(ctx, guestID)
	return int(n), err
}

func (r *QueriesRepository) CreateMock(ctx context.Context, in NewMock) (Mock, error) {
	id, err := r.q.CreateBuddyMock(ctx, db.CreateBuddyMockParams{
		HostUserID: in.HostUserID, GuestUserID: in.GuestUserID,
		StartsAt: pgInstant(in.StartsAt), EndsAt: pgInstant(in.EndsAt),
		Topic: in.Topic, GuestTimezone: in.GuestTimezone,
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return Mock{}, ErrSlotUnavailable
	}
	// SQLSTATE 23P01, not a unique violation's 23505 — the lost race, answered as a stale page.
	if name, ok := pgerr.ExclusionViolationConstraint(err); ok && name == constraintMockNoOverlap {
		return Mock{}, ErrSlotUnavailable
	}
	if err != nil {
		return Mock{}, err
	}
	mockID := uuid.UUID(id.Bytes)
	// The insert returns the id only, and the booked notice needs both handles, zones and
	// addresses. A failed re-read falls back to what was written: the mock is committed,
	// and losing its notice beats failing a booking that happened.
	if full, found, err := r.MockByID(ctx, mockID); err == nil && found {
		return full, nil
	}
	return Mock{
		ID: mockID, HostUserID: in.HostUserID, GuestUserID: in.GuestUserID,
		StartsAt: in.StartsAt, EndsAt: in.EndsAt, Topic: in.Topic, Status: MockConfirmed,
		GuestTimezone: in.GuestTimezone,
	}, nil
}

func (r *QueriesRepository) MockByID(ctx context.Context, id uuid.UUID) (Mock, bool, error) {
	row, err := r.q.GetBuddyMock(ctx, pgtype.UUID{Bytes: id, Valid: true})
	if errors.Is(err, pgx.ErrNoRows) {
		return Mock{}, false, nil
	}
	if err != nil {
		return Mock{}, false, err
	}
	return Mock{
		ID: uuid.UUID(row.ID.Bytes), HostUserID: row.HostUserID, GuestUserID: row.GuestUserID,
		HostHandle: row.HostHandle, GuestHandle: row.GuestHandle,
		StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time,
		Topic: row.Topic, Status: row.Status, CancelReason: row.CancelReason.String,
		HostTimezone: row.HostTimezone, GuestTimezone: row.GuestTimezone, MeetingURL: row.MeetingUrl,
		HostEmail: row.HostEmail, GuestEmail: row.GuestEmail,
	}, true, nil
}
```

- [ ] 5.4 Run: `go test -tags=integration ./internal/engage/buddy/ -run 'TestBooking|TestConcurrentBookings|TestAGuestAlready|TestATopic|TestAFourth|TestPausing' -count=1`. Expected: `ok`.

- [ ] 5.5 Verify and commit:

```bash
gofmt -l internal/engage/buddy && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock.go internal/engage/buddy/mock_service.go internal/engage/buddy/mock_repository.go internal/engage/buddy/mock_booking_integration_test.go
git commit -m "Book a buddy mock interview"
```

## 6. Cancellation, leave and block

The leave already cancels inside `LeaveBuddyMember` and the block inside the `user_blocks` trigger (Task 2). The seam left is the NOTICE: a trigger cannot hand rows back, and the block statement is the messaging core's, which may not import buddy. Every cancellation therefore records `cancelled_by`, and the caller announces afterwards with `AnnounceCancellations(actorID)`, which claims the un-announced ones by that actor (`ClaimBuddyMockCancelNotices`). The api layer composes it after the leave and the block (Task 10) — the only layer that sees both the messaging core and buddy. A crash between the write and the announcement loses that notice; notices are best-effort (design D5), and the cancelled mock still shows as cancelled in both lists.

**Files:**
- Modify: `internal/engage/buddy/mock.go` (`ErrMockNotCancellable`), `internal/engage/buddy/mock_service.go` (interface, `Cancel`, `AnnounceCancellations`), `internal/engage/buddy/mock_repository.go` (`CancelMock`, `ClaimCancelNotices`)
- Create: `internal/engage/buddy/mock_cancel_integration_test.go`

**Interfaces:**
- Consumes: `db.Queries.CancelBuddyMock`, `ClaimBuddyMockCancelNotices`; `MockByID`; `MockNotifier.MockCancelled`; `(*Service).Leave`, `(*Service).Invite`, `(*messaging.Service).Block(ctx, userID, conversationID int64) error`.
- Produces: `var ErrMockNotCancellable error`; `func (*MockService) Cancel(ctx, userID int64, id uuid.UUID) error`; `func (*MockService) AnnounceCancellations(ctx, actorID int64)`; repository `CancelMock(ctx, id uuid.UUID, userID int64) (bool, error)`, `ClaimCancelNotices(ctx, actorID int64) ([]Mock, error)`.

- [ ] 6.1 Write the failing tests `internal/engage/buddy/mock_cancel_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
)


// storedMock reads a mock's status and cancel reason straight from the table.
func storedMock(t *testing.T, pool *pgxpool.Pool, id uuid.UUID) (status, reason string) {
	t.Helper()
	if err := pool.QueryRow(context.Background(),
		`SELECT status, coalesce(cancel_reason, '') FROM buddy_mocks WHERE id = $1::uuid`, id.String(),
	).Scan(&status, &reason); err != nil {
		t.Fatalf("read mock: %v", err)
	}
	return status, reason
}

func TestGuestCancelsFreesTheSlotAndTellsTheHost(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	at := firstSlot(t, mocks, guest, hostHandle)
	m, err := book(mocks, guest, hostHandle, at, buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}

	if err := mocks.Cancel(ctx, guest, m.ID); err != nil {
		t.Fatal(err)
	}
	if status, reason := storedMock(t, pool, m.ID); status != buddy.MockCancelled || reason != buddy.CancelByParticipant {
		t.Fatalf("stored = %s/%s", status, reason)
	}
	if len(n.cancelled) != 1 || n.cancelled[0].ID != m.ID || n.cancelled[0].CancelReason != buddy.CancelByParticipant {
		t.Fatalf("cancel notices = %+v", n.cancelled)
	}
	now := time.Now()
	reopened, err := mocks.OpenSlots(ctx, guest, hostHandle, now, now.AddDate(0, 0, 3), "UTC")
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(reopened.Slots, func(s slots.Interval) bool { return s.Start.Equal(at) }) {
		t.Fatalf("the %v slot is not offered again", at)
	}
	// Announcing again finds nothing: each notice is claimed once.
	mocks.AnnounceCancellations(ctx, guest)
	if len(n.cancelled) != 1 {
		t.Fatalf("cancel notices after a second announce = %d", len(n.cancelled))
	}
}

func TestAStartedMockCannotBeCancelled(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-10*time.Minute))

	if err := mocks.Cancel(context.Background(), guest, id); !errors.Is(err, buddy.ErrMockNotCancellable) {
		t.Fatalf("err = %v, want ErrMockNotCancellable", err)
	}
}

func TestAStrangerCannotCancel(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	stranger, _ := joined(t, pool, svc, "stranger@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(48*time.Hour))

	if err := mocks.Cancel(context.Background(), stranger, id); !errors.Is(err, buddy.ErrMockNotFound) {
		t.Fatalf("err = %v, want ErrMockNotFound", err)
	}
}

func TestLeavingCancelsFutureMocksAndTellsTheOtherSide(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.Leave(ctx, host); err != nil {
		t.Fatal(err)
	}
	mocks.AnnounceCancellations(ctx, host)

	if status, reason := storedMock(t, pool, m.ID); status != buddy.MockCancelled || reason != buddy.CancelByLeave {
		t.Fatalf("stored = %s/%s", status, reason)
	}
	if len(n.cancelled) != 1 || n.cancelled[0].ID != m.ID || n.cancelled[0].CancelReason != buddy.CancelByLeave {
		t.Fatalf("cancel notices = %+v", n.cancelled)
	}
}

func TestBlockingCancelsThePairsFutureMocks(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	m, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding)
	if err != nil {
		t.Fatal(err)
	}
	conv, err := svc.Invite(ctx, guest, hostHandle, "Hi, looking forward to the mock")
	if err != nil {
		t.Fatal(err)
	}

	if err := chat(pool).Block(ctx, host, conv.ID); err != nil {
		t.Fatal(err)
	}
	mocks.AnnounceCancellations(ctx, host)

	if status, reason := storedMock(t, pool, m.ID); status != buddy.MockCancelled || reason != buddy.CancelByBlock {
		t.Fatalf("stored = %s/%s", status, reason)
	}
	if len(n.cancelled) != 1 || n.cancelled[0].CancelReason != buddy.CancelByBlock {
		t.Fatalf("cancel notices = %+v", n.cancelled)
	}
}
```
Add `"slices"`, `"github.com/google/uuid"`, `"github.com/jackc/pgx/v5/pgxpool"` and `"github.com/strelov1/freehire/internal/engage/slots"` to this file's imports.

- [ ] 6.2 Run: `go vet -tags=integration ./internal/engage/buddy/`. Expected: FAIL — `mocks.Cancel undefined`, `mocks.AnnounceCancellations undefined`, `undefined: buddy.ErrMockNotCancellable`.

- [ ] 6.3 Implement. In `internal/engage/buddy/mock.go`, inside the booking-sentinel `var (...)` block, add:

```go
	// ErrMockNotCancellable → 422. The mock has started, or is already cancelled.
	ErrMockNotCancellable = errors.New("buddy: the mock interview has started or is already cancelled")
```
In `internal/engage/buddy/mock_service.go` extend `MockRepository` after `MockByID`:

```go
	// CancelMock cancels a confirmed, not yet started mock userID takes part in, freeing
	// both participants' time in the same statement; false when any of that does not hold.
	CancelMock(ctx context.Context, id uuid.UUID, userID int64) (bool, error)
	// ClaimCancelNotices claims every recent, not yet announced cancellation actorID caused
	// — by cancelling, leaving or blocking — and returns those mocks.
	ClaimCancelNotices(ctx context.Context, actorID int64) ([]Mock, error)
```
and append:

```go
// Cancel cancels one of the caller's mocks before it starts and tells the other side.
// Someone else's mock reads as ErrMockNotFound; a started or cancelled one is
// ErrMockNotCancellable.
func (s *MockService) Cancel(ctx context.Context, userID int64, id uuid.UUID) error {
	m, ok, err := s.repo.MockByID(ctx, id)
	if err != nil {
		return err
	}
	if !ok || (userID != m.HostUserID && userID != m.GuestUserID) {
		return ErrMockNotFound
	}
	if m.Status != MockConfirmed || !s.now().Before(m.StartsAt) {
		return ErrMockNotCancellable
	}
	done, err := s.repo.CancelMock(ctx, id, userID)
	if err != nil {
		return err
	}
	if !done {
		// The start or another cancellation landed between the read and the write.
		return ErrMockNotCancellable
	}
	s.AnnounceCancellations(ctx, userID)
	return nil
}

// AnnounceCancellations tells the other participant of every mock actorID's own act
// cancelled and nobody has announced yet: a participant's Cancel, the leave
// (LeaveBuddyMember cancels in its own statement) or a block (a trigger on user_blocks
// cancels inside the block's statement). The api layer calls it after a leave and after a
// block. Best-effort: a failure is logged, never returned, because the cancellation it
// reports has already happened.
func (s *MockService) AnnounceCancellations(ctx context.Context, actorID int64) {
	if s.notifier == nil {
		return
	}
	cancelled, err := s.repo.ClaimCancelNotices(ctx, actorID)
	if err != nil {
		log.Printf("buddy: claiming cancellation notices of user %d: %v", actorID, err)
	}
	for _, m := range cancelled {
		if err := s.notifier.MockCancelled(ctx, m, actorID); err != nil {
			log.Printf("buddy: cancellation notice for mock %s: %v", m.ID, err)
		}
	}
}
```
In `internal/engage/buddy/mock_repository.go` append:

```go
func (r *QueriesRepository) CancelMock(ctx context.Context, id uuid.UUID, userID int64) (bool, error) {
	_, err := r.q.CancelBuddyMock(ctx, db.CancelBuddyMockParams{ID: pgtype.UUID{Bytes: id, Valid: true}, UserID: userID})
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	return err == nil, err
}

// ClaimCancelNotices returns what it read before an error, so the claimed mocks it did
// read are still announced.
func (r *QueriesRepository) ClaimCancelNotices(ctx context.Context, actorID int64) ([]Mock, error) {
	ids, err := r.q.ClaimBuddyMockCancelNotices(ctx, actorID)
	if err != nil {
		return nil, err
	}
	out := make([]Mock, 0, len(ids))
	for _, id := range ids {
		m, found, err := r.MockByID(ctx, uuid.UUID(id.Bytes))
		if err != nil {
			return out, err
		}
		if found {
			out = append(out, m)
		}
	}
	return out, nil
}
```

- [ ] 6.4 Run: `go test -tags=integration ./internal/engage/buddy/ -run 'TestGuestCancels|TestAStartedMock|TestAStranger|TestLeaving|TestBlocking' -count=1`, then the whole buddy suite to prove the leave change kept the old behaviour: `go test -tags=integration ./internal/engage/buddy/ -count=1`. Expected: `ok` for both.

- [ ] 6.5 Verify and commit:

```bash
gofmt -l internal/engage/buddy && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock.go internal/engage/buddy/mock_service.go internal/engage/buddy/mock_repository.go internal/engage/buddy/mock_cancel_integration_test.go
git commit -m "Cancel buddy mocks by participant, leave and block"
```

## 7. Mock list, blind review and member summary

**Files:**
- Modify: `internal/engage/buddy/mock.go` (review constants, sentinels, `MockReview`, `MockEntry`, `EntryFor`, `settle`, `RevealedReview`, `MockSummary`, `MockList`), `internal/engage/buddy/mock_service.go` (interface, `List`, `Review`, `MemberSummary`), `internal/engage/buddy/mock_repository.go` (`MockEntries`, `CreateMockReview`, `MockSummary`), `internal/engage/buddy/mock_test.go` (`TestMockEntrySettle`)
- Create: `internal/engage/buddy/mock_review_integration_test.go`

**Interfaces:**
- Consumes: `db.Queries.ListBuddyMocksForUser`, `CreateBuddyMockReview`, `GetBuddyMemberMockSummary`, `ListBuddyMemberRevealedReviews`; `visibleMember`, `requireActive`, `MockByID`.
- Produces: `const ReviewWindow = 7 * 24 * time.Hour`, `const MaxReviewCommentRunes = 1000`; `var ErrInvalidReview, ErrAlreadyReviewed error`; `type MockReview struct{ Rating int; Comment string }`; `type MockEntry struct{ ID uuid.UUID; Role, Counterpart string; StartsAt, EndsAt time.Time; Topic, Status, CancelReason, MeetingURL string; MyReview, TheirReview *MockReview; CanCancel, CanReview bool }`; `func (Mock) EntryFor(userID int64, now time.Time) MockEntry`; `type MockList struct{ Upcoming, Past []MockEntry }`; `type RevealedReview struct{ Rating int; Comment, Topic, Day string }`; `type MockSummary struct{ Completed, RatingCount int; RatingAvg float64; Recent []RevealedReview }`; `func (*MockService) List(ctx, userID int64) (MockList, error)`, `Review(ctx, userID int64, id uuid.UUID, rating int, comment string) error`, `MemberSummary(ctx, viewerID int64, handle string) (MockSummary, error)`; repository `MockEntries(ctx, userID int64, limit int32) ([]MockEntry, error)`, `CreateMockReview(ctx, id uuid.UUID, authorID int64, rating int, comment string) (bool, error)`, `MockSummary(ctx, subjectID int64, recent int32) (MockSummary, error)`.

- [ ] 7.1 Write the failing unit test — append to `internal/engage/buddy/mock_test.go`:

```go
func TestMockEntrySettle(t *testing.T) {
	start := time.Date(2026, time.November, 10, 18, 0, 0, 0, time.UTC)
	base := MockEntry{StartsAt: start, EndsAt: start.Add(MockDuration), Status: MockConfirmed, MeetingURL: "https://meet.example.test/room"}

	before := base
	before.settle(start.Add(-time.Minute))
	if !before.CanCancel || before.CanReview || before.MeetingURL == "" {
		t.Errorf("before the start: %+v", before)
	}
	after := base
	after.settle(start.Add(2 * time.Hour))
	if after.CanCancel || !after.CanReview {
		t.Errorf("after the end: %+v", after)
	}
	reviewed := base
	reviewed.MyReview = &MockReview{Rating: 4}
	reviewed.settle(start.Add(2 * time.Hour))
	if reviewed.CanReview {
		t.Error("a reviewed mock still offers a review")
	}
	closed := base
	closed.settle(start.Add(MockDuration + ReviewWindow))
	if closed.CanReview {
		t.Error("a mock past its review window still offers a review")
	}
	cancelled := base
	cancelled.Status = MockCancelled
	cancelled.settle(start.Add(-time.Hour))
	if cancelled.CanCancel || cancelled.MeetingURL != "" {
		t.Errorf("a cancelled mock: %+v", cancelled)
	}
}
```

- [ ] 7.2 Write the failing integration tests `internal/engage/buddy/mock_review_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/engage/buddy"
)

func TestTheGuestSeesTheHostsCurrentLink(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "UTC")
	if _, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicCoding); err != nil {
		t.Fatal(err)
	}
	const fixed = "https://meet.example.test/fixed-room"
	if _, err := mocks.SaveSettings(ctx, host, buddy.MockSettings{Timezone: "UTC", MeetingURL: fixed, Topics: buddy.MockTopics}); err != nil {
		t.Fatal(err)
	}

	list, err := mocks.List(ctx, guest)
	if err != nil {
		t.Fatal(err)
	}
	if len(list.Upcoming) != 1 {
		t.Fatalf("upcoming = %+v", list.Upcoming)
	}
	e := list.Upcoming[0]
	if e.MeetingURL != fixed || e.Role != buddy.RoleGuest || e.Counterpart != hostHandle || !e.CanCancel {
		t.Fatalf("entry = %+v", e)
	}
}

func TestAReviewBeforeTheEndIsRefused(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-30*time.Minute)) // under way

	if err := mocks.Review(context.Background(), guest, id, 5, ""); !errors.Is(err, buddy.ErrInvalidReview) {
		t.Fatalf("err = %v, want ErrInvalidReview", err)
	}
}

func TestReviewsStayHiddenUntilBothHaveReviewed(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, guestHandle := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-2*time.Hour)) // ended an hour ago

	if err := mocks.Review(ctx, guest, id, 4, "Clear on trade-offs"); err != nil {
		t.Fatal(err)
	}
	hostList, err := mocks.List(ctx, host)
	if err != nil {
		t.Fatal(err)
	}
	if e := hostList.Past[0]; e.TheirReview != nil || !e.CanReview {
		t.Fatalf("host before reviewing: %+v", e)
	}
	if sum, err := mocks.MemberSummary(ctx, guest, hostHandle); err != nil || sum.RatingCount != 0 || sum.Completed != 1 {
		t.Fatalf("host page before both reviewed = %+v, %v", sum, err)
	}

	if err := mocks.Review(ctx, host, id, 5, "Great questions"); err != nil {
		t.Fatal(err)
	}
	if hostList, err = mocks.List(ctx, host); err != nil {
		t.Fatal(err)
	}
	if r := hostList.Past[0].TheirReview; r == nil || r.Rating != 4 || r.Comment != "Clear on trade-offs" {
		t.Fatalf("host sees %+v", r)
	}
	guestList, err := mocks.List(ctx, guest)
	if err != nil {
		t.Fatal(err)
	}
	if r := guestList.Past[0].TheirReview; r == nil || r.Rating != 5 {
		t.Fatalf("guest sees %+v", r)
	}
	hostPage, err := mocks.MemberSummary(ctx, guest, hostHandle)
	if err != nil || hostPage.RatingCount != 1 || hostPage.RatingAvg != 4 || len(hostPage.Recent) != 1 ||
		hostPage.Recent[0].Comment != "Clear on trade-offs" || hostPage.Recent[0].Topic != buddy.TopicCoding {
		t.Fatalf("host page = %+v, %v", hostPage, err)
	}
	if guestPage, err := mocks.MemberSummary(ctx, host, guestHandle); err != nil || guestPage.RatingCount != 1 || guestPage.RatingAvg != 5 {
		t.Fatalf("guest page = %+v, %v", guestPage, err)
	}
	if err := mocks.Review(ctx, host, id, 3, ""); !errors.Is(err, buddy.ErrAlreadyReviewed) {
		t.Fatalf("second review: err = %v, want ErrAlreadyReviewed", err)
	}
}

func TestAOneSidedReviewIsRevealedWhenTheWindowCloses(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	ctx := context.Background()
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(-8*24*time.Hour))
	// Written directly: the guest reviewed inside the window, which has closed for anyone
	// writing now.
	if _, err := pool.Exec(ctx, `
		INSERT INTO buddy_mock_reviews (mock_id, author_user_id, subject_user_id, rating, comment)
		VALUES ($1::uuid, $2, $3, 3, 'Ran out of time')`, id.String(), guest, host); err != nil {
		t.Fatal(err)
	}

	hostList, err := mocks.List(ctx, host)
	if err != nil {
		t.Fatal(err)
	}
	if e := hostList.Past[0]; e.TheirReview == nil || e.TheirReview.Rating != 3 || e.CanReview {
		t.Fatalf("host after the window: %+v", e)
	}
	if sum, err := mocks.MemberSummary(ctx, guest, hostHandle); err != nil || sum.RatingCount != 1 {
		t.Fatalf("host page = %+v, %v", sum, err)
	}
	if err := mocks.Review(ctx, host, id, 5, ""); !errors.Is(err, buddy.ErrInvalidReview) {
		t.Fatalf("review after the window: err = %v, want ErrInvalidReview", err)
	}
}
```

- [ ] 7.3 Run: `go test ./internal/engage/buddy/ -run TestMockEntrySettle -count=1` and `go vet -tags=integration ./internal/engage/buddy/`. Expected: FAIL — `undefined: MockEntry`, `mocks.List undefined`, `mocks.Review undefined`, `mocks.MemberSummary undefined`.

- [ ] 7.4 Implement. In `internal/engage/buddy/mock.go`, add to the tunables `const` block after `MaxGuestMocks`:

```go
	// ReviewWindow is how long after a mock's end its participants may review it, and when
	// a review the other side never answered is revealed anyway.
	ReviewWindow = 7 * 24 * time.Hour
	// MaxReviewCommentRunes bounds a review comment, matching buddy_mock_reviews_comment_check.
	MaxReviewCommentRunes = 1000
	// recentReviews is how many revealed reviews a member page shows.
	recentReviews = 5
	// mockListLimit bounds the caller's own list.
	mockListLimit = 200
```
inside the sentinel `var (...)` block add:

```go
	// ErrInvalidReview → 422. A rating outside 1-5, a comment over MaxReviewCommentRunes,
	// a mock that has not ended, was cancelled, or whose review window has closed.
	ErrInvalidReview = errors.New("buddy: invalid review")
	// ErrAlreadyReviewed → 409. One review per participant per mock.
	ErrAlreadyReviewed = errors.New("buddy: mock interview already reviewed")
```
and append to the file:

```go
// MockReview is one participant's rating of the other.
type MockReview struct {
	Rating  int
	Comment string
}

// MockEntry is one mock as a participant sees it: their role, the other side by handle
// (empty once that member left or a block stands between them), the host's current link
// while confirmed, their own review, and the review about them once revealed.
type MockEntry struct {
	ID           uuid.UUID
	Role         string
	Counterpart  string
	StartsAt     time.Time
	EndsAt       time.Time
	Topic        string
	Status       string
	CancelReason string
	MeetingURL   string
	MyReview     *MockReview
	TheirReview  *MockReview
	CanCancel    bool
	CanReview    bool
}

// EntryFor is the mock as participant userID sees it at now.
func (m Mock) EntryFor(userID int64, now time.Time) MockEntry {
	e := MockEntry{
		ID: m.ID, Role: RoleGuest, Counterpart: m.HostHandle, StartsAt: m.StartsAt, EndsAt: m.EndsAt,
		Topic: m.Topic, Status: m.Status, CancelReason: m.CancelReason, MeetingURL: m.MeetingURL,
	}
	if userID == m.HostUserID {
		e.Role, e.Counterpart = RoleHost, m.GuestHandle
	}
	e.settle(now)
	return e
}

// settle derives what the participant may do at now, and drops the link of a mock that is
// no longer confirmed.
func (e *MockEntry) settle(now time.Time) {
	if e.Status != MockConfirmed {
		e.MeetingURL = ""
	}
	e.CanCancel = e.Status == MockConfirmed && now.Before(e.StartsAt)
	e.CanReview = e.Status == MockConfirmed && e.MyReview == nil &&
		!now.Before(e.EndsAt) && now.Before(e.EndsAt.Add(ReviewWindow))
}

// MockList is the caller's own mocks: confirmed ones not yet over, soonest first, and the
// rest (over or cancelled), newest first.
type MockList struct {
	Upcoming []MockEntry
	Past     []MockEntry
}

// RevealedReview is one review on a member page: no author, day precision.
type RevealedReview struct {
	Rating  int
	Comment string
	Topic   string
	Day     string
}

// MockSummary is what a member page shows about their mocks.
type MockSummary struct {
	Completed   int
	RatingCount int
	RatingAvg   float64
	Recent      []RevealedReview
}
```
In `internal/engage/buddy/mock_service.go` add `"strings"` and `"unicode/utf8"` to the imports, extend `MockRepository` after `ClaimCancelNotices`:

```go
	// MockEntries is userID's mocks, newest first, with their own review and the review
	// about them only once revealed. CanCancel/CanReview are left for the service.
	MockEntries(ctx context.Context, userID int64, limit int32) ([]MockEntry, error)
	// CreateMockReview stores authorID's review of the other participant; false when they
	// already reviewed it or the mock is outside the review rules.
	CreateMockReview(ctx context.Context, id uuid.UUID, authorID int64, rating int, comment string) (bool, error)
	// MockSummary is subjectID's completed count and revealed-review aggregate.
	MockSummary(ctx context.Context, subjectID int64, recent int32) (MockSummary, error)
```
and append:

```go
// List is the caller's own mocks.
func (s *MockService) List(ctx context.Context, userID int64) (MockList, error) {
	if err := s.members.requireActive(ctx, userID); err != nil {
		return MockList{}, err
	}
	entries, err := s.repo.MockEntries(ctx, userID, mockListLimit)
	if err != nil {
		return MockList{}, err
	}
	now := s.now()
	list := MockList{Upcoming: []MockEntry{}, Past: []MockEntry{}}
	for _, e := range entries {
		e.settle(now)
		if e.Status == MockConfirmed && now.Before(e.EndsAt) {
			list.Upcoming = append(list.Upcoming, e)
		} else {
			list.Past = append(list.Past, e)
		}
	}
	// The read is newest first; what is coming reads soonest first.
	slices.Reverse(list.Upcoming)
	return list, nil
}

// Review rates the other participant of one of the caller's confirmed mocks, once, between
// its end and ReviewWindow after it. The rating is hidden from its subject and their page
// until they review too or the window closes — computed by every read, never stored.
func (s *MockService) Review(ctx context.Context, userID int64, id uuid.UUID, rating int, comment string) error {
	comment = strings.TrimSpace(comment)
	if rating < 1 || rating > 5 || utf8.RuneCountInString(comment) > MaxReviewCommentRunes {
		return fmt.Errorf("%w: a rating from 1 to 5 and at most %d characters", ErrInvalidReview, MaxReviewCommentRunes)
	}
	m, ok, err := s.repo.MockByID(ctx, id)
	if err != nil {
		return err
	}
	if !ok || (userID != m.HostUserID && userID != m.GuestUserID) {
		return ErrMockNotFound
	}
	now := s.now()
	switch {
	case m.Status != MockConfirmed:
		return fmt.Errorf("%w: the mock interview was cancelled", ErrInvalidReview)
	case now.Before(m.EndsAt):
		return fmt.Errorf("%w: the mock interview has not ended", ErrInvalidReview)
	case !now.Before(m.EndsAt.Add(ReviewWindow)):
		return fmt.Errorf("%w: the review window has closed", ErrInvalidReview)
	}
	created, err := s.repo.CreateMockReview(ctx, id, userID, rating, comment)
	if err != nil {
		return err
	}
	if !created {
		return ErrAlreadyReviewed
	}
	return nil
}

// MemberSummary is what handle's member page shows another active member: completed mocks
// and their revealed reviews. Who reviewed is not part of it.
func (s *MockService) MemberSummary(ctx context.Context, viewerID int64, handle string) (MockSummary, error) {
	target, err := s.members.visibleMember(ctx, viewerID, handle)
	if err != nil {
		return MockSummary{}, err
	}
	return s.repo.MockSummary(ctx, target.UserID, recentReviews)
}
```
In `internal/engage/buddy/mock_repository.go` append:

```go
func (r *QueriesRepository) MockEntries(ctx context.Context, userID int64, limit int32) ([]MockEntry, error) {
	rows, err := r.q.ListBuddyMocksForUser(ctx, db.ListBuddyMocksForUserParams{UserID: userID, RowLimit: limit})
	if err != nil {
		return nil, err
	}
	out := make([]MockEntry, len(rows))
	for i, row := range rows {
		e := MockEntry{
			ID: uuid.UUID(row.ID.Bytes), Role: RoleGuest, Counterpart: row.HostHandle,
			StartsAt: row.StartsAt.Time, EndsAt: row.EndsAt.Time, Topic: row.Topic, Status: row.Status,
			CancelReason: row.CancelReason.String, MeetingURL: row.HostMeetingUrl.String,
		}
		if row.HostUserID == userID {
			e.Role, e.Counterpart = RoleHost, row.GuestHandle
		}
		if !row.CounterpartVisible {
			e.Counterpart = ""
		}
		if row.MyRating.Valid {
			e.MyReview = &MockReview{Rating: int(row.MyRating.Int16), Comment: row.MyComment.String}
		}
		if row.TheirRating.Valid {
			e.TheirReview = &MockReview{Rating: int(row.TheirRating.Int16), Comment: row.TheirComment.String}
		}
		out[i] = e
	}
	return out, nil
}

func (r *QueriesRepository) CreateMockReview(ctx context.Context, id uuid.UUID, authorID int64, rating int, comment string) (bool, error) {
	n, err := r.q.CreateBuddyMockReview(ctx, db.CreateBuddyMockReviewParams{
		MockID: pgtype.UUID{Bytes: id, Valid: true}, AuthorUserID: authorID, Rating: int16(rating), Comment: comment,
	})
	return n == 1, err
}

func (r *QueriesRepository) MockSummary(ctx context.Context, subjectID int64, recent int32) (MockSummary, error) {
	agg, err := r.q.GetBuddyMemberMockSummary(ctx, subjectID)
	if err != nil {
		return MockSummary{}, err
	}
	rows, err := r.q.ListBuddyMemberRevealedReviews(ctx, db.ListBuddyMemberRevealedReviewsParams{
		SubjectUserID: subjectID, RowLimit: recent,
	})
	if err != nil {
		return MockSummary{}, err
	}
	sum := MockSummary{
		Completed: int(agg.Completed), RatingCount: int(agg.RatingCount), RatingAvg: agg.RatingAvg,
		Recent: make([]RevealedReview, len(rows)),
	}
	for i, row := range rows {
		sum.Recent[i] = RevealedReview{
			Rating: int(row.Rating), Comment: row.Comment, Topic: row.Topic,
			Day: row.CreatedAt.Time.UTC().Format(time.DateOnly),
		}
	}
	return sum, nil
}
```

- [ ] 7.5 Run: `go test ./internal/engage/buddy/ -count=1 && go test -tags=integration ./internal/engage/buddy/ -run 'TestTheGuestSees|TestAReview|TestReviews|TestAOneSided' -count=1`. Expected: `ok` for both.

- [ ] 7.6 Verify and commit:

```bash
gofmt -l internal/engage/buddy && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock.go internal/engage/buddy/mock_service.go internal/engage/buddy/mock_repository.go internal/engage/buddy/mock_test.go internal/engage/buddy/mock_review_integration_test.go
git commit -m "List buddy mocks and review them blind"
```

## 8. Notifications

**Files:**
- Create: `internal/engage/buddy/mock_notify.go`, `internal/engage/buddy/mock_notify_test.go`, `internal/engage/buddy/mock_notify_integration_test.go`

**Interfaces:**
- Consumes: `emailnotify.Sender` (`Send(ctx, emailnotify.Message) error`, `internal/engage/emailnotify/notifier.go:33`), `emailnotify.Message{From, To, Subject, HTML, Text string; Group emailprefs.Group}`, `emailprefs.GroupEssential` (transactional, no unsubscribe link — the mentorship booking mail's choice and reason), `db.Queries.RecordNotification(ctx, db.RecordNotificationParams{UserID int64; Kind, Title, Body string; DedupKey pgtype.Text}) (int64, error)` (returns `pgx.ErrNoRows` on a dedup conflict); `Mock`, `ianaZone`, the `MockNotifier` interface (Task 3).
- Produces: `func NewMockMailNotifier(q *db.Queries, sender emailnotify.Sender, from, origin string) *MockMailNotifier` implementing `MockNotifier`. A nil `sender` records notification-center rows only; a nil `q` sends mail only (unit tests). Notification kinds `buddy_mock_booked`, `buddy_mock_cancelled`, `buddy_mock_reminder` (free text; no migration).

- [ ] 8.1 Write the failing unit tests `internal/engage/buddy/mock_notify_test.go`:

```go
package buddy

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
)

type captureSender struct{ sent []emailnotify.Message }

func (c *captureSender) Send(_ context.Context, m emailnotify.Message) error {
	c.sent = append(c.sent, m)
	return nil
}

func (c *captureSender) to(t *testing.T, addr string) emailnotify.Message {
	t.Helper()
	for _, m := range c.sent {
		if m.To == addr {
			return m
		}
	}
	t.Fatalf("nothing sent to %s (sent: %d)", addr, len(c.sent))
	return emailnotify.Message{}
}

// sampleMock is a system-design mock on Tuesday 10 November 2026, 18:00-19:00 UTC: 19:00
// for a host in Berlin and 03:00 on Wednesday for a guest in Tokyo.
func sampleMock() Mock {
	start := time.Date(2026, time.November, 10, 18, 0, 0, 0, time.UTC)
	return Mock{
		ID: uuid.MustParse("3f0e4b8a-1c2d-4e5f-8a9b-0c1d2e3f4a5b"), HostUserID: 1, GuestUserID: 2,
		HostHandle: "calm-otter-11", GuestHandle: "brave-lynx-22",
		StartsAt: start, EndsAt: start.Add(MockDuration), Topic: TopicSystemDesign, Status: MockConfirmed,
		HostTimezone: "Europe/Berlin", GuestTimezone: "Asia/Tokyo", MeetingURL: "https://meet.example.test/room",
		HostEmail: "host@example.test", GuestEmail: "guest@example.test",
	}
}

func notifier(c *captureSender) *MockMailNotifier {
	return NewMockMailNotifier(nil, c, "noreply@freehire.test", "https://freehire.test/")
}

func assertContains(t *testing.T, label, body string, want ...string) {
	t.Helper()
	for _, w := range want {
		if !strings.Contains(body, w) {
			t.Errorf("%s lacks %q:\n%s", label, w, body)
		}
	}
}

func assertLacks(t *testing.T, label, body string, leaks ...string) {
	t.Helper()
	for _, l := range leaks {
		if strings.Contains(body, l) {
			t.Errorf("%s carries %q:\n%s", label, l, body)
		}
	}
}

func TestBookedNoticeNamesTheHandleTopicAndRecipientsZone(t *testing.T) {
	c := &captureSender{}
	if err := notifier(c).MockBooked(context.Background(), sampleMock()); err != nil {
		t.Fatal(err)
	}
	host := c.to(t, "host@example.test")
	all := host.Subject + host.Text + host.HTML
	assertContains(t, "host mail", host.Text, "brave-lynx-22", "system design", "Tuesday 10 November 2026, 19:00", "Europe/Berlin",
		"https://meet.example.test/room", "https://freehire.test/my/buddies/mocks")
	assertLacks(t, "host mail", all, "guest@example.test", "Asia/Tokyo", "calm-otter-11")

	guest := c.to(t, "guest@example.test")
	assertContains(t, "guest mail", guest.Text, "calm-otter-11", "Wednesday 11 November 2026, 03:00", "Asia/Tokyo")
	assertLacks(t, "guest mail", guest.Subject+guest.Text+guest.HTML, "host@example.test", "Europe/Berlin")
}

func TestCancelNoticeGoesOnlyToTheOtherSide(t *testing.T) {
	c := &captureSender{}
	m := sampleMock()
	m.Status, m.CancelReason = MockCancelled, CancelByParticipant
	if err := notifier(c).MockCancelled(context.Background(), m, m.GuestUserID); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 1 || c.sent[0].To != "host@example.test" {
		t.Fatalf("sent = %+v, want the host only", c.sent)
	}
	assertContains(t, "participant cancel", c.sent[0].Text, "brave-lynx-22 cancelled your system design mock interview.")
	assertLacks(t, "participant cancel", c.sent[0].Text, "https://meet.example.test/room")

	c = &captureSender{}
	m.CancelReason = CancelByLeave
	if err := notifier(c).MockCancelled(context.Background(), m, m.HostUserID); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 1 || c.sent[0].To != "guest@example.test" {
		t.Fatalf("sent = %+v, want the guest only", c.sent)
	}
	// A leave or a block is told only as "cancelled", never why.
	assertContains(t, "leave cancel", c.sent[0].Text, "Your system design mock interview with calm-otter-11 was cancelled.")
	assertLacks(t, "leave cancel", c.sent[0].Text, "left", "block")
}

func TestReminderGoesToBothWithTheLink(t *testing.T) {
	c := &captureSender{}
	if err := notifier(c).MockReminder(context.Background(), sampleMock(), time.Hour); err != nil {
		t.Fatal(err)
	}
	if len(c.sent) != 2 {
		t.Fatalf("sent %d mails, want 2", len(c.sent))
	}
	assertContains(t, "host reminder", c.to(t, "host@example.test").Text, "starts in an hour", "brave-lynx-22", "https://meet.example.test/room")
	assertContains(t, "guest reminder", c.to(t, "guest@example.test").Text, "starts in an hour", "calm-otter-11")
}

func TestAnUnusableZoneFallsBackToUTCAndSaysSo(t *testing.T) {
	c := &captureSender{}
	m := sampleMock()
	m.GuestTimezone = "Local"
	if err := notifier(c).MockBooked(context.Background(), m); err != nil {
		t.Fatal(err)
	}
	assertContains(t, "guest mail", c.to(t, "guest@example.test").Text, "Tuesday 10 November 2026, 18:00", "(UTC)")
}
```

- [ ] 8.2 Run: `go test ./internal/engage/buddy/ -run 'Notice|Reminder|UnusableZone' -count=1`. Expected: FAIL — `undefined: NewMockMailNotifier`, `undefined: MockMailNotifier`.

- [ ] 8.3 Create `internal/engage/buddy/mock_notify.go`:

```go
package buddy

import (
	"context"
	"errors"
	"fmt"
	"html"
	"log"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/engage/emailprefs"
	"github.com/strelov1/freehire/internal/platform/db"
)

// Notification-center kinds of a mock notice. user_notifications.kind is free text, so they
// need no migration.
const (
	notificationMockBooked    = "buddy_mock_booked"
	notificationMockCancelled = "buddy_mock_cancelled"
	notificationMockReminder  = "buddy_mock_reminder"
)

var _ MockNotifier = (*MockMailNotifier)(nil)

// MockMailNotifier delivers mock notices by email and records each in the notification
// center. Every notice names the other participant by handle only — never an account name
// or address — and states the time in its recipient's own zone: the host's from their
// settings, the guest's from the booking, falling back to UTC and saying so.
//
// The mail is emailprefs.GroupEssential, as mentorship's booking mail is and for its
// reason: it is a commitment the member made and another member is holding an hour for,
// and an unsubscribe link on it would be an offer that cannot be honoured.
type MockMailNotifier struct {
	q      *db.Queries
	sender emailnotify.Sender
	from   string
	origin string
}

// NewMockMailNotifier builds a MockMailNotifier. A nil sender records notification-center
// rows only (a deployment with no mail transport); a nil q sends mail only.
func NewMockMailNotifier(q *db.Queries, sender emailnotify.Sender, from, origin string) *MockMailNotifier {
	return &MockMailNotifier{q: q, sender: sender, from: from, origin: strings.TrimRight(origin, "/")}
}

// mockRecipient is one participant as their own notice addresses them.
type mockRecipient struct {
	userID int64
	email  string
	zone   string
	host   bool
	// other is the OTHER participant's handle: the only thing a notice says about them.
	other string
}

func (m Mock) hostRecipient() mockRecipient {
	return mockRecipient{userID: m.HostUserID, email: m.HostEmail, zone: m.HostTimezone, host: true, other: m.GuestHandle}
}

func (m Mock) guestRecipient() mockRecipient {
	return mockRecipient{userID: m.GuestUserID, email: m.GuestEmail, zone: m.GuestTimezone, other: m.HostHandle}
}

// mockNotice is one rendered notice before it is addressed.
type mockNotice struct {
	kind    string
	subject string
	line    string
	// link is whether the host's meeting link belongs in it: yes for a booking and a
	// reminder, never for a cancellation.
	link bool
	// event names the occurrence in the dedup key, so a retried send records once.
	event string
}

func (n *MockMailNotifier) MockBooked(ctx context.Context, m Mock) error {
	topic := topicPhrase(m.Topic)
	return n.deliver(ctx, m, []mockRecipient{m.hostRecipient(), m.guestRecipient()}, func(r mockRecipient) mockNotice {
		line := fmt.Sprintf("Your %s mock interview with %s is booked.", topic, r.other)
		if r.host {
			line = fmt.Sprintf("%s booked a %s mock interview with you.", r.other, topic)
		}
		return mockNotice{kind: notificationMockBooked, subject: "Mock interview booked", line: line, link: true, event: "booked"}
	})
}

func (n *MockMailNotifier) MockCancelled(ctx context.Context, m Mock, actorID int64) error {
	to := m.hostRecipient()
	if actorID == m.HostUserID {
		to = m.guestRecipient()
	}
	topic := topicPhrase(m.Topic)
	return n.deliver(ctx, m, []mockRecipient{to}, func(r mockRecipient) mockNotice {
		// A leave or a block reads only as "cancelled": why is the other member's business.
		line := fmt.Sprintf("Your %s mock interview with %s was cancelled.", topic, r.other)
		if m.CancelReason == CancelByParticipant {
			line = fmt.Sprintf("%s cancelled your %s mock interview.", r.other, topic)
		}
		return mockNotice{kind: notificationMockCancelled, subject: "Mock interview cancelled", line: line, event: "cancelled"}
	})
}

func (n *MockMailNotifier) MockReminder(ctx context.Context, m Mock, before time.Duration) error {
	topic, lead := topicPhrase(m.Topic), leadPhrase(before)
	return n.deliver(ctx, m, []mockRecipient{m.hostRecipient(), m.guestRecipient()}, func(r mockRecipient) mockNotice {
		return mockNotice{
			kind:    notificationMockReminder,
			subject: "Your mock interview starts in " + lead,
			line:    fmt.Sprintf("Your %s mock interview with %s starts in %s.", topic, r.other, lead),
			link:    true,
			event:   fmt.Sprintf("reminder-%d", int(before/time.Minute)),
		}
	})
}

// deliver sends one notice per recipient and records each delivered one in the
// notification center, joining failures rather than stopping at the first: one
// unreachable mailbox must not cost the other participant their notice.
func (n *MockMailNotifier) deliver(ctx context.Context, m Mock, to []mockRecipient, render func(mockRecipient) mockNotice) error {
	var errs []error
	for _, r := range to {
		nt := render(r)
		when := mockWhen(m.StartsAt, m.EndsAt, r.zone)
		if n.sender != nil && r.email != "" {
			if err := n.sender.Send(ctx, emailnotify.Message{
				From: n.from, To: r.email, Subject: nt.subject,
				HTML: n.renderHTML(nt, when, m), Text: n.renderText(nt, when, m),
				Group: emailprefs.GroupEssential,
			}); err != nil {
				errs = append(errs, fmt.Errorf("buddy: %s notice to user %d: %w", nt.event, r.userID, err))
				continue
			}
		}
		n.record(ctx, r.userID, nt, when, m)
	}
	return errors.Join(errs...)
}

// record writes the notification-center row. Best-effort: a failure here never fails the
// delivery it records.
func (n *MockMailNotifier) record(ctx context.Context, userID int64, nt mockNotice, when string, m Mock) {
	if n.q == nil {
		return
	}
	if _, err := n.q.RecordNotification(ctx, db.RecordNotificationParams{
		UserID: userID, Kind: nt.kind, Title: nt.subject, Body: nt.line + " " + when,
		DedupKey: pgtype.Text{String: fmt.Sprintf("%s:%s:%s:%d", nt.kind, m.ID, nt.event, userID), Valid: true},
	}); err != nil && !errors.Is(err, pgx.ErrNoRows) {
		log.Printf("buddy: recording %s notice for user %d: %v", nt.kind, userID, err)
	}
}

func (n *MockMailNotifier) cabinetURL() string { return n.origin + "/my/buddies/mocks" }

func (n *MockMailNotifier) renderText(nt mockNotice, when string, m Mock) string {
	lines := []string{nt.line, "", when}
	if nt.link && m.MeetingURL != "" {
		lines = append(lines, "", "Join: "+m.MeetingURL)
	}
	lines = append(lines, "", "Your mock interviews: "+n.cabinetURL())
	return strings.Join(lines, "\n")
}

func (n *MockMailNotifier) renderHTML(nt mockNotice, when string, m Mock) string {
	var b strings.Builder
	fmt.Fprintf(&b, "<p>%s</p><p><strong>%s</strong></p>", html.EscapeString(nt.line), html.EscapeString(when))
	if nt.link && m.MeetingURL != "" {
		fmt.Fprintf(&b, `<p><a href="%s">Join the call</a></p>`, html.EscapeString(m.MeetingURL))
	}
	fmt.Fprintf(&b, `<p><a href="%s">Your mock interviews</a></p>`, html.EscapeString(n.cabinetURL()))
	return b.String()
}

// mockWhen states a mock in one recipient's zone, naming the zone. A zone that is not an
// IANA place falls back to UTC AND SAYS SO — never to the other participant's zone, since a
// time labelled with a zone that is not the reader's is undetectably wrong.
func mockWhen(start, end time.Time, zoneName string) string {
	zone, ok := ianaZone(zoneName)
	if !ok {
		zone, zoneName = time.UTC, "UTC"
	}
	return fmt.Sprintf("%s – %s (%s)",
		start.In(zone).Format("Monday 2 January 2006, 15:04"), end.In(zone).Format("15:04"), zoneName)
}

// topicPhrase is a topic as it reads inside a sentence.
func topicPhrase(topic string) string {
	switch topic {
	case TopicCoding:
		return "coding"
	case TopicSystemDesign:
		return "system design"
	case TopicBehavioral:
		return "behavioral"
	}
	return topic
}

// leadPhrase renders a reminder offset as somebody would say it.
func leadPhrase(d time.Duration) string {
	switch {
	case d == time.Hour:
		return "an hour"
	case d >= time.Hour:
		return fmt.Sprintf("%d hours", int(d/time.Hour))
	default:
		return fmt.Sprintf("%d minutes", int(d/time.Minute))
	}
}
```

- [ ] 8.4 Run: `go test ./internal/engage/buddy/ -run 'Notice|Reminder|UnusableZone' -count=1`. Expected: `ok`.

- [ ] 8.5 Write the integration test proving the notification-center rows carry handles only, `internal/engage/buddy/mock_notify_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"strings"
	"sync"
	"testing"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/emailnotify"
	"github.com/strelov1/freehire/internal/platform/db"
)

type mailbox struct {
	mu   sync.Mutex
	sent []emailnotify.Message
}

func (b *mailbox) Send(_ context.Context, m emailnotify.Message) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.sent = append(b.sent, m)
	return nil
}

func TestBookingNoticesNameHandlesOnly(t *testing.T) {
	pool, svc := setup(t)
	ctx := context.Background()
	q := db.New(pool)
	box := &mailbox{}
	mocks := buddy.NewMockService(svc, buddy.NewQueriesRepository(q),
		buddy.NewMockMailNotifier(q, box, "noreply@freehire.test", "https://freehire.test"))
	host, hostHandle := joined(t, pool, svc, "host@example.test")
	guest, guestHandle := joined(t, pool, svc, "guest@example.test")
	openAllWeek(t, mocks, host, "Europe/Berlin")

	if _, err := book(mocks, guest, hostHandle, firstSlot(t, mocks, guest, hostHandle), buddy.TopicSystemDesign); err != nil {
		t.Fatal(err)
	}
	if len(box.sent) != 2 {
		t.Fatalf("sent %d mails, want one per participant", len(box.sent))
	}
	for _, m := range box.sent {
		other := "guest@example.test"
		if m.To == "guest@example.test" {
			other = "host@example.test"
		}
		if strings.Contains(m.Subject+m.Text+m.HTML, other) {
			t.Fatalf("mail to %s carries %s", m.To, other)
		}
	}
	var kind, body string
	if err := pool.QueryRow(ctx, `SELECT kind, body FROM user_notifications WHERE user_id = $1`, host).Scan(&kind, &body); err != nil {
		t.Fatal(err)
	}
	if kind != "buddy_mock_booked" || !strings.Contains(body, guestHandle) || strings.Contains(body, "guest@example.test") {
		t.Fatalf("host notification = %s %q", kind, body)
	}
}
```
Run: `go test -tags=integration ./internal/engage/buddy/ -run TestBookingNoticesNameHandlesOnly -count=1`. Expected: `ok`.

- [ ] 8.6 Verify and commit:

```bash
gofmt -l internal/engage/buddy && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock_notify.go internal/engage/buddy/mock_notify_test.go internal/engage/buddy/mock_notify_integration_test.go
git commit -m "Notify buddy mock participants by handle only"
```

## 9. Reminders in cmd/mentorship-remind

**Files:**
- Create: `internal/engage/buddy/mock_remind.go`, `internal/engage/buddy/mock_remind_integration_test.go`
- Modify: `internal/engage/buddy/mock_service.go` (interface), `internal/engage/buddy/mock_repository.go` (three methods), `cmd/mentorship-remind/main.go:1-24` (doc comment), `:26-34` (imports), `:70-85` (the pass)

**Interfaces:**
- Consumes: `db.Queries.ListBuddyMocksDueForReminder`, `RecordBuddyMockReminder`, `DeleteBuddyMockReminder`; `MockByID`; `MockNotifier.MockReminder`; `NewMockMailNotifier` (Task 8).
- Produces: `var MockReminderOffsets = []time.Duration{24 * time.Hour, time.Hour}`; `type MockReminderStats struct{ Sent, Failed int }`; `func (*MockService) SendDueMockReminders(ctx, maxPerRun int32) (MockReminderStats, error)`; repository `MocksDueForReminder(ctx, offset, floor time.Duration, limit int32) ([]Mock, error)`, `ClaimMockReminder(ctx, id uuid.UUID, offset time.Duration) (bool, error)`, `ReleaseMockReminder(ctx, id uuid.UUID, offset time.Duration) error`.

- [ ] 9.1 Write the failing tests `internal/engage/buddy/mock_remind_integration_test.go`:

```go
//go:build integration

package buddy_test

import (
	"context"
	"testing"
	"time"
)

func TestAMockBooked13HoursAheadGetsOnlyTheOneHourReminder(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(13*time.Hour))

	if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 0 {
		t.Fatalf("13h ahead: %+v, %v; want nothing (the 24-hour window never held it)", stats, err)
	}
	// Twelve and a half hours pass.
	if _, err := pool.Exec(ctx, `UPDATE buddy_mocks SET starts_at = now() + interval '30 minutes',
		ends_at = now() + interval '90 minutes' WHERE id = $1::uuid`, id.String()); err != nil {
		t.Fatal(err)
	}
	stats, err := mocks.SendDueMockReminders(ctx, 100)
	if err != nil || stats.Sent != 1 {
		t.Fatalf("30m ahead: %+v, %v", stats, err)
	}
	if want := id.String() + ":1h0m0s"; len(n.reminded) != 1 || n.reminded[0] != want {
		t.Fatalf("reminded = %v, want [%s]", n.reminded, want)
	}
	if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 0 {
		t.Fatalf("a rerun sent %+v, %v", stats, err)
	}
}

func TestTheTwentyFourHourReminderGoesOut(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	id := seedMock(t, pool, host, guest, time.Now().Add(23*time.Hour+30*time.Minute))

	if stats, err := mocks.SendDueMockReminders(context.Background(), 100); err != nil || stats.Sent != 1 {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
	if want := id.String() + ":24h0m0s"; len(n.reminded) != 1 || n.reminded[0] != want {
		t.Fatalf("reminded = %v, want [%s]", n.reminded, want)
	}
}

func TestAFailedReminderIsReleasedAndRetried(t *testing.T) {
	pool, svc, mocks, n := mockSetup(t)
	ctx := context.Background()
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	seedMock(t, pool, host, guest, time.Now().Add(30*time.Minute))
	n.failReminders = 1

	stats, err := mocks.SendDueMockReminders(ctx, 100)
	if err != nil || stats.Failed != 1 || stats.Sent != 0 {
		t.Fatalf("failing run = %+v, %v", stats, err)
	}
	var claims int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM buddy_mock_reminders`).Scan(&claims); err != nil || claims != 0 {
		t.Fatalf("claims after a failed delivery = %d (err %v), want the claim released", claims, err)
	}
	if stats, err := mocks.SendDueMockReminders(ctx, 100); err != nil || stats.Sent != 1 {
		t.Fatalf("retry = %+v, %v", stats, err)
	}
}

func TestAStartedMockIsNotReminded(t *testing.T) {
	pool, svc, mocks, _ := mockSetup(t)
	host, _ := joined(t, pool, svc, "host@example.test")
	guest, _ := joined(t, pool, svc, "guest@example.test")
	seedMock(t, pool, host, guest, time.Now().Add(-5*time.Minute))

	if stats, err := mocks.SendDueMockReminders(context.Background(), 100); err != nil || stats.Sent != 0 {
		t.Fatalf("stats = %+v, %v", stats, err)
	}
}
```

- [ ] 9.2 Run: `go vet -tags=integration ./internal/engage/buddy/`. Expected: FAIL — `mocks.SendDueMockReminders undefined`.

- [ ] 9.3 Implement. Extend `MockRepository` in `internal/engage/buddy/mock_service.go` after `MockSummary`:

```go
	// MocksDueForReminder is confirmed mocks starting in (floor, offset] from now and not
	// yet reminded at offset.
	MocksDueForReminder(ctx context.Context, offset, floor time.Duration, limit int32) ([]Mock, error)
	// ClaimMockReminder records one reminder as sent; false when another run holds it.
	ClaimMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) (bool, error)
	// ReleaseMockReminder gives a claim back after a failed delivery.
	ReleaseMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) error
```
Create `internal/engage/buddy/mock_remind.go`:

```go
package buddy

import (
	"context"
	"fmt"
	"log"
	"time"
)

// MockReminderOffsets is how far ahead of a mock each reminder goes out: the mentorship
// offsets, for the same unmeasured reason. The claim is keyed on the offset in minutes, so
// a third offset costs no migration.
var MockReminderOffsets = []time.Duration{24 * time.Hour, time.Hour}

// mockReminderWindow is each offset's firing window, against a worker run every ten
// minutes: wide enough to survive missed runs, narrow enough that "in 24 hours" is true.
const mockReminderWindow = time.Hour

// mockReminderFloor is the bottom of one offset's window, never below zero.
func mockReminderFloor(offset time.Duration) time.Duration {
	if floor := offset - mockReminderWindow; floor > 0 {
		return floor
	}
	return 0
}

// MockReminderStats is what one run did.
type MockReminderStats struct {
	Sent   int
	Failed int
}

// SendDueMockReminders reminds both participants of each confirmed mock at most once per
// offset, in mentorship's order: CLAIM first and send only what was won, and RELEASE the
// claim when delivery fails so the next run retries — a missing "starts in an hour" costs
// somebody the mock, a duplicate costs them a duplicate. A mock booked inside an offset's
// window never gets that reminder (there was never that much notice to give); one already
// started gets none. Failing to read the work is the only error returned.
func (s *MockService) SendDueMockReminders(ctx context.Context, maxPerRun int32) (MockReminderStats, error) {
	var stats MockReminderStats
	// Nothing to send with means nothing to claim: a claim without a delivery is a
	// reminder marked sent forever.
	if s.notifier == nil {
		return stats, nil
	}
	for _, offset := range MockReminderOffsets {
		due, err := s.repo.MocksDueForReminder(ctx, offset, mockReminderFloor(offset), maxPerRun)
		if err != nil {
			return stats, fmt.Errorf("buddy: reading the %v mock reminders: %w", offset, err)
		}
		for _, m := range due {
			claimed, err := s.repo.ClaimMockReminder(ctx, m.ID, offset)
			if err != nil {
				stats.Failed++
				log.Printf("buddy: claiming the %v reminder of mock %s: %v", offset, m.ID, err)
				continue
			}
			if !claimed {
				continue
			}
			if err := s.notifier.MockReminder(ctx, m, offset); err != nil {
				stats.Failed++
				log.Printf("buddy: the %v reminder of mock %s: %v", offset, m.ID, err)
				// The release must land even when the run's own context is what failed the
				// delivery: a claim left in place silences this reminder for good.
				rctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
				if err := s.repo.ReleaseMockReminder(rctx, m.ID, offset); err != nil {
					log.Printf("buddy: releasing the %v reminder of mock %s: %v", offset, m.ID, err)
				}
				cancel()
				continue
			}
			stats.Sent++
		}
	}
	return stats, nil
}
```
Append to `internal/engage/buddy/mock_repository.go`:

```go
func (r *QueriesRepository) MocksDueForReminder(ctx context.Context, offset, floor time.Duration, limit int32) ([]Mock, error) {
	ids, err := r.q.ListBuddyMocksDueForReminder(ctx, db.ListBuddyMocksDueForReminderParams{
		OffsetMinutes: int32(offset / time.Minute), FloorMinutes: int32(floor / time.Minute), RowLimit: limit,
	})
	if err != nil {
		return nil, err
	}
	out := make([]Mock, 0, len(ids))
	for _, id := range ids {
		m, found, err := r.MockByID(ctx, uuid.UUID(id.Bytes))
		if err != nil {
			return nil, err
		}
		if found {
			out = append(out, m)
		}
	}
	return out, nil
}

func (r *QueriesRepository) ClaimMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) (bool, error) {
	n, err := r.q.RecordBuddyMockReminder(ctx, db.RecordBuddyMockReminderParams{
		MockID: pgtype.UUID{Bytes: id, Valid: true}, OffsetMinutes: int32(offset / time.Minute),
	})
	return n == 1, err
}

func (r *QueriesRepository) ReleaseMockReminder(ctx context.Context, id uuid.UUID, offset time.Duration) error {
	_, err := r.q.DeleteBuddyMockReminder(ctx, db.DeleteBuddyMockReminderParams{
		MockID: pgtype.UUID{Bytes: id, Valid: true}, OffsetMinutes: int32(offset / time.Minute),
	})
	return err
}
```

- [ ] 9.4 Run: `go test -tags=integration ./internal/engage/buddy/ -run 'TestAMockBooked13|TestTheTwentyFour|TestAFailedReminder|TestAStartedMockIsNot' -count=1`. Expected: `ok`.

- [ ] 9.5 Run the buddy pass from the existing reminder worker. In `cmd/mentorship-remind/main.go`:
  - append to the package doc comment (after the paragraph ending `…what rolling it back looks like — and it must not merely skip SENDING, because claiming a reminder it cannot deliver would mark it sent forever.`):

```go
//
// It runs a second pass for buddy mock interviews (buddy.SendDueMockReminders): the same
// offsets, windows and claim order, on the same schedule and mail transport, so the
// feature needs no unit of its own. A failure in one pass does not skip the other, and the
// run exits non-zero if either could not read its work.
```
  - add `"github.com/strelov1/freehire/internal/engage/buddy"` to the imports;
  - replace lines 70-84 (`svc := mentorship.New(` through `return 0`) with:

```go
	q := db.New(pool)
	svc := mentorship.New(
		mentorship.NewQueriesRepository(q, pool),
		mentorship.Config{
			Notifier: mentorship.NewMailNotifier(sender, cfg.NotifyEmailFrom,
				cfg.FrontendOrigin+"/my/sessions"),
		},
	)
	// The buddy pass needs no messaging core: reminders never start a conversation.
	buddyRepo := buddy.NewQueriesRepository(q)
	mocks := buddy.NewMockService(buddy.New(buddyRepo, nil), buddyRepo,
		buddy.NewMockMailNotifier(q, sender, cfg.NotifyEmailFrom, cfg.FrontendOrigin))

	exit := 0
	stats, err := svc.SendDueReminders(ctx, maxPerRun())
	if err != nil {
		log.Printf("mentorship-remind: %v", err)
		exit = 1
	} else {
		log.Printf("mentorship-remind: sent=%d failed=%d", stats.Sent, stats.Failed)
	}
	mockStats, err := mocks.SendDueMockReminders(ctx, maxPerRun())
	if err != nil {
		log.Printf("mentorship-remind: buddy mocks: %v", err)
		exit = 1
	} else {
		log.Printf("mentorship-remind: buddy mocks sent=%d failed=%d", mockStats.Sent, mockStats.Failed)
	}
	return exit
```

- [ ] 9.6 Verify and commit:

```bash
gofmt -l internal/engage/buddy cmd/mentorship-remind && go build ./cmd/mentorship-remind/ && go vet ./... && go test ./... && go vet -tags=integration ./internal/engage/buddy/
git add internal/engage/buddy/mock_remind.go internal/engage/buddy/mock_remind_integration_test.go internal/engage/buddy/mock_service.go internal/engage/buddy/mock_repository.go cmd/mentorship-remind/main.go
git commit -m "Remind buddy mock participants from mentorship-remind"
```

## 10. HTTP handlers and routes

`web/static/openapi.yaml` describes no buddy route (`grep -n buddies web/static/openapi.yaml` prints nothing) and mocks are not exposed over MCP, OJCP or the public API (design Non-Goals), so the contract file is not touched.

**Files:**
- Create: `internal/api/handler/buddy_mocks.go`, `internal/api/handler/buddy_mocks_integration_test.go`
- Modify: `internal/api/handler/buddies.go:21-41` (struct, constructor, register), `:115-124` (Leave); `internal/api/handler/conversations.go:19-25` (struct, constructor), `:152-155` (Block); `internal/api/handler/mentorship.go:344-354` (extract `slotResponses`); `internal/api/handler/handler.go:946-951` (composition); `internal/api/handler/buddies_integration_test.go:74-77` (test env)

**Interfaces:**
- Consumes: every `MockService` method (Tasks 3-7), `Mock.EntryFor`; `slotWindow(c) (from, to time.Time, err error)`, `slotResponse`, `offsetLabel`, `availabilityRuleRequest.toRule() (slots.Rule, error)`, `weeklyAvailabilityRequest`, `toAvailabilityResponse(slots.Rule) availabilityRuleResponse` (mentorship handlers, same package); `requireUserID`, `pathID`, `buddyError`.
- Produces (all `mw.cookie`; `/api/v1` prefix):
  - `GET /me/buddies/mocks` → `{"data": {"upcoming": [mock], "past": [mock]}}`
  - `GET|PUT /me/buddies/mocks/settings` → `{"data": {timezone, meeting_url, topics, paused} | null}`
  - `GET /me/buddies/mocks/availability` → `{"data": [rule]}`; `PUT …/availability/weekly` `{rules}` → 204; `POST …/availability/overrides` `{date, start, end}` → 201 `{"data": rule}`; `DELETE …/availability/:id` → 204
  - `POST /me/buddies/mocks/:id/cancel` → 204; `POST /me/buddies/mocks/:id/review` `{rating, comment}` → 204
  - `GET /me/buddies/profiles/:handle/mock-slots?from&to&timezone` → `{"data": [slot], "meta": {timezone, count, topics}}`
  - `POST /me/buddies/profiles/:handle/mocks` `{starts_at, topic, timezone}` → 201 `{"data": mock}`
  - `GET /me/buddies/profiles/:handle/mock-reviews` → `{"data": {completed, rating_count, rating_avg, recent: [{rating, comment, topic, day}]}}`
  - `mock` = `{id, role, counterpart|null, starts_at, ends_at, topic, status, meeting_url|null, can_cancel, can_review, my_review|null, their_review|null}`
  - Status mapping: 422 invalid settings/topic/review/not cancellable/rule; 409 slot unavailable, already reviewed; 429 guest limit; 404 not found (unknown/left/blocked/self handle, unknown mock); 403 not a member; 500 a stored zone that no longer resolves.
  - `newBuddyHandlers(svc *buddy.Service, mocks *buddy.MockService) *buddyHandlers`; `newConversationHandlers(msg *messaging.Service, mocks *buddy.MockService) *conversationHandlers`; `slotResponses(ivs []slots.Interval) []slotResponse`.

- [ ] 10.1 Write the failing handler tests `internal/api/handler/buddy_mocks_integration_test.go`:

```go
//go:build integration

package handler

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

// openMockHost gives user settings and a whole week of hours over the HTTP contract, and
// returns their meeting link.
func (e *buddyEnv) openMockHost(user string) string {
	e.t.Helper()
	link := "https://meet.example.test/" + user + "-room"
	e.want(http.StatusOK, http.MethodPut, "/api/v1/me/buddies/mocks/settings", user, map[string]any{
		"timezone": "UTC", "meeting_url": link, "topics": []string{"coding", "behavioral"}, "paused": false,
	})
	rules := make([]map[string]any, 0, 7)
	for day := 0; day < 7; day++ {
		rules = append(rules, map[string]any{"weekday": day, "start": "00:00", "end": "24:00"})
	}
	e.raw(http.StatusNoContent, http.MethodPut, "/api/v1/me/buddies/mocks/availability/weekly", user, map[string]any{"rules": rules})
	return link
}

func TestBuddyMockSettingsAreValidated(t *testing.T) {
	env := newBuddyEnv(t, "a")
	env.want(http.StatusUnauthorized, http.MethodGet, "/api/v1/me/buddies/mocks", "", nil)
	env.join("a")

	env.want(http.StatusUnprocessableEntity, http.MethodPut, "/api/v1/me/buddies/mocks/settings", "a", map[string]any{
		"timezone": "Mars/Olympus", "meeting_url": "https://meet.example.test/x", "topics": []string{"coding"},
	})
	env.want(http.StatusUnprocessableEntity, http.MethodPut, "/api/v1/me/buddies/mocks/settings", "a", map[string]any{
		"timezone": "UTC", "meeting_url": "http://meet.example.test/x", "topics": []string{"coding"},
	})
	if out := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks/settings", "a", nil); out["data"] != nil {
		t.Fatalf("refused settings were stored: %v", out)
	}
}

func TestBuddyMockBookingOverHTTP(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest", "late")
	hostHandle := env.join("host")
	guestHandle := env.join("guest")
	env.join("late")
	link := env.openMockHost("host")
	slotsPath := "/api/v1/me/buddies/profiles/" + hostHandle + "/mock-slots?timezone=Asia/Tokyo"

	if raw := env.raw(http.StatusOK, http.MethodGet, slotsPath, "guest", nil); strings.Contains(raw, link) {
		t.Fatal("the slot listing carries the meeting link")
	}
	listing := env.want(http.StatusOK, http.MethodGet, slotsPath, "guest", nil)
	if meta := listing["meta"].(map[string]any); meta["timezone"] != "Asia/Tokyo" || len(meta["topics"].([]any)) != 2 {
		t.Fatalf("meta = %v", meta)
	}
	first := listing["data"].([]any)[0].(map[string]any)["starts_at"].(string)
	body := map[string]any{"starts_at": first, "topic": "coding", "timezone": "Asia/Tokyo"}
	bookPath := "/api/v1/me/buddies/profiles/" + hostHandle + "/mocks"

	mock := env.want(http.StatusCreated, http.MethodPost, bookPath, "guest", body)["data"].(map[string]any)
	if mock["role"] != "guest" || mock["counterpart"] != hostHandle || mock["meeting_url"] != link || mock["can_cancel"] != true {
		t.Fatalf("booked = %v", mock)
	}
	env.want(http.StatusConflict, http.MethodPost, bookPath, "late", body)
	env.want(http.StatusUnprocessableEntity, http.MethodPost, bookPath, "late",
		map[string]any{"starts_at": first, "topic": "system_design", "timezone": "UTC"})

	hostRaw := env.raw(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", "host", nil)
	for _, leak := range []string{"guest@example.test", strconv.FormatInt(env.users["guest"], 10)} {
		if strings.Contains(hostRaw, leak) {
			t.Fatalf("the host's list carries %q", leak)
		}
	}
	upcoming := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", "host", nil)["data"].(map[string]any)["upcoming"].([]any)
	if up := upcoming[0].(map[string]any); up["role"] != "host" || up["counterpart"] != guestHandle {
		t.Fatalf("host's upcoming = %v", up)
	}

	env.raw(http.StatusNoContent, http.MethodPost, "/api/v1/me/buddies/mocks/"+mock["id"].(string)+"/cancel", "guest", nil)
	past := env.want(http.StatusOK, http.MethodGet, "/api/v1/me/buddies/mocks", "guest", nil)["data"].(map[string]any)["past"].([]any)
	if p := past[0].(map[string]any); p["status"] != "cancelled" || p["meeting_url"] != nil {
		t.Fatalf("guest's past = %v", p)
	}
	env.want(http.StatusNotFound, http.MethodPost, "/api/v1/me/buddies/mocks/not-a-uuid/cancel", "guest", nil)
}

func TestBuddyMockRoutesHideAMemberAcrossABlock(t *testing.T) {
	env := newBuddyEnv(t, "host", "guest")
	hostHandle := env.join("host")
	env.join("guest")
	env.openMockHost("host")
	if _, err := env.pool.Exec(context.Background(),
		`INSERT INTO user_blocks (blocker_id, blocked_id) VALUES ($1, $2)`, env.users["host"], env.users["guest"]); err != nil {
		t.Fatal(err)
	}
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/"+hostHandle+"/mock-slots", "guest", nil)
	env.want(http.StatusNotFound, http.MethodGet, "/api/v1/me/buddies/profiles/"+hostHandle+"/mock-reviews", "guest", nil)
}
```

- [ ] 10.2 Run: `go vet -tags=integration ./internal/api/handler/`. Expected: PASS (the test only calls existing helpers); then `go test -tags=integration ./internal/api/handler/ -run 'TestBuddyMock' -count=1`. Expected: FAIL — 404 on `/api/v1/me/buddies/mocks/settings` (no route).

- [ ] 10.3 Extract the slot rendering mentorship already does so both endpoints share it. In `internal/api/handler/mentorship.go` replace lines 344-354

```go
	out := make([]slotResponse, 0, len(result.Slots))
	for _, s := range result.Slots {
		_, offset := s.Start.Zone()
		out = append(out, slotResponse{
			StartsAt:   s.Start.UTC(),
			EndsAt:     s.End.UTC(),
			LocalStart: s.Start.Format(time.RFC3339),
			LocalEnd:   s.End.Format(time.RFC3339),
			UTCOffset:  offsetLabel(offset),
		})
	}
```
with
```go
	out := slotResponses(result.Slots)
```
and add after `GetMentorSlots`:

```go
// slotResponses renders engine slots for the wire: the instant, the wall clock and the
// offset, for the reasons slotResponse gives. Buddy mock slots use it too.
func slotResponses(ivs []slots.Interval) []slotResponse {
	out := make([]slotResponse, 0, len(ivs))
	for _, s := range ivs {
		_, offset := s.Start.Zone()
		out = append(out, slotResponse{
			StartsAt:   s.Start.UTC(),
			EndsAt:     s.End.UTC(),
			LocalStart: s.Start.Format(time.RFC3339),
			LocalEnd:   s.End.Format(time.RFC3339),
			UTCOffset:  offsetLabel(offset),
		})
	}
	return out
}
```

- [ ] 10.4 Create `internal/api/handler/buddy_mocks.go`:

```go
package handler

import (
	"errors"
	"time"

	"github.com/gofiber/fiber/v2"
	"github.com/google/uuid"

	"github.com/strelov1/freehire/internal/engage/buddy"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// registerMocks mounts the buddy mock-interview routes (buddy.MockService). Cookie-only,
// like every buddy route, so a leaked API key cannot book or read a member's mocks.
func (h *buddyHandlers) registerMocks(api fiber.Router, mw middleware) {
	api.Get("/me/buddies/mocks", mw.cookie, h.ListMocks)
	api.Get("/me/buddies/mocks/settings", mw.cookie, h.GetMockSettings)
	api.Put("/me/buddies/mocks/settings", mw.cookie, h.SaveMockSettings)
	api.Get("/me/buddies/mocks/availability", mw.cookie, h.GetMockAvailability)
	api.Put("/me/buddies/mocks/availability/weekly", mw.cookie, h.ReplaceMockWeekly)
	api.Post("/me/buddies/mocks/availability/overrides", mw.cookie, h.AddMockOverride)
	api.Delete("/me/buddies/mocks/availability/:id", mw.cookie, h.DeleteMockRule)
	api.Post("/me/buddies/mocks/:id/cancel", mw.cookie, h.CancelMock)
	api.Post("/me/buddies/mocks/:id/review", mw.cookie, h.ReviewMock)
	api.Get("/me/buddies/profiles/:handle/mock-slots", mw.cookie, h.MockSlots)
	api.Post("/me/buddies/profiles/:handle/mocks", mw.cookie, h.BookMock)
	api.Get("/me/buddies/profiles/:handle/mock-reviews", mw.cookie, h.MockReviews)
}

// mockError renders a mock sentinel as the status its declaration documents, and anything
// else as buddyError does.
func mockError(err error) error {
	switch {
	case errors.Is(err, buddy.ErrInvalidMockSettings), errors.Is(err, buddy.ErrInvalidTopic),
		errors.Is(err, buddy.ErrInvalidReview), errors.Is(err, buddy.ErrMockNotCancellable),
		errors.Is(err, slots.ErrInvalidTimeOfDay), errors.Is(err, slots.ErrInvalidDate),
		errors.Is(err, slots.ErrInvalidWeekday), errors.Is(err, slots.ErrInvalidSpan):
		return fiber.NewError(fiber.StatusUnprocessableEntity, err.Error())
	case errors.Is(err, buddy.ErrSlotUnavailable):
		return fiber.NewError(fiber.StatusConflict, "that time is no longer available")
	case errors.Is(err, buddy.ErrAlreadyReviewed):
		return fiber.NewError(fiber.StatusConflict, "you already reviewed this mock interview")
	case errors.Is(err, buddy.ErrGuestMockLimit):
		return fiber.NewError(fiber.StatusTooManyRequests, "you already have 3 upcoming mock interviews as a guest")
	case errors.Is(err, buddy.ErrMockNotFound):
		return fiber.NewError(fiber.StatusNotFound, "mock interview not found")
	case errors.Is(err, slots.ErrNoOwnerZone):
		// A stored zone that no longer resolves is a broken row, not a bad request.
		return fiber.NewError(fiber.StatusInternalServerError, "this member's schedule is misconfigured")
	}
	return buddyError(err)
}

type buddyMockSettingsBody struct {
	Timezone   string   `json:"timezone"`
	MeetingURL string   `json:"meeting_url"`
	Topics     []string `json:"topics"`
	Paused     bool     `json:"paused"`
}

func toBuddyMockSettingsBody(s buddy.MockSettings) buddyMockSettingsBody {
	return buddyMockSettingsBody{Timezone: s.Timezone, MeetingURL: s.MeetingURL, Topics: s.Topics, Paused: s.Paused}
}

// GetMockSettings is the caller's own settings, or null before the first save.
func (h *buddyHandlers) GetMockSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	s, ok, err := h.mocks.Settings(c.Context(), userID)
	if err != nil {
		return mockError(err)
	}
	if !ok {
		return c.JSON(fiber.Map{"data": nil})
	}
	return c.JSON(fiber.Map{"data": toBuddyMockSettingsBody(s)})
}

// SaveMockSettings stores the caller's settings whole.
func (h *buddyHandlers) SaveMockSettings(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in buddyMockSettingsBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	s, err := h.mocks.SaveSettings(c.Context(), userID, buddy.MockSettings{
		Timezone: in.Timezone, MeetingURL: in.MeetingURL, Topics: in.Topics, Paused: in.Paused,
	})
	if err != nil {
		return mockError(err)
	}
	return c.JSON(fiber.Map{"data": toBuddyMockSettingsBody(s)})
}

// GetMockAvailability is the caller's whole schedule, in the mentor schedule's wire shape.
func (h *buddyHandlers) GetMockAvailability(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	rules, err := h.mocks.Availability(c.Context(), userID)
	if err != nil {
		return mockError(err)
	}
	out := make([]availabilityRuleResponse, 0, len(rules))
	for _, rule := range rules {
		out = append(out, toAvailabilityResponse(rule))
	}
	return c.JSON(fiber.Map{"data": out})
}

// ReplaceMockWeekly swaps the whole recurring week, as the mentor schedule's PUT does.
func (h *buddyHandlers) ReplaceMockWeekly(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req weeklyAvailabilityRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	rules := make([]slots.Rule, 0, len(req.Rules))
	for _, raw := range req.Rules {
		if raw.Date != nil {
			return fiber.NewError(fiber.StatusUnprocessableEntity,
				"the weekly schedule takes weekday rules; use the overrides endpoint for a date")
		}
		rule, err := raw.toRule()
		if err != nil {
			return err
		}
		rules = append(rules, rule)
	}
	if err := h.mocks.ReplaceWeekly(c.Context(), userID, rules); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

// AddMockOverride adds one dated row and answers it with its id, so it can be removed.
func (h *buddyHandlers) AddMockOverride(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var req availabilityRuleRequest
	if err := c.BodyParser(&req); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if req.Date == nil {
		return fiber.NewError(fiber.StatusUnprocessableEntity, "an override needs a date")
	}
	rule, err := req.toRule()
	if err != nil {
		return err
	}
	stored, err := h.mocks.AddOverride(c.Context(), userID, rule)
	if err != nil {
		return mockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toAvailabilityResponse(stored)})
}

// DeleteMockRule removes one row of the caller's own schedule.
func (h *buddyHandlers) DeleteMockRule(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := pathID(c)
	if err != nil {
		return err
	}
	if err := h.mocks.DeleteRule(c.Context(), userID, id); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyMockReviewResponse struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// buddyMockResponse is one mock as its participant sees it. It names the other side by
// handle only (null once they left or a block stands between them), and carries no
// account id and no address.
type buddyMockResponse struct {
	ID          string                   `json:"id"`
	Role        string                   `json:"role"`
	Counterpart *string                  `json:"counterpart"`
	StartsAt    time.Time                `json:"starts_at"`
	EndsAt      time.Time                `json:"ends_at"`
	Topic       string                   `json:"topic"`
	Status      string                   `json:"status"`
	MeetingURL  *string                  `json:"meeting_url"`
	CanCancel   bool                     `json:"can_cancel"`
	CanReview   bool                     `json:"can_review"`
	MyReview    *buddyMockReviewResponse `json:"my_review"`
	TheirReview *buddyMockReviewResponse `json:"their_review"`
}

func toBuddyMockResponse(e buddy.MockEntry) buddyMockResponse {
	out := buddyMockResponse{
		ID: e.ID.String(), Role: e.Role, StartsAt: e.StartsAt.UTC(), EndsAt: e.EndsAt.UTC(),
		Topic: e.Topic, Status: e.Status, CanCancel: e.CanCancel, CanReview: e.CanReview,
	}
	if e.Counterpart != "" {
		out.Counterpart = &e.Counterpart
	}
	if e.MeetingURL != "" {
		out.MeetingURL = &e.MeetingURL
	}
	if e.MyReview != nil {
		out.MyReview = &buddyMockReviewResponse{Rating: e.MyReview.Rating, Comment: e.MyReview.Comment}
	}
	if e.TheirReview != nil {
		out.TheirReview = &buddyMockReviewResponse{Rating: e.TheirReview.Rating, Comment: e.TheirReview.Comment}
	}
	return out
}

func toBuddyMockResponses(entries []buddy.MockEntry) []buddyMockResponse {
	out := make([]buddyMockResponse, 0, len(entries))
	for _, e := range entries {
		out = append(out, toBuddyMockResponse(e))
	}
	return out
}

// ListMocks is the caller's own mocks, upcoming and past.
func (h *buddyHandlers) ListMocks(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	list, err := h.mocks.List(c.Context(), userID)
	if err != nil {
		return mockError(err)
	}
	return c.JSON(fiber.Map{"data": fiber.Map{
		"upcoming": toBuddyMockResponses(list.Upcoming),
		"past":     toBuddyMockResponses(list.Past),
	}})
}

// MockSlots lists a member's open slots in the viewer's zone, with the topics they offer.
// The meeting link is never part of it.
func (h *buddyHandlers) MockSlots(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	from, to, err := slotWindow(c)
	if err != nil {
		return err
	}
	res, err := h.mocks.OpenSlots(c.Context(), userID, c.Params("handle"), from, to, c.Query("timezone"))
	if err != nil {
		return mockError(err)
	}
	out := slotResponses(res.Slots)
	return c.JSON(fiber.Map{
		"data": out,
		"meta": fiber.Map{"timezone": res.Zone, "count": len(out), "topics": res.Topics},
	})
}

type buddyBookMockBody struct {
	StartsAt time.Time `json:"starts_at"`
	Topic    string    `json:"topic"`
	Timezone string    `json:"timezone"`
}

// BookMock takes one of a member's slots for the caller, as guest.
func (h *buddyHandlers) BookMock(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	var in buddyBookMockBody
	if err := c.BodyParser(&in); err != nil || in.StartsAt.IsZero() || in.Topic == "" {
		return fiber.NewError(fiber.StatusBadRequest, "starts_at and topic are required")
	}
	m, err := h.mocks.Book(c.Context(), buddy.BookInput{
		HostHandle: c.Params("handle"), GuestUserID: userID, StartsAt: in.StartsAt,
		Topic: in.Topic, GuestZone: in.Timezone,
	})
	if err != nil {
		return mockError(err)
	}
	return c.Status(fiber.StatusCreated).JSON(fiber.Map{"data": toBuddyMockResponse(m.EntryFor(userID, time.Now()))})
}

// mockIDParam reads the mock id from the path. A malformed one is 404: an id that cannot
// exist and one that does not are the same answer.
func mockIDParam(c *fiber.Ctx) (uuid.UUID, error) {
	id, err := uuid.Parse(c.Params("id"))
	if err != nil {
		return uuid.UUID{}, fiber.NewError(fiber.StatusNotFound, "mock interview not found")
	}
	return id, nil
}

// CancelMock cancels one of the caller's mocks before it starts.
func (h *buddyHandlers) CancelMock(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := mockIDParam(c)
	if err != nil {
		return err
	}
	if err := h.mocks.Cancel(c.Context(), userID, id); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyMockReviewBody struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
}

// ReviewMock rates the other participant of one of the caller's mocks.
func (h *buddyHandlers) ReviewMock(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	id, err := mockIDParam(c)
	if err != nil {
		return err
	}
	var in buddyMockReviewBody
	if err := c.BodyParser(&in); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "invalid body")
	}
	if err := h.mocks.Review(c.Context(), userID, id, in.Rating, in.Comment); err != nil {
		return mockError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
}

type buddyRevealedReviewResponse struct {
	Rating  int    `json:"rating"`
	Comment string `json:"comment"`
	Topic   string `json:"topic"`
	Day     string `json:"day"`
}

type buddyMockSummaryResponse struct {
	Completed   int                           `json:"completed"`
	RatingCount int                           `json:"rating_count"`
	RatingAvg   float64                       `json:"rating_avg"`
	Recent      []buddyRevealedReviewResponse `json:"recent"`
}

// MockReviews is what a member page shows about that member's mocks.
func (h *buddyHandlers) MockReviews(c *fiber.Ctx) error {
	userID, err := requireUserID(c)
	if err != nil {
		return err
	}
	sum, err := h.mocks.MemberSummary(c.Context(), userID, c.Params("handle"))
	if err != nil {
		return mockError(err)
	}
	out := buddyMockSummaryResponse{
		Completed: sum.Completed, RatingCount: sum.RatingCount, RatingAvg: sum.RatingAvg,
		Recent: make([]buddyRevealedReviewResponse, 0, len(sum.Recent)),
	}
	for _, r := range sum.Recent {
		out.Recent = append(out.Recent, buddyRevealedReviewResponse{Rating: r.Rating, Comment: r.Comment, Topic: r.Topic, Day: r.Day})
	}
	return c.JSON(fiber.Map{"data": out})
}
```

- [ ] 10.5 Wire the handlers. In `internal/api/handler/buddies.go` replace

```go
type buddyHandlers struct {
	buddy *buddy.Service
}

func newBuddyHandlers(svc *buddy.Service) *buddyHandlers {
	return &buddyHandlers{buddy: svc}
}
```
with
```go
type buddyHandlers struct {
	buddy *buddy.Service
	mocks *buddy.MockService
}

func newBuddyHandlers(svc *buddy.Service, mocks *buddy.MockService) *buddyHandlers {
	return &buddyHandlers{buddy: svc, mocks: mocks}
}
```
add `	h.registerMocks(api, mw)` as the last line of `register`, and in `Leave` replace

```go
	if err := h.buddy.Leave(c.Context(), userID); err != nil {
		return buddyError(err)
	}
	return c.SendStatus(fiber.StatusNoContent)
```
with
```go
	if err := h.buddy.Leave(c.Context(), userID); err != nil {
		return buddyError(err)
	}
	// The leave cancelled the member's future mocks in its own statement; this tells the
	// other participants.
	h.mocks.AnnounceCancellations(c.Context(), userID)
	return c.SendStatus(fiber.StatusNoContent)
```
In `internal/api/handler/conversations.go` replace

```go
type conversationHandlers struct {
	msg *messaging.Service
}

func newConversationHandlers(msg *messaging.Service) *conversationHandlers {
	return &conversationHandlers{msg: msg}
}
```
with
```go
type conversationHandlers struct {
	msg   *messaging.Service
	mocks *buddy.MockService
}

func newConversationHandlers(msg *messaging.Service, mocks *buddy.MockService) *conversationHandlers {
	return &conversationHandlers{msg: msg, mocks: mocks}
}
```
and replace

```go
func (h *conversationHandlers) Block(c *fiber.Ctx) error {
	return h.byID(c, h.msg.Block)
}
```
with
```go
//
// A block also cancels every future buddy mock between the two, inside the block's own
// statement (a trigger the buddy migration puts on user_blocks, because the core never
// imports a feature). This layer, the only one that sees both, tells the other side.
func (h *conversationHandlers) Block(c *fiber.Ctx) error {
	return h.byID(c, func(ctx context.Context, userID, id int64) error {
		if err := h.msg.Block(ctx, userID, id); err != nil {
			return err
		}
		h.mocks.AnnounceCancellations(ctx, userID)
		return nil
	})
}
```
(the leading `//` line continues the existing doc comment above `Block`). In `internal/api/handler/handler.go` replace lines 946-951

```go
	// Conversations are the messaging core's; each persona a participant may speak as is
	// registered here.
	buddyRepo := buddy.NewQueriesRepository(queries)
	messagingSvc := messaging.New(messaging.NewQueriesRepository(queries), buddy.NewPersona(buddyRepo))
	conversationH := newConversationHandlers(messagingSvc)
	buddyH := newBuddyHandlers(buddy.New(buddyRepo, messagingSvc))
```
with
```go
	// Conversations are the messaging core's; each persona a participant may speak as is
	// registered here.
	buddyRepo := buddy.NewQueriesRepository(queries)
	messagingSvc := messaging.New(messaging.NewQueriesRepository(queries), buddy.NewPersona(buddyRepo))
	buddySvc := buddy.New(buddyRepo, messagingSvc)
	// Buddy mock interviews. Without a mail transport the notices still reach the
	// notification center. A properly nil interface, never a typed-nil *Client.
	var mockMail emailnotify.Sender
	if mailClient != nil && cfg.NotifyEmailFrom != "" {
		mockMail = mailClient
	}
	buddyMocks := buddy.NewMockService(buddySvc, buddyRepo,
		buddy.NewMockMailNotifier(queries, mockMail, cfg.NotifyEmailFrom, cfg.FrontendOrigin))
	conversationH := newConversationHandlers(messagingSvc, buddyMocks)
	buddyH := newBuddyHandlers(buddySvc, buddyMocks)
```
In `internal/api/handler/buddies_integration_test.go` replace

```go
	newBuddyHandlers(buddy.New(repo, msg)).register(env.app.Group("/api/v1"), mw)
	newConversationHandlers(msg).register(env.app.Group("/api/v1"), mw)
```
with
```go
	svc := buddy.New(repo, msg)
	mocks := buddy.NewMockService(svc, repo, nil)
	newBuddyHandlers(svc, mocks).register(env.app.Group("/api/v1"), mw)
	newConversationHandlers(msg, mocks).register(env.app.Group("/api/v1"), mw)
```

- [ ] 10.6 Run: `go build ./... && go vet -tags=integration ./internal/api/handler/ && go test -tags=integration ./internal/api/handler/ -run 'TestBuddy|Mentor' -count=1`. Expected: `ok` (the existing buddy and mentorship handler tests pass with the new wiring).

- [ ] 10.7 Verify and commit:

```bash
gofmt -l internal/api/handler && go vet ./... && go test ./... && go vet -tags=integration ./...
git add internal/api/handler/buddy_mocks.go internal/api/handler/buddy_mocks_integration_test.go internal/api/handler/buddies.go internal/api/handler/conversations.go internal/api/handler/mentorship.go internal/api/handler/handler.go internal/api/handler/buddies_integration_test.go
git commit -m "Serve buddy mock interviews over HTTP"
```

## 11. Shared Svelte components take their API as props

Mentor pages keep their behaviour exactly; only where the calls come from moves. `MentorBookingView`'s booking form (note, sign-in bounce, book, redirect) moves verbatim into a new `MentorBookingForm.svelte`, passed back in as a snippet, so the buddy page can pass a topic picker instead.

Before any web command in Tasks 11-12, install the linked design system first (web/AGENTS.md): `pnpm --dir design-system install && pnpm --dir web install`.

**Files:**
- Modify: `web/src/lib/types.ts` (after `MentorAvailabilityRule`, line 2058), `web/src/lib/components/MentorScheduleEditor.svelte` (script, lines 1-87; empty-week paragraph), `web/src/lib/components/MentorBookingView.svelte` (imports, props, `load`, the "taking the hour" block, the chosen-slot markup and footer), `web/src/routes/my/mentorship/schedule/+page.svelte`, `web/src/routes/mentors/[slug]/+page.svelte`
- Create: `web/src/lib/components/MentorBookingForm.svelte`, `web/src/lib/components/MentorScheduleEditor.spec.ts`

**Interfaces:**
- Produces (TS): `export interface AvailabilityApi { replaceWeek(rows: { weekday: number; start: string; end: string }[]): Promise<void>; reload(): Promise<MentorAvailabilityRule[]>; addOverride(date: string, start: string, end: string): Promise<MentorAvailabilityRule>; remove(id: number): Promise<void>; }`
- `MentorScheduleEditor` props: `{ rules: MentorAvailabilityRule[] ($bindable); schedule: AvailabilityApi; emptyMessage: string }`
- `MentorBookingView` props: `{ loadSlots: (from: string, to: string, timezone: string, signal: AbortSignal) => Promise<MentorSlots>; sessionMinutes: number; counterpart: string; booker: Snippet<[{ slot: MentorSlot; day: string; zone: string; refused: () => void }]> }`
- `MentorBookingForm` props: `{ mentor: Mentor; slot: MentorSlot; zone: string; refused: () => void }`
- Consumes: `api.replaceWeeklyAvailability`, `api.myMentorAvailability`, `api.addAvailabilityOverride`, `api.deleteAvailabilityRule`, `api.getMentorSlots`, `api.bookMentorSession` (`web/src/lib/api.ts`).

- [ ] 11.1 Add the type. In `web/src/lib/types.ts`, right after the closing `}` of `export interface MentorAvailabilityRule` (line 2058), insert:

```ts
/** The four calls an availability editor makes. The mentor cabinet and the buddy mock page
 *  share one editor over two APIs, so the editor takes them rather than importing one. */
export interface AvailabilityApi {
  replaceWeek(rows: { weekday: number; start: string; end: string }[]): Promise<void>;
  reload(): Promise<MentorAvailabilityRule[]>;
  addOverride(date: string, start: string, end: string): Promise<MentorAvailabilityRule>;
  remove(id: number): Promise<void>;
}
```

- [ ] 11.2 Write the failing component test `web/src/lib/components/MentorScheduleEditor.spec.ts`:

```ts
import { fireEvent, render, screen, waitFor } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import MentorScheduleEditor from './MentorScheduleEditor.svelte';
import type { AvailabilityApi, MentorAvailabilityRule } from '#lib/types.js';

const tuesday: MentorAvailabilityRule = { id: 1, weekday: 2, date: null, start: '18:00', end: '20:00', closure: false };
const away: MentorAvailabilityRule = { id: 7, weekday: null, date: '2026-12-01', start: '00:00', end: '00:00', closure: true };

function fakeSchedule(): AvailabilityApi & { [K in keyof AvailabilityApi]: ReturnType<typeof vi.fn> } {
  return {
    replaceWeek: vi.fn().mockResolvedValue(undefined),
    reload: vi.fn().mockResolvedValue([tuesday]),
    addOverride: vi.fn().mockResolvedValue(away),
    remove: vi.fn().mockResolvedValue(undefined),
  };
}

describe('MentorScheduleEditor', () => {
  // The editor serves the mentor cabinet and the buddy mock page: every write must go
  // through the schedule it was handed, never through one API it knows by name.
  it('saves the week through the schedule it was given, then re-reads it', async () => {
    const schedule = fakeSchedule();
    render(MentorScheduleEditor, { rules: [tuesday], schedule, emptyMessage: 'Nothing yet.' });

    await fireEvent.click(screen.getByRole('button', { name: 'Edit' }));
    await fireEvent.click(screen.getByRole('button', { name: 'Save the week' }));

    await waitFor(() => expect(schedule.reload).toHaveBeenCalledOnce());
    expect(schedule.replaceWeek).toHaveBeenCalledWith([{ weekday: 2, start: '18:00', end: '20:00' }]);
  });

  it('removes an exception through the schedule it was given', async () => {
    const schedule = fakeSchedule();
    render(MentorScheduleEditor, { rules: [tuesday, away], schedule, emptyMessage: 'Nothing yet.' });

    await fireEvent.click(screen.getByRole('button', { name: 'Remove' }));

    await waitFor(() => expect(schedule.remove).toHaveBeenCalledWith(7));
  });

  it('says what an empty week means in the words of the page that hosts it', () => {
    render(MentorScheduleEditor, { rules: [], schedule: fakeSchedule(), emptyMessage: 'Nobody can book you yet.' });
    expect(screen.getByText('Nobody can book you yet.')).toBeTruthy();
  });
});
```
Run: `pnpm --dir web exec vitest run src/lib/components/MentorScheduleEditor.spec.ts`. Expected: FAIL — the first test sees no call on `schedule.replaceWeek` (the component still calls `api.replaceWeeklyAvailability`), the third finds no `Nobody can book you yet.`.

- [ ] 11.3 Refactor `web/src/lib/components/MentorScheduleEditor.svelte`. Replace the top of the script

```svelte
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Badge, Button, Card } from '#lib/ui/index.js';
  import { splitAvailability, weekdayLabel, weekdayOrder } from '#lib/mentorship.js';
  import type { MentorAvailabilityRule } from '#lib/types.js';

  let { rules = $bindable() }: { rules: MentorAvailabilityRule[] } = $props();
```
with
```svelte
  import { errorMessage } from '#lib/utils.js';
  import { Badge, Button, Card } from '#lib/ui/index.js';
  import { splitAvailability, weekdayLabel, weekdayOrder } from '#lib/mentorship.js';
  import type { AvailabilityApi, MentorAvailabilityRule } from '#lib/types.js';

  let {
    rules = $bindable(),
    schedule,
    emptyMessage,
  }: {
    rules: MentorAvailabilityRule[];
    /** Where the edits go: the mentor schedule's endpoints or the buddy mock ones. */
    schedule: AvailabilityApi;
    /** What an empty week means on the page hosting the editor. */
    emptyMessage: string;
  } = $props();
```
then replace the four calls: `await api.replaceWeeklyAvailability(draft);` → `await schedule.replaceWeek(draft);`, `rules = await api.myMentorAvailability();` → `rules = await schedule.reload();`, `const added = await api.addAvailabilityOverride(overrideDate, overrideStart, end);` → `const added = await schedule.addOverride(overrideDate, overrideStart, end);`, `await api.deleteAvailabilityRule(id);` → `await schedule.remove(id);`. In the markup replace

```svelte
      <p class="text-muted-foreground text-sm">
        No hours yet — until you add some, your profile offers nothing to book.
      </p>
```
with
```svelte
      <p class="text-muted-foreground text-sm">{emptyMessage}</p>
```

- [ ] 11.4 Pass the mentor API from the mentor cabinet. Replace `web/src/routes/my/mentorship/schedule/+page.svelte`'s script and editor line:

```svelte
<script lang="ts">
  import { api } from '#lib/api.js';
  import MentorScheduleCalendar from '#lib/components/MentorScheduleCalendar.svelte';
  import MentorScheduleEditor from '#lib/components/MentorScheduleEditor.svelte';
  import MentorSessionSettings from '#lib/components/MentorSessionSettings.svelte';
  import type { AvailabilityApi } from '#lib/types.js';
  import type { PageData } from './$types';

  let { data }: { data: PageData } = $props();

  // Local copies so the editors can write back without a round trip through `load`.
  let availability = $state(data.availability);
  let profile = $state(data.profile);

  const mentorSchedule: AvailabilityApi = {
    replaceWeek: api.replaceWeeklyAvailability,
    reload: api.myMentorAvailability,
    addOverride: api.addAvailabilityOverride,
    remove: api.deleteAvailabilityRule,
  };
</script>
```
and `<MentorScheduleEditor bind:rules={availability} />` with

```svelte
  <MentorScheduleEditor
    bind:rules={availability}
    schedule={mentorSchedule}
    emptyMessage="No hours yet — until you add some, your profile offers nothing to book."
  />
```
Run: `pnpm --dir web exec vitest run src/lib/components/MentorScheduleEditor.spec.ts`. Expected: PASS.

- [ ] 11.5 Move the mentor booking form out of `MentorBookingView`. Create `web/src/lib/components/MentorBookingForm.svelte` (the logic and markup are the ones that lived in `MentorBookingView`, with `chosen` → `slot` and `reload()` → `refused()`):

```svelte
<script lang="ts">
  import { goto } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { signinUrl } from '#lib/signin.js';
  import { Button } from '#lib/ui/index.js';
  import type { Mentor, MentorSlot } from '#lib/types.js';

  // Taking one of a mentor's hours, rendered by MentorBookingView for the chosen slot.
  let {
    mentor,
    slot,
    zone,
    refused,
  }: { mentor: Mentor; slot: MentorSlot; zone: string; refused: () => void } = $props();

  const signedIn = $derived(Boolean(page.data.user));

  let note = $state('');
  let booking = $state(false);
  let bookingError = $state('');

  // Not promptSignIn(): that reads `page.url.search`, which lags our shallow writes, so a
  // visitor would return from signing in with the slot no longer chosen — the one thing
  // putting it in the URL was for. Read the address bar, which is current.
  function signInAndComeBack() {
    const returnTo = location.pathname + location.search;
    // eslint-disable-next-line svelte/no-navigation-without-resolve -- signinUrl() wraps resolve('/signin'); the rule can't see through the appended query
    void goto(signinUrl({ returnTo, mode: 'login' }));
  }

  async function book() {
    booking = true;
    bookingError = '';
    try {
      const session = await api.bookMentorSession(mentor.slug, {
        // The absolute instant, never the wall clock: this is what the server re-derives
        // the slot from.
        starts_at: slot.starts_at,
        // Recorded on the booking so the confirmation is written in the zone the seeker
        // actually booked in — `zone`, which is what the server used, not what we asked.
        timezone: zone,
        note,
      });
      void goto(resolve('/my/mentorship/sessions/[id]', { id: session.id }));
    } catch (e) {
      // The hour may simply have gone: somebody else took it, or the mentor moved their
      // availability inside the minute the slot cache holds. Re-ask rather than leaving a
      // list that still offers it.
      bookingError = errorMessage(e, 'That hour could not be booked.');
      refused();
    } finally {
      booking = false;
    }
  }
</script>

{#if signedIn}
  <label class="mt-3 block text-sm">
    <span class="text-muted-foreground">What would you like to talk about? (optional)</span>
    <textarea
      bind:value={note}
      rows="3"
      maxlength="1000"
      class="border-input bg-background mt-1 w-full rounded-md border px-3 py-2 text-sm"
      placeholder="A sentence is plenty — it helps them prepare."
    ></textarea>
  </label>

  <Button class="mt-3" disabled={booking} onclick={book}>
    {booking ? 'Booking…' : 'Book this session'}
  </Button>
{:else}
  <p class="text-muted-foreground mt-3 text-sm">
    Sign in to book. Your chosen hour is in the address, so it will still be here.
  </p>
  <Button class="mt-3" onclick={signInAndComeBack}>Sign in to book</Button>
{/if}

{#if bookingError}
  <p class="text-destructive mt-2 text-sm">{bookingError}</p>
{/if}
```

- [ ] 11.6 Make `web/src/lib/components/MentorBookingView.svelte` take its slots and its booking form as props.
  - Replace the imports

```svelte
  import { onMount } from 'svelte';
  import { goto, replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { signinUrl } from '#lib/signin.js';
  import { Badge, Button, Card, Skeleton } from '#lib/ui/index.js';
```
with
```svelte
  import { onMount, type Snippet } from 'svelte';
  import { replaceState } from '$app/navigation';
  import { page } from '$app/state';
  import { Badge, Card, Skeleton } from '#lib/ui/index.js';
```
  - Replace

```svelte
  import type { Mentor, MentorSlot } from '#lib/types.js';

  let { mentor }: { mentor: Mentor } = $props();
```
with
```svelte
  import type { MentorSlot, MentorSlots } from '#lib/types.js';

  // A month of open slots and the day's hours, for whoever offers them: a mentor on their
  // public page, or a buddy offering mock interviews. Where the slots come from and what
  // taking one looks like are the host page's.
  let {
    loadSlots,
    sessionMinutes,
    counterpart,
    booker,
  }: {
    /** Reads one window's open slots in the viewer's zone. */
    loadSlots: (from: string, to: string, timezone: string, signal: AbortSignal) => Promise<MentorSlots>;
    sessionMinutes: number;
    /** Who the session is with, as the chosen-slot line names them. */
    counterpart: string;
    /** Taking the chosen slot. `refused` re-reads the slots after a booking that did not land. */
    booker: Snippet<[{ slot: MentorSlot; day: string; zone: string; refused: () => void }]>;
  } = $props();
```
  - Replace

```svelte
      const res = await api.getMentorSlots(
        mentor.slug,
        window.from,
        window.to,
        browserZone,
        controller.signal,
      );
```
with
```svelte
      const res = await loadSlots(window.from, window.to, browserZone, controller.signal);
```
  - Delete everything from the line `  // ---- taking the hour ----------------------------------------------------------` down to, not including, the line `  // From the module that owns the Monday-first order, not a second list beside it.`, and put in its place:

```svelte
  const chosen = $derived(daySlots.find((s) => s.starts_at === selectedSlot));

```
  - In the markup, replace everything from `  {#if chosen}` to the closing `</p>` of the `Times shown in {zone}…` paragraph (the last element before `</Card>`) with:

```svelte
  {#if chosen}
    <div class="mt-4 border-t pt-4">
      <p class="text-sm">
        <span class="font-medium">{selectedDay} at {slotLocalTime(chosen)}</span>
        <span class="text-muted-foreground">
          (UTC{chosen.utc_offset}) · {sessionMinutes} minutes with {counterpart}
        </span>
      </p>
      {@render booker({ slot: chosen, day: selectedDay, zone, refused: reload })}
    </div>
  {/if}

  <p class="text-muted-foreground mt-4 text-xs">
    Times shown in {zone}. Sessions run {sessionMinutes} minutes.
  </p>
```

- [ ] 11.7 Pass the mentor's slots and form from the mentor page. In `web/src/routes/mentors/[slug]/+page.svelte` replace

```svelte
  import MentorBookingView from '#lib/components/MentorBookingView.svelte';
```
with
```svelte
  import { api } from '#lib/api.js';
  import MentorBookingForm from '#lib/components/MentorBookingForm.svelte';
  import MentorBookingView from '#lib/components/MentorBookingView.svelte';
```
and `  <MentorBookingView {mentor} />` with

```svelte
  <MentorBookingView
    loadSlots={(from, to, timezone, signal) => api.getMentorSlots(mentor.slug, from, to, timezone, signal)}
    sessionMinutes={mentor.session_minutes}
    counterpart={mentor.name}
  >
    {#snippet booker({ slot, zone, refused })}
      <MentorBookingForm {mentor} {slot} {zone} {refused} />
    {/snippet}
  </MentorBookingView>
```

- [ ] 11.8 Verify the mentor pages are unchanged in behaviour:

```bash
pnpm --dir web exec vitest run src/lib/components/MentorScheduleEditor.spec.ts src/lib/mentorship.test.ts
pnpm --dir web check
pnpm --dir web lint
```
Expected: tests pass; `svelte-check` reports 0 errors; lint clean. Then, with a local stack (`make up`, `pnpm --dir web dev`), run the mentorship end-to-end story: `PLAYWRIGHT_BASE_URL=http://localhost:5173 pnpm --dir web test:e2e e2e/mentorship.spec.ts`. Expected: passes (it publishes a schedule through the editor and books through the view).

- [ ] 11.9 Commit:

```bash
git add web/src/lib/types.ts web/src/lib/components/MentorScheduleEditor.svelte web/src/lib/components/MentorScheduleEditor.spec.ts web/src/lib/components/MentorBookingView.svelte web/src/lib/components/MentorBookingForm.svelte web/src/routes/my/mentorship/schedule/+page.svelte 'web/src/routes/mentors/[slug]/+page.svelte'
git commit -m "Let the schedule editor and slot picker take their API as props"
```

## 12. Buddy mock pages

**Files:**
- Create: `web/src/lib/buddyMocks.ts`, `web/src/lib/buddyMocks.test.ts`, `web/src/lib/components/BuddyMockSettingsForm.svelte`, `web/src/lib/components/BuddyMockReviewForm.svelte`, `web/src/lib/components/BuddyMockList.svelte`, `web/src/lib/components/BuddyMockPanel.svelte`, `web/src/routes/my/buddies/mocks/+page.svelte`
- Modify: `web/src/lib/types.ts` (after `BuddyActivity`, line 1164), `web/src/lib/api.ts` (type imports `:140-160`, functions after `inviteBuddy` `:2389-2391`, the returned object after `inviteBuddy,` `:3398`), `web/src/lib/api.test.ts` (append), `web/src/routes/my/buddies/[handle]/+page.svelte` (imports, panel), `web/src/lib/components/BuddiesView.svelte` (header link, leave dialog wording)

**Interfaces:**
- Produces (TS): `type BuddyMockTopic = 'coding' | 'system_design' | 'behavioral'`; `interface BuddyMockSettings { timezone: string; meeting_url: string; topics: BuddyMockTopic[]; paused: boolean }`; `interface BuddyMockSlots extends MentorSlots { topics: BuddyMockTopic[] }`; `interface BuddyMock { id: string; role: 'host' | 'guest'; counterpart: string | null; starts_at: string; ends_at: string; topic: BuddyMockTopic; status: 'confirmed' | 'cancelled'; meeting_url: string | null; can_cancel: boolean; can_review: boolean; my_review: BuddyMockReview | null; their_review: BuddyMockReview | null }`; `interface BuddyMocks { upcoming: BuddyMock[]; past: BuddyMock[] }`; `interface BuddyMockSummary { completed: number; rating_count: number; rating_avg: number; recent: { rating: number; comment: string; topic: BuddyMockTopic; day: string }[] }`.
- `api.getBuddyMockSettings(): Promise<BuddyMockSettings | null>`, `saveBuddyMockSettings(s: BuddyMockSettings): Promise<BuddyMockSettings>`, `myBuddyMockAvailability(): Promise<MentorAvailabilityRule[]>`, `replaceBuddyMockWeekly(rows): Promise<void>`, `addBuddyMockOverride(date, start, end): Promise<MentorAvailabilityRule>`, `deleteBuddyMockAvailabilityRule(id: number): Promise<void>`, `getBuddyMockSlots(handle, from, to, timezone, signal?): Promise<BuddyMockSlots>`, `bookBuddyMock(handle, { starts_at, topic, timezone }): Promise<BuddyMock>`, `listBuddyMocks(): Promise<BuddyMocks>`, `cancelBuddyMock(id: string): Promise<void>`, `reviewBuddyMock(id: string, rating: number, comment: string): Promise<void>`, `getBuddyMockSummary(handle: string): Promise<BuddyMockSummary>`.
- `buddyMocks.ts`: `MOCK_DURATION_MINUTES = 60`, `MOCK_TOPICS`, `mockTopicLabel(t)`, `isHttpsLink(raw)`, `mockWhen(mock, zone)`, `reviewSummaryLine(summary)`, `mockCounterpartLabel(mock)`, `mockRoleLabel(mock)`.

- [ ] 12.1 Add the types. In `web/src/lib/types.ts`, after the `export interface BuddyActivity …` line (1164), insert:

```ts
/** The mock-interview topics, the server's vocabulary (internal/engage/buddy). */
export type BuddyMockTopic = 'coding' | 'system_design' | 'behavioral';
/** A member's mock-host settings. The link reaches only the two people in a booked mock. */
export interface BuddyMockSettings { timezone: string; meeting_url: string; topics: BuddyMockTopic[]; paused: boolean }
/** One member's open mock slots in the viewer's zone, and the topics they host. Never the link. */
export interface BuddyMockSlots extends MentorSlots { topics: BuddyMockTopic[] }
interface BuddyMockReview { rating: number; comment: string }
/** One mock as its participant sees it. `counterpart` is null once the other member left or a
 *  block stands between them; `their_review` is null until revealed (both reviewed, or a week
 *  after the end). Instants are absolute (`Z`), so they may be rendered in any zone. */
export interface BuddyMock {
  id: string;
  role: 'host' | 'guest';
  counterpart: string | null;
  starts_at: string;
  ends_at: string;
  topic: BuddyMockTopic;
  status: 'confirmed' | 'cancelled';
  meeting_url: string | null;
  can_cancel: boolean;
  can_review: boolean;
  my_review: BuddyMockReview | null;
  their_review: BuddyMockReview | null;
}
export interface BuddyMocks { upcoming: BuddyMock[]; past: BuddyMock[] }
/** A member page's mock record: revealed reviews only, without their authors. */
export interface BuddyMockSummary {
  completed: number;
  rating_count: number;
  rating_avg: number;
  recent: { rating: number; comment: string; topic: BuddyMockTopic; day: string }[];
}
```

- [ ] 12.2 Write the failing tests `web/src/lib/buddyMocks.test.ts`:

```ts
import { describe, expect, it } from 'vitest';
import {
  MOCK_TOPICS,
  isHttpsLink,
  mockCounterpartLabel,
  mockRoleLabel,
  mockTopicLabel,
  mockWhen,
  reviewSummaryLine,
} from './buddyMocks';

describe('buddy mocks', () => {
  it('labels the three topics in the server order', () => {
    expect(MOCK_TOPICS.map(mockTopicLabel)).toEqual(['Coding', 'System design', 'Behavioral']);
  });

  it('accepts only an https link with a host', () => {
    expect(isHttpsLink(' https://meet.google.com/abc-defg-hij ')).toBe(true);
    expect(isHttpsLink('http://meet.google.com/abc')).toBe(false);
    expect(isHttpsLink('https://')).toBe(false);
    expect(isHttpsLink('meet.google.com/abc')).toBe(false);
  });

  // A mock carries instants, so rendering one in the reader's zone is safe (unlike a slot
  // label, which the server already resolved).
  it('renders a mock in the given zone', () => {
    const mock = { starts_at: '2026-11-10T18:00:00Z', ends_at: '2026-11-10T19:00:00Z' };
    const berlin = mockWhen(mock, 'Europe/Berlin');
    expect(berlin).toContain('10 November');
    expect(berlin).toContain('19:00');
    expect(berlin).toContain('20:00');
    expect(mockWhen(mock, 'Asia/Tokyo')).toContain('11 November');
  });

  it('never states an average of nothing', () => {
    expect(reviewSummaryLine({ completed: 0, rating_count: 0, rating_avg: 0 })).toBe('0 mock interviews · no ratings yet');
    expect(reviewSummaryLine({ completed: 1, rating_count: 1, rating_avg: 4 })).toBe('1 mock interview · 4.0 ★ from 1 rating');
    expect(reviewSummaryLine({ completed: 5, rating_count: 3, rating_avg: 4.333 })).toBe('5 mock interviews · 4.3 ★ from 3 ratings');
  });

  it('names a departed or blocked counterpart without a handle', () => {
    expect(mockCounterpartLabel({ counterpart: 'calm-otter-11' })).toBe('calm-otter-11');
    expect(mockCounterpartLabel({ counterpart: null })).toBe('A former buddy');
    expect(mockRoleLabel({ role: 'host' })).toBe('You host');
    expect(mockRoleLabel({ role: 'guest' })).toBe('You are the guest');
  });
});
```
Run: `pnpm --dir web exec vitest run src/lib/buddyMocks.test.ts`. Expected: FAIL — cannot resolve `./buddyMocks`.

- [ ] 12.3 Create `web/src/lib/buddyMocks.ts`:

```ts
import type { BuddyMock, BuddyMockSummary, BuddyMockTopic } from './types.js';

/** A mock runs one hour: buddy.MockDuration on the server. */
export const MOCK_DURATION_MINUTES = 60;

/** The topics a host may offer, in the server's order. */
export const MOCK_TOPICS: readonly BuddyMockTopic[] = ['coding', 'system_design', 'behavioral'];

const TOPIC_LABELS: Record<BuddyMockTopic, string> = {
  coding: 'Coding',
  system_design: 'System design',
  behavioral: 'Behavioral',
};

export function mockTopicLabel(topic: BuddyMockTopic): string {
  return TOPIC_LABELS[topic];
}

/** The server refuses any link that is not https:// — saying so before the request. */
export function isHttpsLink(raw: string): boolean {
  const value = raw.trim();
  if (!value.startsWith('https://')) return false;
  try {
    return new URL(value).host !== '';
  } catch {
    return false;
  }
}

/** A booked mock's time in `zone`. A mock carries absolute instants, so parsing them is
 *  safe here — unlike a slot label, which the server already resolved (web/AGENTS.md). */
export function mockWhen(mock: Pick<BuddyMock, 'starts_at' | 'ends_at'>, zone: string): string {
  const clock = { hour: '2-digit', minute: '2-digit', hourCycle: 'h23', timeZone: zone } as const;
  const day = new Intl.DateTimeFormat('en-GB', { weekday: 'long', day: 'numeric', month: 'long', timeZone: zone });
  const time = new Intl.DateTimeFormat('en-GB', clock);
  const start = new Date(mock.starts_at);
  return `${day.format(start)}, ${time.format(start)}–${time.format(new Date(mock.ends_at))}`;
}

/** The member page's one-line record. An average of no ratings is not zero stars. */
export function reviewSummaryLine(s: Pick<BuddyMockSummary, 'completed' | 'rating_count' | 'rating_avg'>): string {
  const mocks = s.completed === 1 ? '1 mock interview' : `${s.completed} mock interviews`;
  if (s.rating_count === 0) return `${mocks} · no ratings yet`;
  const ratings = s.rating_count === 1 ? '1 rating' : `${s.rating_count} ratings`;
  return `${mocks} · ${s.rating_avg.toFixed(1)} ★ from ${ratings}`;
}

/** The other participant by handle — or nothing about them once they left or a block
 *  stands between you, the rule the buddy conversation list keeps. */
export function mockCounterpartLabel(mock: Pick<BuddyMock, 'counterpart'>): string {
  return mock.counterpart ?? 'A former buddy';
}

export function mockRoleLabel(mock: Pick<BuddyMock, 'role'>): string {
  return mock.role === 'host' ? 'You host' : 'You are the guest';
}
```
Run: `pnpm --dir web exec vitest run src/lib/buddyMocks.test.ts`. Expected: PASS.

- [ ] 12.4 Add the API calls. In `web/src/lib/api.ts` add `BuddyMock,`, `BuddyMocks,`, `BuddyMockSettings,`, `BuddyMockSlots,`, `BuddyMockSummary,` and `BuddyMockTopic,` to the `import type { … } from './types'` list after `BuddyActivity,` (line 153). After `inviteBuddy` (lines 2389-2391) insert:

```ts

  // ── Buddy mock interviews ────────────────────────────────────────────────
  async function getBuddyMockSettings(): Promise<BuddyMockSettings | null> {
    return requestData('/api/v1/me/buddies/mocks/settings');
  }
  async function saveBuddyMockSettings(settings: BuddyMockSettings): Promise<BuddyMockSettings> {
    return requestData('/api/v1/me/buddies/mocks/settings', jsonBody('PUT', settings));
  }
  async function myBuddyMockAvailability(): Promise<MentorAvailabilityRule[]> {
    return requestData('/api/v1/me/buddies/mocks/availability');
  }
  /** The whole recurring week, as the mentor schedule's PUT. 204, so callers re-read. */
  async function replaceBuddyMockWeekly(rules: { weekday: number; start: string; end: string }[]): Promise<void> {
    await call('/api/v1/me/buddies/mocks/availability/weekly', jsonBody('PUT', { rules }));
  }
  async function addBuddyMockOverride(date: string, start: string, end: string): Promise<MentorAvailabilityRule> {
    return requestData('/api/v1/me/buddies/mocks/availability/overrides', jsonBody('POST', { date, start, end }));
  }
  async function deleteBuddyMockAvailabilityRule(id: number): Promise<void> {
    await call(`/api/v1/me/buddies/mocks/availability/${id}`, { method: 'DELETE' });
  }
  /** A member's open mock slots in the viewer's zone and the topics they host. The answer's
   *  `timezone` is the zone actually used, as with mentor slots. Never carries the link. */
  async function getBuddyMockSlots(
    handle: string,
    from: string,
    to: string,
    timezone: string,
    signal?: AbortSignal,
  ): Promise<BuddyMockSlots> {
    const params = new URLSearchParams({ from, to });
    if (timezone) params.set('timezone', timezone);
    const res = await request<{ data: MentorSlot[]; meta: { timezone: string; topics: BuddyMockTopic[] } }>(
      `/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/mock-slots?${params}`,
      { signal },
    );
    return { slots: res.data, timezone: res.meta.timezone, topics: res.meta.topics };
  }
  async function bookBuddyMock(
    handle: string,
    body: { starts_at: string; topic: BuddyMockTopic; timezone: string },
  ): Promise<BuddyMock> {
    return requestData(`/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/mocks`, jsonBody('POST', body));
  }
  async function listBuddyMocks(): Promise<BuddyMocks> {
    return requestData('/api/v1/me/buddies/mocks');
  }
  async function cancelBuddyMock(id: string): Promise<void> {
    await call(`/api/v1/me/buddies/mocks/${encodeURIComponent(id)}/cancel`, { method: 'POST' });
  }
  async function reviewBuddyMock(id: string, rating: number, comment: string): Promise<void> {
    await call(`/api/v1/me/buddies/mocks/${encodeURIComponent(id)}/review`, jsonBody('POST', { rating, comment }));
  }
  async function getBuddyMockSummary(handle: string): Promise<BuddyMockSummary> {
    return requestData(`/api/v1/me/buddies/profiles/${encodeURIComponent(handle)}/mock-reviews`);
  }
```
and in the returned object, after `    inviteBuddy,` (line 3398) insert:

```ts
    getBuddyMockSettings,
    saveBuddyMockSettings,
    myBuddyMockAvailability,
    replaceBuddyMockWeekly,
    addBuddyMockOverride,
    deleteBuddyMockAvailabilityRule,
    getBuddyMockSlots,
    bookBuddyMock,
    listBuddyMocks,
    cancelBuddyMock,
    reviewBuddyMock,
    getBuddyMockSummary,
```
Append to `web/src/lib/api.test.ts`:

```ts
describe('buddy mock paths', () => {
  it('encodes the member handle and the mock id', async () => {
    const urls: string[] = [];
    const fetcher = ((url: string) => {
      urls.push(url);
      return Promise.resolve(new Response('{"data":null}', { status: 200 }));
    }) as unknown as typeof fetch;
    const client = createApi(fetcher, '', {});
    await client.getBuddyMockSummary('a/b');
    await client.cancelBuddyMock('x/y');
    expect(urls).toEqual([
      '/api/v1/me/buddies/profiles/a%2Fb/mock-reviews',
      '/api/v1/me/buddies/mocks/x%2Fy/cancel',
    ]);
  });
});
```
Run: `pnpm --dir web exec vitest run src/lib/api.test.ts`. Expected: PASS (if the base-URL join differs, assert with `toContain` on each path instead — the paths are what the test is about).

- [ ] 12.5 Create `web/src/lib/components/BuddyMockSettingsForm.svelte`:

```svelte
<script lang="ts">
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button, Card } from '#lib/ui/index.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { MOCK_TOPICS, isHttpsLink, mockTopicLabel } from '#lib/buddyMocks.js';
  import type { BuddyMockSettings, BuddyMockTopic } from '#lib/types.js';

  let { settings = $bindable() }: { settings: BuddyMockSettings | null } = $props();

  // The form edits a copy and writes back only what the server stored.
  let timezone = $state(settings?.timezone ?? browserTimezone());
  let meetingUrl = $state(settings?.meeting_url ?? '');
  let topics = $state<BuddyMockTopic[]>(settings ? [...settings.topics] : [...MOCK_TOPICS]);
  let paused = $state(settings?.paused ?? false);
  let saving = $state(false);
  let saved = $state(false);
  let error = $state('');

  const linkOk = $derived(isHttpsLink(meetingUrl));

  function toggle(topic: BuddyMockTopic, on: boolean) {
    topics = MOCK_TOPICS.filter((t) => (t === topic ? on : topics.includes(t)));
  }

  async function save() {
    saving = true;
    saved = false;
    error = '';
    try {
      settings = await api.saveBuddyMockSettings({ timezone, meeting_url: meetingUrl.trim(), topics, paused });
      saved = true;
    } catch (e) {
      error = errorMessage(e, 'Your mock interview settings could not be saved.');
    } finally {
      saving = false;
    }
  }
</script>

<Card class="flex flex-col gap-4 p-4">
  <h2 class="text-sm font-medium">Hosting mock interviews</h2>

  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">Your time zone</span>
    <input
      bind:value={timezone}
      class="border-input bg-background h-9 rounded-md border px-2 text-sm"
      placeholder="Europe/Berlin"
    />
  </label>

  <label class="flex flex-col gap-1 text-sm">
    <span class="text-muted-foreground">Your meeting link</span>
    <input
      type="url"
      bind:value={meetingUrl}
      class="border-input bg-background h-9 rounded-md border px-2 text-sm"
      placeholder="https://meet.google.com/…"
    />
    <span class="text-muted-foreground text-xs">
      Your guest may see your name through this link — for example the name on your Zoom or
      Google account. Only the member who books you sees the link, and only after booking.
    </span>
    {#if meetingUrl && !linkOk}
      <span class="text-destructive text-xs">The link must start with https://</span>
    {/if}
  </label>

  <fieldset class="flex flex-col gap-1 text-sm">
    <legend class="text-muted-foreground mb-1">Topics you host</legend>
    {#each MOCK_TOPICS as topic (topic)}
      <label class="flex items-center gap-2">
        <input type="checkbox" checked={topics.includes(topic)} onchange={(e) => toggle(topic, e.currentTarget.checked)} />
        {mockTopicLabel(topic)}
      </label>
    {/each}
  </fieldset>

  <label class="flex items-center gap-2 text-sm">
    <input type="checkbox" bind:checked={paused} />
    Pause hosting — members cannot book you; booked mock interviews stay
  </label>

  <div class="flex items-center gap-3">
    <Button size="sm" disabled={saving || !linkOk || topics.length === 0} onclick={save}>
      {saving ? 'Saving…' : 'Save'}
    </Button>
    {#if saved}<span class="text-muted-foreground text-sm">Saved.</span>{/if}
  </div>
  {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
</Card>
```

- [ ] 12.6 Create `web/src/lib/components/BuddyMockReviewForm.svelte`:

```svelte
<script lang="ts">
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button } from '#lib/ui/index.js';

  let { id, onreviewed }: { id: string; onreviewed: () => Promise<void> } = $props();

  let rating = $state(0);
  let comment = $state('');
  let sending = $state(false);
  let error = $state('');

  async function send() {
    sending = true;
    error = '';
    try {
      await api.reviewBuddyMock(id, rating, comment.trim());
      await onreviewed();
    } catch (e) {
      error = errorMessage(e, 'Your review could not be saved.');
    } finally {
      sending = false;
    }
  }
</script>

<div class="flex flex-col gap-2 border-t pt-3">
  <p class="text-sm font-medium">How did it go?</p>
  <p class="text-muted-foreground text-xs">
    They see your rating only once they have rated you too, or a week after the mock — so
    neither of you can answer the other's.
  </p>
  <div class="flex gap-1" role="radiogroup" aria-label="Rating">
    {#each [1, 2, 3, 4, 5] as value (value)}
      <button
        type="button"
        role="radio"
        aria-checked={rating === value}
        class="rounded-md border px-2 py-1 text-sm {rating === value ? 'border-primary bg-primary/5' : 'border-input'}"
        onclick={() => (rating = value)}>{value} ★</button
      >
    {/each}
  </div>
  <textarea
    bind:value={comment}
    rows="3"
    maxlength="1000"
    aria-label="Comment"
    class="border-input bg-background w-full rounded-md border px-3 py-2 text-sm"
    placeholder="What went well, and what to practise next (optional)"
  ></textarea>
  <div>
    <Button size="sm" disabled={sending || rating === 0} onclick={send}>
      {sending ? 'Sending…' : 'Send review'}
    </Button>
  </div>
  {#if error}<p class="text-destructive text-sm">{error}</p>{/if}
</div>
```

- [ ] 12.7 Create `web/src/lib/components/BuddyMockList.svelte`:

```svelte
<script lang="ts">
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Badge, Button, Card, ConfirmDialog } from '#lib/ui/index.js';
  import { browserTimezone } from '#lib/mentorship.js';
  import { mockCounterpartLabel, mockRoleLabel, mockTopicLabel, mockWhen } from '#lib/buddyMocks.js';
  import BuddyMockReviewForm from './BuddyMockReviewForm.svelte';
  import type { BuddyMock } from '#lib/types.js';

  let {
    mocks,
    upcoming,
    onchange,
  }: { mocks: BuddyMock[]; upcoming: boolean; onchange: () => Promise<void> } = $props();

  const zone = browserTimezone();
  let cancelling = $state<BuddyMock | null>(null);
  let confirmOpen = $state(false);
  let error = $state('');

  function askCancel(mock: BuddyMock) {
    cancelling = mock;
    confirmOpen = true;
  }

  async function cancel() {
    if (!cancelling) return;
    error = '';
    try {
      await api.cancelBuddyMock(cancelling.id);
      await onchange();
    } catch (e) {
      error = errorMessage(e, 'The mock interview could not be cancelled.');
    } finally {
      cancelling = null;
    }
  }
</script>

{#if error}<p role="alert" class="text-destructive text-sm">{error}</p>{/if}

<ul class="flex flex-col gap-3">
  {#each mocks as mock (mock.id)}
    <li>
      <Card class="flex flex-col gap-2 p-4">
        <div class="flex flex-wrap items-center gap-2 text-sm">
          <span class="font-medium">{mockCounterpartLabel(mock)}</span>
          <Badge variant="secondary">{mockTopicLabel(mock.topic)}</Badge>
          <span class="text-muted-foreground">{mockRoleLabel(mock)}</span>
          {#if mock.status === 'cancelled'}<Badge variant="outline">Cancelled</Badge>{/if}
        </div>
        <p class="text-sm">{mockWhen(mock, zone)} <span class="text-muted-foreground">({zone})</span></p>

        {#if upcoming && mock.meeting_url}
          <!-- eslint-disable-next-line svelte/no-navigation-without-resolve -- the host's own meeting room, an external URL, not an internal route -->
          <a class="w-fit text-sm font-medium underline" href={mock.meeting_url} rel="noreferrer" target="_blank">Join the call</a>
        {/if}
        {#if mock.can_cancel}
          <div><Button variant="ghost" size="sm" onclick={() => askCancel(mock)}>Cancel</Button></div>
        {/if}

        {#if mock.their_review}
          <p class="text-sm">
            Their rating: {mock.their_review.rating} ★{#if mock.their_review.comment} — {mock.their_review.comment}{/if}
          </p>
        {/if}
        {#if mock.my_review}
          <p class="text-muted-foreground text-sm">You rated them {mock.my_review.rating} ★</p>
        {/if}
        {#if mock.can_review}
          <BuddyMockReviewForm id={mock.id} onreviewed={onchange} />
        {/if}
      </Card>
    </li>
  {/each}
</ul>

<ConfirmDialog
  bind:open={confirmOpen}
  title="Cancel this mock interview?"
  description="The other member is told it was cancelled, and the time is offered again."
  confirmLabel="Cancel it"
  onConfirm={cancel}
/>
```

- [ ] 12.8 Create `web/src/lib/components/BuddyMockPanel.svelte` (the booking panel on a member page):

```svelte
<script lang="ts">
  import { goto } from '$app/navigation';
  import { resolve } from '$app/paths';
  import { api } from '#lib/api.js';
  import { errorMessage } from '#lib/utils.js';
  import { Button } from '#lib/ui/index.js';
  import MentorBookingView from './MentorBookingView.svelte';
  import { MOCK_DURATION_MINUTES, mockTopicLabel, reviewSummaryLine } from '#lib/buddyMocks.js';
  import type { BuddyMockSummary, BuddyMockTopic, MentorSlot, MentorSlots } from '#lib/types.js';

  let { handle }: { handle: string } = $props();

  let summary = $state<BuddyMockSummary | null>(null);
  let topics = $state<BuddyMockTopic[] | null>(null);
  let topic = $state<BuddyMockTopic | ''>('');
  let booking = $state(false);
  let bookingError = $state('');

  $effect(() => {
    const current = handle;
    let cancelled = false;
    summary = null;
    api.getBuddyMockSummary(current).then(
      (s) => {
        if (!cancelled) summary = s;
      },
      () => {},
    );
    return () => {
      cancelled = true;
    };
  });

  // The slot read also says which topics this member hosts; an empty list is a member who
  // is not hosting (no settings, or paused).
  async function loadSlots(from: string, to: string, timezone: string, signal: AbortSignal): Promise<MentorSlots> {
    const res = await api.getBuddyMockSlots(handle, from, to, timezone, signal);
    topics = res.topics;
    if (topic && !res.topics.includes(topic)) topic = '';
    return { slots: res.slots, timezone: res.timezone };
  }

  async function book(slot: MentorSlot, zone: string, refused: () => void) {
    if (!topic) return;
    booking = true;
    bookingError = '';
    try {
      // The absolute instant and the zone the server answered in, as a mentor booking sends.
      await api.bookBuddyMock(handle, { starts_at: slot.starts_at, topic, timezone: zone });
      void goto(resolve('/my/buddies/mocks'));
    } catch (e) {
      bookingError = errorMessage(e, 'That time could not be booked.');
      refused();
    } finally {
      booking = false;
    }
  }
</script>

<section class="flex flex-col gap-3">
  <h2 class="text-sm font-medium">Mock interview</h2>
  {#if summary}
    <p class="text-muted-foreground text-sm">{reviewSummaryLine(summary)}</p>
    {#if summary.recent.length > 0}
      <ul class="flex flex-col gap-1 text-sm">
        {#each summary.recent as review, i (i)}
          <li>
            <span class="font-medium">{review.rating} ★</span>
            <span class="text-muted-foreground">· {mockTopicLabel(review.topic)} · {review.day}</span>
            {#if review.comment}<span> — {review.comment}</span>{/if}
          </li>
        {/each}
      </ul>
    {/if}
  {/if}

  {#if topics !== null && topics.length === 0}
    <p class="text-muted-foreground text-sm">This member is not hosting mock interviews right now.</p>
  {/if}

  <MentorBookingView {loadSlots} sessionMinutes={MOCK_DURATION_MINUTES} counterpart={handle}>
    {#snippet booker({ slot, zone, refused })}
      <label class="mt-3 block text-sm">
        <span class="text-muted-foreground">Topic</span>
        <select bind:value={topic} class="border-input bg-background mt-1 h-9 w-full rounded-md border px-2 text-sm">
          <option value="" disabled>Choose a topic</option>
          {#each topics ?? [] as t (t)}
            <option value={t}>{mockTopicLabel(t)}</option>
          {/each}
        </select>
      </label>
      <p class="text-muted-foreground mt-2 text-xs">
        You get the meeting link once booked. Sixty minutes; swap roles halfway.
      </p>
      <Button class="mt-3" disabled={booking || !topic} onclick={() => book(slot, zone, refused)}>
        {booking ? 'Booking…' : 'Book this mock interview'}
      </Button>
      {#if bookingError}<p class="text-destructive mt-2 text-sm">{bookingError}</p>{/if}
    {/snippet}
  </MentorBookingView>
</section>
```

- [ ] 12.9 Create the page `web/src/routes/my/buddies/mocks/+page.svelte`:

```svelte
<script lang="ts">
  import { onMount } from 'svelte';
  import { ArrowLeft } from '@lucide/svelte';
  import { resolve } from '$app/paths';
  import { ApiError, api } from '#lib/api.js';
  import BuddyMockList from '#lib/components/BuddyMockList.svelte';
  import BuddyMockSettingsForm from '#lib/components/BuddyMockSettingsForm.svelte';
  import MentorScheduleEditor from '#lib/components/MentorScheduleEditor.svelte';
  import States from '#lib/components/States.svelte';
  import type { AvailabilityApi, BuddyMockSettings, BuddyMocks, MentorAvailabilityRule } from '#lib/types.js';

  // The caller's own mock interviews: what is coming, when they host, and what is past.
  // Booking happens on another member's page (/my/buddies/[handle]).

  const mockSchedule: AvailabilityApi = {
    replaceWeek: api.replaceBuddyMockWeekly,
    reload: api.myBuddyMockAvailability,
    addOverride: api.addBuddyMockOverride,
    remove: api.deleteBuddyMockAvailabilityRule,
  };

  let view = $state<'loading' | 'ready' | 'not-member' | 'error'>('loading');
  let settings = $state<BuddyMockSettings | null>(null);
  let rules = $state<MentorAvailabilityRule[]>([]);
  let mocks = $state<BuddyMocks>({ upcoming: [], past: [] });

  async function refreshMocks() {
    mocks = await api.listBuddyMocks();
  }

  onMount(async () => {
    try {
      [settings, rules, mocks] = await Promise.all([
        api.getBuddyMockSettings(),
        api.myBuddyMockAvailability(),
        api.listBuddyMocks(),
      ]);
      view = 'ready';
    } catch (err) {
      view = err instanceof ApiError && err.status === 403 ? 'not-member' : 'error';
    }
  });
</script>

<svelte:head><title>Mock interviews · Interview buddies</title></svelte:head>

<div class="flex max-w-4xl flex-col gap-5">
  <a href={resolve('/my/buddies')} class="text-muted-foreground hover:text-foreground flex w-fit items-center gap-1.5 text-sm">
    <ArrowLeft class="size-4" aria-hidden="true" />
    Interview buddies
  </a>
  <header class="flex flex-col gap-1">
    <h1 class="text-2xl font-semibold">Mock interviews</h1>
    <p class="text-muted-foreground text-sm">
      Practise with another member: sixty minutes on the host's own call link, booked at least
      twelve hours ahead. Members book you from your buddy page; you book them from theirs.
    </p>
  </header>

  {#if view === 'loading'}<States state="loading" />
  {:else if view === 'not-member'}
    <section class="border-border rounded-lg border p-6 text-sm">
      <p>Join interview buddies to practise mock interviews with other members.</p>
      <a href={resolve('/my/buddies')} class="mt-3 inline-block font-medium underline hover:no-underline">Go to interview buddies</a>
    </section>
  {:else if view === 'error'}<States state="error" />
  {:else}
    <section class="flex flex-col gap-3">
      <h2 class="text-lg font-medium">Upcoming</h2>
      {#if mocks.upcoming.length === 0}
        <p class="text-muted-foreground text-sm">No mock interviews booked.</p>
      {:else}
        <BuddyMockList mocks={mocks.upcoming} upcoming onchange={refreshMocks} />
      {/if}
    </section>

    <BuddyMockSettingsForm bind:settings />

    <section class="flex flex-col gap-3">
      <h2 class="text-lg font-medium">When you can host</h2>
      <p class="text-muted-foreground text-sm">Hours are in the time zone saved above.</p>
      <MentorScheduleEditor
        bind:rules
        schedule={mockSchedule}
        emptyMessage="No hours yet — until you add some, nobody can book a mock interview with you."
      />
    </section>

    {#if mocks.past.length > 0}
      <section class="flex flex-col gap-3">
        <h2 class="text-lg font-medium">Past</h2>
        <BuddyMockList mocks={mocks.past} upcoming={false} onchange={refreshMocks} />
      </section>
    {/if}
  {/if}
</div>
```

- [ ] 12.10 Put the panel on the member page and the way in on the buddies page.
  - In `web/src/routes/my/buddies/[handle]/+page.svelte` add `  import BuddyMockPanel from '#lib/components/BuddyMockPanel.svelte';` after the `BuddyMessageButton` import, and insert, directly before `      <section class="flex flex-col gap-2">` (the "Activity" section):

```svelte
      <BuddyMockPanel handle={profile.handle} />
```
  - In `web/src/lib/components/BuddiesView.svelte` add `  import { resolve } from '$app/paths';` after `  import { page } from '$app/state';`, replace

```svelte
      <Button variant="outline" size="sm" onclick={() => (confirmLeaveOpen = true)}>Leave</Button>
```
with
```svelte
      <div class="flex items-center gap-3">
        <a href={resolve('/my/buddies/mocks')} class="text-sm font-medium underline hover:no-underline">Mock interviews</a>
        <Button variant="outline" size="sm" onclick={() => (confirmLeaveOpen = true)}>Leave</Button>
      </div>
```
and in the leave dialog replace `description="You disappear from other members' lists and feed, and every conversation closes. You can join again later with the same handle."` with `description="You disappear from other members' lists and feed, every conversation closes and your upcoming mock interviews are cancelled. You can join again later with the same handle."`.

- [ ] 12.11 Verify:

```bash
pnpm --dir web exec vitest run src/lib/buddyMocks.test.ts src/lib/api.test.ts src/lib/components/MentorScheduleEditor.spec.ts src/lib/buddies.test.ts
pnpm --dir web check
pnpm --dir web lint
```
Expected: all pass, 0 svelte-check errors, lint clean. Then against a local stack (`make up`, `make run`, `pnpm --dir web dev`): sign in as two accounts in two browsers, join buddies with both, save settings and a week on `/my/buddies/mocks` as the first, open the first's page as the second, pick a day, slot and topic, book; confirm the second lands on `/my/buddies/mocks` with "Join the call", the first sees the mock with the second's handle, cancelling as either re-offers the slot, and the settings form shows the name-through-link warning.

- [ ] 12.12 Commit:

```bash
git add web/src/lib/buddyMocks.ts web/src/lib/buddyMocks.test.ts web/src/lib/types.ts web/src/lib/api.ts web/src/lib/api.test.ts web/src/lib/components/BuddyMockSettingsForm.svelte web/src/lib/components/BuddyMockReviewForm.svelte web/src/lib/components/BuddyMockList.svelte web/src/lib/components/BuddyMockPanel.svelte web/src/routes/my/buddies/mocks/+page.svelte 'web/src/routes/my/buddies/[handle]/+page.svelte' web/src/lib/components/BuddiesView.svelte
git commit -m "Add the buddy mock interview pages"
```

## 13. Documentation

**Files:**
- Modify: `internal/engage/buddy/AGENTS.md`, `internal/engage/mentorship/AGENTS.md` (lines 5-6, 23, 29-30, 37, 146), `internal/engage/AGENTS.md` (Packages), `AGENTS.md` (the `mentorship-remind` bullet; the Interview buddies row of the module table)

**Interfaces:** none (docs).

- [ ] 13.1 In `internal/engage/buddy/AGENTS.md`, append to the Scope paragraph (after ``…`internal/platform/db/queries/buddies.sql`. HTTP lives in `internal/api/handler`, not here.``):

```markdown

Mock interviews (openspec/changes/buddy-mock-interviews) live in `mock*.go`: a member
publishes when they can host and another books 60 minutes of it. Tables `buddy_mock_*`
(migration 0214), queries in `internal/platform/db/queries/buddy_mocks.sql`; the slot
arithmetic is the shared engine in [internal/engage/slots](../slots/doc.go), the reminders a
second pass of `cmd/mentorship-remind`, HTTP `internal/api/handler/buddy_mocks.go`.
```
and append to "Always true":

```markdown
- **One person, one mock at a time, whatever the role.** `buddy_mock_participants` holds a
  row per person per mock under one `EXCLUDE` (`user_id =`, `[)` range `&&`, `WHERE active`);
  a mentor-style constraint on the host alone could not refuse "host at 19:00 and guest at
  19:00". A lost race and a stale page are the same `ErrSlotUnavailable`. Cancelling sets
  `active = false` in the statement that cancels.
- **The meeting link reaches the two participants of a confirmed mock and nobody else** —
  never a slot listing, a member page or a notice to anyone else. It is read live from the
  host's settings, so a corrected link reaches guests who already booked.
- **A leave or a block cancels future mocks in its own statement.** `LeaveBuddyMember`
  cancels in a CTE; a block cancels through a trigger this feature puts on `user_blocks`,
  because the block is the messaging core's statement and the core never imports a feature.
  Both record `cancelled_by`; the notice is sent afterwards by
  `MockService.AnnounceCancellations(actor)`, which the api layer calls after the leave and
  after `POST /me/conversations/:id/block`. A new path that writes a block or a leave must
  call it too.
- **Reviews are blind.** A review is shown to its subject and on their page only once the
  subject has reviewed back or seven days have passed since the end. The rule is computed in
  `ListBuddyMocksForUser`, `GetBuddyMemberMockSummary` and `ListBuddyMemberRevealedReviews`;
  change all three together. A member page never names who wrote a review.
- **Mock notices name the other member by handle only**, render the time in the
  recipient's zone (UTC, stated, when it does not resolve), and say only "cancelled" for a
  leave or a block.
```

- [ ] 13.2 In `internal/engage/mentorship/AGENTS.md` replace

```markdown
publishes when they are free, and a seeker books half an hour of it. The slot engine is
in slots.go/expand.go/override.go/subtract.go/slice.go; the domain services in
```
with
```markdown
publishes when they are free, and a seeker books half an hour of it. The slot engine lives
in internal/engage/slots (shared with buddy mock interviews; the rules below still bind
it); the domain services in
```
replace `  could diagnose it (expand.go's `resolve`).` with `  could diagnose it (internal/engage/slots/expand.go's `resolve`).`; replace

```markdown
  loop on an unauthenticated endpoint (schedule.go:`Date.next`, and the regression tests
  in midnight_test.go).
```
with
```markdown
  loop on an unauthenticated endpoint (internal/engage/slots/schedule.go:`Date.next`, and
  the regression tests in internal/engage/slots/midnight_test.go).
```
replace `  midnight cannot see it; anchor_test.go now calls the engine the way the handler does.` with `  midnight cannot see it; internal/engage/slots/anchor_test.go calls the engine the way the handler does.`; and replace `Run the slot-engine tests with `-count=1`: the layering guard next door reports `ok` with `Run the slot-engine tests (`go test ./internal/engage/slots/`) with `-count=1`: the layering guard next door reports `ok`.

- [ ] 13.3 In `internal/engage/AGENTS.md` replace `` `referral` `reminder` `report` `socialdigest` `` with `` `referral` `reminder` `report` `slots` `socialdigest` ``.

- [ ] 13.4 In `AGENTS.md`, in the `mentorship-remind` bullet, insert right before its closing "See [internal/engage/mentorship/AGENTS.md]…" sentence:

```markdown
The same run reminds buddy mock interviews (`buddy.SendDueMockReminders`: same offsets, windows and claim order); a failure in one pass does not skip the other, and the run exits non-zero if either could not read its work.
```
and append the buddy AGENTS.md link to that closing sentence, so it reads:

```markdown
See [internal/engage/mentorship/AGENTS.md](internal/engage/mentorship/AGENTS.md) and [internal/engage/buddy/AGENTS.md](internal/engage/buddy/AGENTS.md).
```
In the module table, extend the Interview buddies row's description from "(anonymous peer network: handle, feed vocabulary, one conversation per pair)" to "(anonymous peer network: handle, feed vocabulary, one conversation per pair, mock interviews)".

- [ ] 13.5 Check the links and commit:

```bash
pnpm check:links
git add internal/engage/buddy/AGENTS.md internal/engage/mentorship/AGENTS.md internal/engage/AGENTS.md AGENTS.md
git commit -m "Document buddy mock interviews and the shared slot engine"
```
Expected: `pnpm check:links` reports no broken link.

## 14. Final verification

- [ ] 14.1 Run the whole suite once more from a clean tree:

```bash
gofmt -l . && go build ./... && go vet ./... && go test ./... && go vet -tags=integration ./...
go test -tags=integration ./internal/platform/db/ ./internal/engage/buddy/ ./internal/engage/mentorship/ ./internal/api/handler/ -count=1
pnpm check:sql && pnpm check:links
pnpm --dir web test && pnpm --dir web check && pnpm --dir web lint
openspec validate buddy-mock-interviews --strict
```
Expected: `gofmt -l .` prints nothing and every other command succeeds.

- [ ] 14.2 Walk the spec once more: every scenario in `specs/buddy-mock-interviews/spec.md` maps to a test named in Tasks 2-12 (settings 3.5; availability 3.1; slot listing 4.2; booking 5.1 and 2.1; link visibility 7.2 and 10.1; cancellation 6.1 and 2.1; reminders 9.1; blind review 7.2; anonymous notifications 8.1 and 8.5). Fix any scenario left without one before opening the PR.
