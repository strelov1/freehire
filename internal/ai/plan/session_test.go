package plan

import "testing"

func TestTurnDecisionWithinTheCeiling(t *testing.T) {
	cfg := enforcing()
	// One ceiling bought, and the session has run fewer turns than it allows.
	d := cfg.decideTurn(TierFree, 1, cfg.TailorTurnsPerSession-1)

	if !d.Allowed {
		t.Fatal("a turn inside the ceiling was refused")
	}
	if d.Ceiling != cfg.TailorTurnsPerSession {
		t.Errorf("Ceiling = %d, want %d", d.Ceiling, cfg.TailorTurnsPerSession)
	}
}

func TestTurnDecisionAtTheCeiling(t *testing.T) {
	cfg := enforcing()
	d := cfg.decideTurn(TierFree, 1, cfg.TailorTurnsPerSession)

	if d.Allowed {
		t.Fatal("a turn past the ceiling was allowed; this is the bound that stops one session running 54 turns")
	}
	if d.Ceiling != cfg.TailorTurnsPerSession {
		t.Errorf("Ceiling = %d, want %d", d.Ceiling, cfg.TailorTurnsPerSession)
	}
}

func TestTurnCeilingGrowsWithEachExtension(t *testing.T) {
	cfg := enforcing()
	per := cfg.TailorTurnsPerSession

	// Two ceilings bought: the session may run twice as far, and the turn that was
	// refused a moment ago now goes through.
	d := cfg.decideTurn(TierFree, 2, per)
	if !d.Allowed {
		t.Fatal("a turn was still refused after a second ceiling was bought")
	}
	if d.Ceiling != 2*per {
		t.Errorf("Ceiling = %d after two extensions, want %d", d.Ceiling, 2*per)
	}
	if cfg.decideTurn(TierFree, 2, 2*per).Allowed {
		t.Error("the extended ceiling does not bound anything")
	}
}

func TestASessionWithNoChargeRunsNoTurns(t *testing.T) {
	cfg := enforcing()
	// No ceiling bought means the session was never paid for. A turn in it must not run:
	// otherwise a caller that skipped the bootstrap charge gets the workspace for free.
	if cfg.decideTurn(TierFree, 0, 0).Allowed {
		t.Fatal("a session that was never charged ran a turn")
	}
}

func TestProSessionsHaveNoTurnCeiling(t *testing.T) {
	cfg := enforcing()
	d := cfg.decideTurn(TierPro, 1, 500)

	if !d.Allowed {
		t.Fatal("a pro session was stopped by a turn ceiling; pro is the same product without the ceilings")
	}
	if !d.Unlimited {
		t.Error("a pro turn decision reports a countable ceiling")
	}
}

func TestShadowModeDoesNotStopATurn(t *testing.T) {
	cfg := DefaultConfig() // enforcement off, as shipped
	d := cfg.decideTurn(TierFree, 1, cfg.TailorTurnsPerSession+5)

	if !d.Allowed {
		t.Fatal("shadow mode stopped a turn; the shadow run measures what a ceiling would stop without stopping anyone")
	}
	if !d.Shadowed {
		t.Error("the decision does not record that it would have refused")
	}
}

func TestSessionRefIsTerminatedSoOneSessionCannotBorrowAnother(t *testing.T) {
	// 'sess-1' must not collect 'sess-12''s charges. The terminator is what separates
	// them, and it lives in one place so a caller cannot forget it.
	if got, want := sessionRef("sess-1", 1), "sess-1#1"; got != want {
		t.Errorf("sessionRef = %q, want %q", got, want)
	}
	if got, want := sessionPrefix("sess-1"), "sess-1#"; got != want {
		t.Errorf("sessionPrefix = %q, want %q", got, want)
	}
}
