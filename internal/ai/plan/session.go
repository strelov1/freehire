package plan

import (
	"context"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A tailoring session is metered by two bounds: the daily allowance caps how many vacancies a
// candidate works on, the turn ceiling caps how far one goes. A session count alone leaves a
// single session unbounded.
//
// The ceiling in force is derived from the session's charges: '<session>#1' is the first, '#2'
// buys another ceiling's worth. A bare session id could only be charged once, while '#n' makes
// an extension a distinct, still idempotent event, so a double-clicked "continue" consumes one
// allowance.

// TurnDecision is the answer to "may this session run one more turn".
type TurnDecision struct {
	Allowed   bool
	Turns     int
	Ceiling   int
	Unlimited bool

	// Shadowed marks a turn that WOULD have been refused with enforcement on.
	Shadowed bool
}

// ceilingsHeld is how many ceilings a session holds, given the highest slot its ledger charges
// reach. It is the one place the implicit ceiling is granted.
//
// A session with no charge predates this metering and gets slot 1 implicitly, so live
// conversations are not refused. It is granted as a slot, not a bare ceiling, so the first
// extension buys slot 2 instead of landing back on the ceiling it already had.
func ceilingsHeld(highestSlot int) int {
	if highestSlot < 1 {
		return 1
	}
	return highestSlot
}

// decideTurn is the rule as a pure function of the plan, the highest ceiling slot the
// session's charges reach, and how many turns it has already run.
func (c Config) decideTurn(tier Tier, highestSlot, turnsSoFar int) TurnDecision {
	d := TurnDecision{Turns: turnsSoFar}
	// Every paid tier, not pro specifically, so a pricier plan never runs out of turns first.
	if tier.Paid() {
		d.Allowed, d.Unlimited = true, true
		return d
	}
	d.Ceiling = ceilingsHeld(highestSlot) * c.TailorTurnsPerSession

	if turnsSoFar < d.Ceiling {
		d.Allowed = true
		return d
	}
	if !c.Enforced(FeatureTailor) {
		d.Allowed, d.Shadowed = true, true
	}
	return d
}

// sessionRef is the ledger reference for the nth ceiling of a session. The '#' terminator keeps
// the scan for "sess-1" from matching "sess-12".
func sessionRef(sessionID string, n int) string {
	return sessionPrefix(sessionID) + strconv.Itoa(n)
}

// sessionPrefix is what the ceiling scan matches on, terminated for the same reason.
func sessionPrefix(sessionID string) string { return sessionID + "#" }

// refSlot reads back the ceiling number sessionRef wrote. A reference in any other format
// reports 0 and is not counted, rather than failing a turn.
func refSlot(ref string) int {
	_, suffix, found := strings.Cut(ref, "#")
	if !found {
		return 0
	}
	n, err := strconv.Atoi(suffix)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

// StartSession consumes one tailoring-session allowance for a session being created. It is
// charged at creation, not when the workspace opens, so a reload is not billed; returning to an
// existing session does not call it.
func (s *Store) StartSession(ctx context.Context, userID int64, sessionID string) (Decision, error) {
	return s.Consume(ctx, userID, FeatureTailor, sessionRef(sessionID, 1))
}

// ExtendSession buys one further ceiling's worth of turns for a session that reached its
// current one.
//
// The slot bought is the one after the ceilings already held (slot 2 for a pre-metering
// session), not a raw row count. Two simultaneous "continue" clicks serialise on today's counter
// row; the second sees the first's charge and consumes nothing.
func (s *Store) ExtendSession(ctx context.Context, userID int64, sessionID string) (Decision, error) {
	held, err := s.ceilingsHeld(ctx, userID, sessionID)
	if err != nil {
		return Decision{}, err
	}
	return s.Consume(ctx, userID, FeatureTailor, sessionRef(sessionID, held+1))
}

// ReleaseSession gives back the allowance a session's first charge took, for a bootstrap
// that failed before a usable session existed. Like every release it is safe to call blind.
func (s *Store) ReleaseSession(ctx context.Context, userID int64, sessionID string) error {
	return s.Release(ctx, userID, FeatureTailor, sessionRef(sessionID, 1))
}

// AllowTurn reports whether a tailoring session may run one more turn. The caller passes
// turnsSoFar because this package should not read the assistant's transcript tables.
func (s *Store) AllowTurn(ctx context.Context, userID int64, sessionID string, turnsSoFar int) (TurnDecision, error) {
	tier, err := s.Tier(ctx, userID)
	if err != nil {
		return TurnDecision{}, err
	}
	slot, err := s.highestSlot(ctx, userID, sessionID)
	if err != nil {
		return TurnDecision{}, err
	}
	return s.cfg.decideTurn(tier, slot, turnsSoFar), nil
}

// highestSlot is the largest ceiling number the session's live charges reach, or 0 when it
// holds none. It is a slot, not a row count: a release voids a row without un-selling the slot,
// and an implicitly granted first ceiling has no row.
func (s *Store) highestSlot(ctx context.Context, userID int64, sessionID string) (int, error) {
	refs, err := s.q.ListConsumptionRefsByPrefix(ctx, db.ListConsumptionRefsByPrefixParams{
		UserID: userID, Feature: string(FeatureTailor), RefPrefix: sessionPrefix(sessionID),
	})
	if err != nil {
		return 0, err
	}
	highest := 0
	for _, ref := range refs {
		if n := refSlot(ref.String); n > highest {
			highest = n
		}
	}
	return highest, nil
}

// ceilingsHeld is highestSlot resolved through the implicit-slot rule, for the callers that
// need to know how many ceilings a session has rather than which slot to sell next.
func (s *Store) ceilingsHeld(ctx context.Context, userID int64, sessionID string) (int, error) {
	slot, err := s.highestSlot(ctx, userID, sessionID)
	if err != nil {
		return 0, err
	}
	return ceilingsHeld(slot), nil
}
