package plan

import (
	"context"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/platform/db"
)

// A tailoring session is metered by TWO bounds, and they stop different things. The daily
// allowance bounds how many vacancies a candidate works on; the turn ceiling bounds how
// far one of them goes.
//
// Either alone leaves a hole. Measured on production over three August weeks, the median
// session ran 2.7 turns and one ran 54 — and the account that ran it opened two sessions,
// well inside any daily count, while consuming $25 of model calls. A session count would
// not have noticed it. That is the shape of what this replaces: the old ledger charged
// three points when a session was created and nothing for anything said inside it.
//
// The ceiling in force is not stored anywhere of its own. It is derived from the charges
// the session holds: '<session>#1' is the first, '#2' buys another ceiling's worth. This
// falls out of the idempotency key rather than fighting it — a bare session id could only
// ever be charged once, which is right for starting a session and wrong for extending one,
// while '#n' makes an extension a distinct event that is still idempotent. A
// double-clicked "continue" therefore consumes one allowance, not two.

// TurnDecision is the answer to "may this session run one more turn".
type TurnDecision struct {
	Allowed   bool
	Turns     int
	Ceiling   int
	Unlimited bool

	// Shadowed marks a turn that WOULD have been refused with enforcement on.
	Shadowed bool
}

// ceilingsHeld is how many ceilings a session holds, given the highest slot its ledger
// charges reach. It is the ONE place the implicit ceiling is granted, so the turn rule and
// the extension price cannot disagree about how many the session already has.
//
// A session holding no charge predates this metering: every session open on the day it
// deploys was created before anything wrote a ledger row for one. Treating "no charge" as
// "never paid for" would 402 the next turn of every live conversation until its owner
// bought an extension for work they had already started.
//
// So an uncharged session is given slot 1 implicitly. It is still bounded — a caller who
// skips the bootstrap gets one ceiling's worth of turns and no more — and the charge is
// taken where sessions are created, which is the place that can refuse before anything
// exists. Granting it as a SLOT rather than as a bare ceiling is what makes the first
// extension buy slot 2: granted as a ceiling, the extension would buy slot 1, land the
// session back on exactly the ceiling it already had, and cost one of the day's sessions
// for no extra turns at all.
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
	if tier == TierPro {
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

// sessionRef is the ledger reference for the nth ceiling of a session. The terminator
// matters: without it, the scan for session "sess-1" would sweep up the charges of
// "sess-12" as well, because one id is a prefix of the other.
func sessionRef(sessionID string, n int) string {
	return sessionPrefix(sessionID) + strconv.Itoa(n)
}

// sessionPrefix is what the ceiling scan matches on, terminated for the same reason.
func sessionPrefix(sessionID string) string { return sessionID + "#" }

// refSlot reads back the ceiling number sessionRef wrote. A reference this package did not
// write — nothing produces one today, but the ledger is append-only and outlives any one
// format — reports 0 and is simply not counted, rather than failing a turn over a row
// somebody will have to look at anyway.
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

// StartSession consumes one tailoring-session allowance for a session being created.
//
// It is charged when the session is minted, not when the workspace is opened: the
// workspace is addressed by vacancy, so a reload re-runs the bootstrap, and charging that
// would bill a candidate for pressing refresh. Returning to a session that already exists
// consumes nothing — the caller simply does not call this.
func (s *Store) StartSession(ctx context.Context, userID int64, sessionID string) (Decision, error) {
	return s.Consume(ctx, userID, FeatureTailor, sessionRef(sessionID, 1))
}

// ExtendSession buys one further ceiling's worth of turns for a session that reached its
// current one.
//
// The slot it buys is the one AFTER however many ceilings the session already holds, which
// for a session that predates the metering is slot 2 — slot 1 being the one it was granted
// implicitly. Pricing the extension off a raw row count instead would sell that session
// slot 1, leave its ceiling exactly where it was, and take one of the day's tailoring
// sessions for nothing.
//
// Two simultaneous "continue" clicks both read the same ceiling count and both attempt the
// same next reference; they serialise on today's counter row, so the second sees the first's
// charge, is reported as already charged and consumes nothing. The race costs a candidate
// one allowance rather than two, without a lock of its own.
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

// AllowTurn reports whether a tailoring session may run one more turn.
//
// The caller passes how many turns the session has already run, because that is the
// caller's own data — this package would otherwise have to read the assistant's transcript
// tables to find out, and knowing how a conversation is stored is not its business.
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
// holds none. It is the slot rather than the row count because a release voids a row without
// un-selling the slot, and because a session granted its first ceiling implicitly holds no
// row at all — see ceilingsHeld.
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
