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

// A session holding no charge is one that predates this metering — every conversation
// open on the day it deploys is in that state. It gets the ceiling its first charge would
// have bought, so a live conversation is not stopped mid-way and asked to pay for work its
// owner already started.
func TestASessionWithNoChargeGetsOneCeiling(t *testing.T) {
	cfg := enforcing()
	per := cfg.TailorTurnsPerSession

	d := cfg.decideTurn(TierFree, 0, per-1)
	if !d.Allowed {
		t.Fatal("a session that predates metering was stopped; every live conversation would be")
	}
	if d.Ceiling != per {
		t.Errorf("Ceiling = %d, want %d — one ceiling's worth, not unbounded", d.Ceiling, per)
	}
	// Still bounded, though: a caller who skipped the bootstrap gets one ceiling and no more.
	if cfg.decideTurn(TierFree, 0, per).Allowed {
		t.Error("an uncharged session ran past the one ceiling it was given")
	}
}

// The implicit ceiling is granted as SLOT 1, which is what makes the first extension of a
// grandfathered session worth something. Priced off a row count instead, that extension
// would buy slot 1, land the session on the ceiling it already had, and cost one of the
// day's two tailoring sessions for no extra turns at all.
func TestExtendingAGrandfatheredSessionBuysTheNextSlot(t *testing.T) {
	cfg := enforcing()
	per := cfg.TailorTurnsPerSession

	// No charges: slot 1 is implicit, so the extension to sell is slot 2.
	if got, want := ceilingsHeld(0)+1, 2; got != want {
		t.Fatalf("an uncharged session extends into slot %d, want %d", got, want)
	}
	// And having bought it, the session really can run further than before.
	if !cfg.decideTurn(TierFree, 2, per).Allowed {
		t.Fatal("the extension bought no turns; a session allowance was spent for nothing")
	}
	if got := cfg.decideTurn(TierFree, 2, per).Ceiling; got != 2*per {
		t.Errorf("Ceiling = %d after extending a grandfathered session, want %d", got, 2*per)
	}
}

// A session that started normally extends into slot 2 as well — the implicit rule must not
// hand a charged session a ceiling it did not buy.
func TestExtendingAChargedSessionBuysTheNextSlot(t *testing.T) {
	if got, want := ceilingsHeld(1)+1, 2; got != want {
		t.Errorf("a session holding slot 1 extends into slot %d, want %d", got, want)
	}
}

// The ceiling follows the highest slot sold, not how many rows survive. A bootstrap whose
// charge was released leaves slot 2 standing, and shrinking the ceiling back would refuse
// turns the candidate paid for.
func TestTheCeilingFollowsTheHighestSlotNotTheRowCount(t *testing.T) {
	cfg := enforcing()
	if got, want := cfg.decideTurn(TierFree, 2, 0).Ceiling, 2*cfg.TailorTurnsPerSession; got != want {
		t.Errorf("Ceiling = %d for a session whose highest slot is 2, want %d", got, want)
	}
}

func TestRefSlotReadsBackWhatSessionRefWrote(t *testing.T) {
	if got := refSlot(sessionRef("sess-1", 3)); got != 3 {
		t.Errorf("refSlot = %d, want 3", got)
	}
	// A reference this package did not write is not counted rather than failing a turn.
	if got := refSlot("no-terminator"); got != 0 {
		t.Errorf("refSlot of an unterminated ref = %d, want 0", got)
	}
	if got := refSlot("sess-1#not-a-number"); got != 0 {
		t.Errorf("refSlot of a non-numeric slot = %d, want 0", got)
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
