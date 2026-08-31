package plan

import (
	"context"
	"strconv"

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

// decideTurn is the rule as a pure function of the plan, how many ceilings the session
// has been charged for, and how many turns it has already run.
func (c Config) decideTurn(tier Tier, ceilingsBought, turnsSoFar int) TurnDecision {
	d := TurnDecision{Turns: turnsSoFar}
	if tier == TierPro {
		d.Allowed, d.Unlimited = true, true
		return d
	}
	d.Ceiling = ceilingsBought * c.TailorTurnsPerSession

	// A session holding no charge predates this metering: every session open on the day it
	// deploys was created before anything wrote a ledger row for one. Treating "no charge"
	// as "never paid for" would 402 the next turn of every live conversation until its
	// owner bought an extension for work they had already started.
	//
	// So an uncharged session is given the ceiling its first charge would have bought. It
	// is still bounded — a caller who skips the bootstrap gets one ceiling's worth of turns
	// and no more — and the charge is taken where sessions are created, which is the place
	// that can refuse before anything exists.
	if d.Ceiling == 0 {
		d.Ceiling = c.TailorTurnsPerSession
	}

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
// matters: without it, the count for session "sess-1" would sweep up the charges of
// "sess-12" as well, because one id is a prefix of the other.
func sessionRef(sessionID string, n int) string {
	return sessionPrefix(sessionID) + strconv.Itoa(n)
}

// sessionPrefix is what the ceiling count matches on, terminated for the same reason.
func sessionPrefix(sessionID string) string { return sessionID + "#" }

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
// Two simultaneous "continue" clicks both read the same ceiling count and both attempt the
// same next reference; the idempotency index admits one, and the other is reported as
// already charged and consumes nothing. So the race costs a candidate one allowance rather
// than two, without a lock of its own.
func (s *Store) ExtendSession(ctx context.Context, userID int64, sessionID string) (Decision, error) {
	bought, err := s.ceilingsBought(ctx, userID, sessionID)
	if err != nil {
		return Decision{}, err
	}
	return s.Consume(ctx, userID, FeatureTailor, sessionRef(sessionID, bought+1))
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
	bought, err := s.ceilingsBought(ctx, userID, sessionID)
	if err != nil {
		return TurnDecision{}, err
	}
	return s.cfg.decideTurn(tier, bought, turnsSoFar), nil
}

// ceilingsBought counts the session's charges, which is the number of ceilings it holds.
func (s *Store) ceilingsBought(ctx context.Context, userID int64, sessionID string) (int, error) {
	n, err := s.q.CountConsumptionsByRefPrefix(ctx, db.CountConsumptionsByRefPrefixParams{
		UserID: userID, Feature: string(FeatureTailor), RefPrefix: sessionPrefix(sessionID),
	})
	return int(n), err
}
