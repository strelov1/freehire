//go:build integration

// Integration tests for the two tailoring bounds against a real Postgres: starting a
// session spends the daily allowance, the turn ceiling is derived from the charges the
// session holds, extending it buys another ceiling's worth, and two simultaneous
// "continue" clicks cost one allowance rather than two. Run with:
// go test -tags=integration ./internal/ai/plan/
package plan

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

func TestStartingSessionsSpendsTheDailyAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-daily@example.test")

	limit := s.cfg.FreeDaily(FeatureTailor)
	for i := range limit {
		if _, err := s.StartSession(ctx, user, sessionID(i)); err != nil {
			t.Fatalf("starting session %d of %d was refused: %v", i+1, limit, err)
		}
	}
	if _, err := s.StartSession(ctx, user, sessionID(limit)); !errors.Is(err, ErrRefused) {
		t.Fatalf("starting a session past the daily allowance returned %v, want ErrRefused", err)
	}

	// Returning to a session that already exists costs nothing — the caller does not call
	// StartSession at all, and if it does (a bootstrap racing itself) the reference is
	// already charged and consumes nothing.
	d, err := s.StartSession(ctx, user, sessionID(0))
	if err != nil {
		t.Fatalf("re-entering an existing session with no allowance left was refused: %v", err)
	}
	if d.Charge != 0 {
		t.Errorf("re-entering charged %d, want 0 — a reload would bill the candidate for pressing refresh", d.Charge)
	}
}

func TestTurnCeilingBoundsOneSession(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-ceiling@example.test")
	const sess = "sess-ceiling"

	if _, err := s.StartSession(ctx, user, sess); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	per := s.cfg.TailorTurnsPerSession

	d, err := s.AllowTurn(ctx, user, sess, per-1)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if !d.Allowed || d.Ceiling != per {
		t.Fatalf("a turn inside the ceiling: allowed=%v ceiling=%d, want true/%d", d.Allowed, d.Ceiling, per)
	}

	if d, err = s.AllowTurn(ctx, user, sess, per); err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if d.Allowed {
		t.Fatal("the turn at the ceiling was allowed; this is the bound that would have stopped the 54-turn session")
	}
}

func TestExtendingASessionBuysAnotherCeiling(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-extend@example.test")
	const sess = "sess-extend"

	if _, err := s.StartSession(ctx, user, sess); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	per := s.cfg.TailorTurnsPerSession

	ext, err := s.ExtendSession(ctx, user, sess)
	if err != nil {
		t.Fatalf("ExtendSession: %v", err)
	}
	if ext.Charge != 1 {
		t.Errorf("extending charged %d, want 1 — it spends another of the day's sessions", ext.Charge)
	}

	d, err := s.AllowTurn(ctx, user, sess, per)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if !d.Allowed {
		t.Fatal("the turn refused a moment ago is still refused after the extension")
	}
	if d.Ceiling != 2*per {
		t.Errorf("Ceiling = %d after one extension, want %d", d.Ceiling, 2*per)
	}

	// The extension came out of the same daily allowance: two of the day's two sessions
	// are now spent on this one vacancy, so a second vacancy is refused.
	if _, err := s.StartSession(ctx, user, "sess-other"); !errors.Is(err, ErrRefused) {
		t.Fatalf("starting another session returned %v, want ErrRefused — the extension did not come out of the daily allowance", err)
	}
}

// TestExtendingAGrandfatheredSessionBuysTurns is the end of the path a live conversation
// takes on deploy day: it holds no ledger row, it is given one ceiling implicitly, and when
// it reaches that ceiling its owner spends another of the day's sessions to continue.
//
// Priced off a row count, that extension would buy slot 1 — the slot the implicit ceiling
// already stands on — leaving the ceiling exactly where it was. The candidate would pay one
// of two daily sessions, be refused on the very next message, and be offered the same button
// again.
func TestExtendingAGrandfatheredSessionBuysTurns(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-grandfathered@example.test")
	const sess = "sess-predates-the-ledger" // never passed through StartSession

	per := s.cfg.TailorTurnsPerSession

	// One ceiling's worth, not zero: a live conversation is bounded, not walled off.
	d, err := s.AllowTurn(ctx, user, sess, per-1)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if !d.Allowed || d.Ceiling != per {
		t.Fatalf("an uncharged session got Allowed=%v Ceiling=%d, want true and %d", d.Allowed, d.Ceiling, per)
	}
	if d, err = s.AllowTurn(ctx, user, sess, per); err != nil || d.Allowed {
		t.Fatalf("the uncharged session ran past its one ceiling (allowed=%v, err %v)", d.Allowed, err)
	}

	ext, err := s.ExtendSession(ctx, user, sess)
	if err != nil {
		t.Fatalf("ExtendSession: %v", err)
	}
	if ext.Charge != 1 {
		t.Fatalf("extending charged %d, want 1", ext.Charge)
	}

	d, err = s.AllowTurn(ctx, user, sess, per)
	if err != nil {
		t.Fatalf("AllowTurn after extending: %v", err)
	}
	if !d.Allowed {
		t.Fatal("the extension bought no turns; one of the day's two sessions was spent for nothing")
	}
	if d.Ceiling != 2*per {
		t.Errorf("Ceiling = %d after extending a grandfathered session, want %d", d.Ceiling, 2*per)
	}
}

func TestExtendingWithNoAllowanceLeftIsRefused(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-extend-broke@example.test")

	// Spend the whole daily allowance on separate sessions.
	for i := range s.cfg.FreeDaily(FeatureTailor) {
		if _, err := s.StartSession(ctx, user, sessionID(i)); err != nil {
			t.Fatalf("StartSession %d: %v", i, err)
		}
	}
	if _, err := s.ExtendSession(ctx, user, sessionID(0)); !errors.Is(err, ErrRefused) {
		t.Fatalf("ExtendSession returned %v, want ErrRefused", err)
	}

	// The session stays exactly as it was: still readable, still bounded by the one
	// ceiling it paid for.
	d, err := s.AllowTurn(ctx, user, sessionID(0), 0)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if !d.Allowed || d.Ceiling != s.cfg.TailorTurnsPerSession {
		t.Errorf("after a refused extension the session reads allowed=%v ceiling=%d", d.Allowed, d.Ceiling)
	}
}

func TestTwoSimultaneousContinuesCostOneAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-double-click@example.test")
	const sess = "sess-double"

	if _, err := s.StartSession(ctx, user, sess); err != nil {
		t.Fatalf("StartSession: %v", err)
	}

	var wg sync.WaitGroup
	for range 2 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _ = s.ExtendSession(ctx, user, sess)
		}()
	}
	wg.Wait()

	if got := usedOn(t, pool, user, FeatureTailor, Day(now)); got != 2 {
		t.Fatalf("a double-clicked continue consumed %d of the day's allowance, want 2 (the start plus one extension)", got)
	}
	d, err := s.AllowTurn(ctx, user, sess, 0)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if d.Ceiling != 2*s.cfg.TailorTurnsPerSession {
		t.Errorf("Ceiling = %d, want %d — the double click bought two ceilings", d.Ceiling, 2*s.cfg.TailorTurnsPerSession)
	}
}

func TestOneSessionCannotBorrowAnothersCeilings(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-prefix@example.test")

	// 'sess-1' is a prefix of 'sess-12'. Without the terminator on the reference, the
	// shorter id would count the longer one's charges as its own.
	if _, err := s.StartSession(ctx, user, "sess-1"); err != nil {
		t.Fatalf("StartSession sess-1: %v", err)
	}
	if _, err := s.StartSession(ctx, user, "sess-12"); err != nil {
		t.Fatalf("StartSession sess-12: %v", err)
	}

	d, err := s.AllowTurn(ctx, user, "sess-1", s.cfg.TailorTurnsPerSession)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if d.Allowed {
		t.Fatal("sess-1 ran past its ceiling on sess-12's charge")
	}
	if d.Ceiling != s.cfg.TailorTurnsPerSession {
		t.Errorf("sess-1 holds a ceiling of %d, want %d", d.Ceiling, s.cfg.TailorTurnsPerSession)
	}
}

func TestReleasingAFailedBootstrapGivesTheSessionBack(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-release@example.test")
	const sess = "sess-failed"

	if _, err := s.StartSession(ctx, user, sess); err != nil {
		t.Fatalf("StartSession: %v", err)
	}
	if err := s.ReleaseSession(ctx, user, sess); err != nil {
		t.Fatalf("ReleaseSession: %v", err)
	}
	if got := usedOn(t, pool, user, FeatureTailor, Day(now)); got != 0 {
		t.Fatalf("the day's counter reads %d after releasing a failed bootstrap, want 0", got)
	}

	// And the whole allowance is available again, so the retry is charged once.
	for i := range s.cfg.FreeDaily(FeatureTailor) {
		if _, err := s.StartSession(ctx, user, sessionID(i)); err != nil {
			t.Fatalf("retry %d after release was refused: %v", i, err)
		}
	}
}

func TestTailorTurnsDoNotTouchTheAssistantAllowance(t *testing.T) {
	now := time.Date(2026, 9, 15, 10, 0, 0, 0, time.UTC)
	s, pool := newStore(t, enforcing(), now)
	ctx := context.Background()
	user := insertUser(t, pool, "session-separate@example.test")

	// Spend the whole assistant allowance in chat.
	for i := range s.cfg.FreeDaily(FeatureAssistant) {
		if _, err := s.Consume(ctx, user, FeatureAssistant, jobRef(i)); err != nil {
			t.Fatalf("assistant turn %d: %v", i, err)
		}
	}
	if _, err := s.Consume(ctx, user, FeatureAssistant, "one-more"); !errors.Is(err, ErrRefused) {
		t.Fatalf("expected the assistant allowance to be spent, got %v", err)
	}

	// Tailoring is metered by its own two bounds, so it is untouched. Otherwise the daily
	// assistant allowance would decide how deep one CV may be edited.
	if _, err := s.StartSession(ctx, user, "sess-after-chat"); err != nil {
		t.Fatalf("starting a tailoring session was refused after the assistant allowance ran out: %v", err)
	}
	d, err := s.AllowTurn(ctx, user, "sess-after-chat", 0)
	if err != nil {
		t.Fatalf("AllowTurn: %v", err)
	}
	if !d.Allowed {
		t.Fatal("a tailoring turn was refused because the assistant allowance was spent")
	}
}

func sessionID(i int) string { return "sess-" + jobRef(i) }
