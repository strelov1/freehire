// Package promo decides what discount an account is owed on its next subscription
// invoice: a redeemed promo code, an invitee's first-month discount, and the reward a
// referrer earns once the person they invited pays.
//
// It never talks to the payment provider; internal/identity/billing owns that and neither
// imports the other. The checkout handler and the reward worker join them.
//
// Every value (seat limits, codes) lives in a table, not in source, because the repository
// is public.
package promo

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"regexp"
	"strings"
)

// InvitePercent is the first-month discount an invitee gets and, equally, the credit the
// referrer earns.
const InvitePercent int16 = 50

// Where a discount came from, reported with the percentage so a page can name the offer.
const (
	SourcePromo  = "promo"
	SourceInvite = "invite"
)

// AttributionCookie carries the referrer's code from the invite link to registration.
//
// A cookie, not browser storage: OAuth signup returns on a GET redirect with no body to
// carry a value. Set by the server, not script, because Safari caps script-written cookies
// at seven days and the attribution window is thirty.
//
// The name lives here so the web app cannot drift from it: a renamed cookie silently stops
// attributing.
const AttributionCookie = "fh_ref"

// PromoCookie carries a promo code from a link so the pricing page can prefill the field.
const PromoCookie = "fh_promo"

// inviteCodeBytes is the randomness in an invite code. The code is a public account
// identifier in a pasted URL, so a guessable one would make the link an account enumerator.
const inviteCodeBytes = 12

// mintAttempts bounds retries when a drawn invite code collides; at this width that is
// vanishingly rare.
const mintAttempts = 3

// promoCodeShape mirrors the table's CHECK constraint, so a value that cannot match is
// refused without a read on the rate-limited preview route.
var promoCodeShape = regexp.MustCompile(`^[A-Z0-9]{4,32}$`)

// inviteCodeShape guards the attribution cookie value, which the visitor controls.
var inviteCodeShape = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

var (
	// ErrNotUsable is every refusal about the CODE: unknown, inactive, expired, out of
	// seats. One error on purpose: distinguishing them would be an oracle for guessing codes.
	ErrNotUsable = errors.New("promo: code is not usable")

	// ErrAlreadyRedeemed is about the CALLER, not a code, so it is separate and discloses
	// nothing about any code.
	ErrAlreadyRedeemed = errors.New("promo: this account has already redeemed a code")

	// ErrCodeTaken is a minted invite code that collided; Link retries, so callers never see it.
	ErrCodeTaken = errors.New("promo: invite code already taken")
)

// Stats is what an account's invite page shows: counts and a total only. Naming invitees
// would disclose who signed up, so the type has nowhere to put it.
type Stats struct {
	Invitees    int64 `json:"invitees"`
	Rewarded    int64 `json:"rewarded"`
	CreditCents int64 `json:"credit_cents"`
}

// Discount is the one percentage a checkout session may carry, and its source. The zero
// value means no discount.
type Discount struct {
	Percent int16  `json:"percent"`
	Source  string `json:"source"`
}

// Repository is the persistence this package needs; QueriesRepository implements it.
type Repository interface {
	// PreviewCode returns the percentage a usable code carries, or ErrNotUsable.
	PreviewCode(ctx context.Context, code string) (int16, error)
	// Redeem claims a seat and records the redemption in one statement. ErrNotUsable covers
	// both an unusable code and an already-redeemed caller; disambiguate with HasRedeemed.
	Redeem(ctx context.Context, userID int64, code string) (int16, error)
	// HasRedeemed reports whether this account has spent its one redemption.
	HasRedeemed(ctx context.Context, userID int64) (bool, error)
	// RedeemedPercent is what the account's redeemed code is worth, or zero if none.
	RedeemedPercent(ctx context.Context, userID int64) (int16, error)
	// EnsureInviteCode returns the account's invite code, storing the offered one if it
	// has none. ErrCodeTaken means the offered value collided.
	EnsureInviteCode(ctx context.Context, userID int64, code string) (string, error)
	// ReferrerByCode resolves an invite code to the account that owns it, or ErrNotUsable.
	ReferrerByCode(ctx context.Context, code string) (int64, error)
	// Attribute records that referee arrived through referrer's link, reporting whether
	// this call was the one that wrote it.
	Attribute(ctx context.Context, referrerID, refereeID int64) (bool, error)
	// Stats aggregates one account's invitations.
	Stats(ctx context.Context, userID int64) (Stats, error)
	// HasPendingInvite reports whether this account still owes itself the invitee's
	// first-month discount.
	HasPendingInvite(ctx context.Context, userID int64) (bool, error)

	// PendingRewards are attributed signups whose invitee holds a provider customer.
	PendingRewards(ctx context.Context, max int32) ([]PendingReward, error)
	// CountGranted is how many rewards a referrer has already earned, for the ceiling.
	CountGranted(ctx context.Context, referrerID int64) (int64, error)
	// Grant moves one reward to earned, reporting whether this call moved it. The status and
	// the ceiling are both checked inside the one statement; a prior read would let two
	// passes each grant one more than the bound.
	Grant(ctx context.Context, id, amountCents, ceiling int64) (bool, error)
	// UndeliveredRewards are earned rewards not yet placed on a referrer's balance.
	UndeliveredRewards(ctx context.Context, max int32) ([]EarnedReward, error)
	// MarkDelivered stamps a reward as placed, reporting whether this call stamped it.
	MarkDelivered(ctx context.Context, id int64) (bool, error)
}

// Config is what this package cannot derive.
type Config struct {
	// SiteURL is where an invite link points; Link needs an absolute URL.
	SiteURL string

	// RewardCeiling bounds rewards per referrer. Zero or negative means DefaultRewardCeiling;
	// "unlimited" is deliberately not configurable.
	RewardCeiling int
}

// Service is the use case.
type Service struct {
	repo Repository
	cfg  Config
}

// New constructs a Service.
func New(repo Repository, cfg Config) *Service {
	return &Service{repo: repo, cfg: cfg}
}

// Preview reports what a code is worth to this account without consuming a seat.
func (s *Service) Preview(ctx context.Context, userID int64, code string) (int16, error) {
	normalised, ok := normaliseCode(code)
	if !ok {
		return 0, ErrNotUsable
	}

	// Asked first: a caller who already redeemed cannot use any code, so the code's own
	// status would only confuse.
	redeemed, err := s.repo.HasRedeemed(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("promo: reading redemption history for user %d: %w", userID, err)
	}
	if redeemed {
		return 0, ErrAlreadyRedeemed
	}

	return s.repo.PreviewCode(ctx, normalised)
}

// Redeem spends this account's one redemption on a code and returns the percentage.
//
// The claim is one repository statement, so concurrent redemptions cannot half-succeed;
// this method adds the refusal reason, which the statement does not report.
func (s *Service) Redeem(ctx context.Context, userID int64, code string) (int16, error) {
	normalised, ok := normaliseCode(code)
	if !ok {
		return 0, ErrNotUsable
	}

	pct, err := s.repo.Redeem(ctx, userID, normalised)
	if err == nil {
		return pct, nil
	}
	if !errors.Is(err, ErrNotUsable) {
		return 0, err
	}

	// Only the caller's own reason is worth a second read.
	redeemed, checkErr := s.repo.HasRedeemed(ctx, userID)
	if checkErr == nil && redeemed {
		return 0, ErrAlreadyRedeemed
	}
	return 0, ErrNotUsable
}

// Link is this account's invite URL, minting the code on first ask.
func (s *Service) Link(ctx context.Context, userID int64) (string, error) {
	code, err := s.ensureCode(ctx, userID)
	if err != nil {
		return "", err
	}
	return strings.TrimSuffix(s.cfg.SiteURL, "/") + "/r/" + code, nil
}

// ensureCode returns the account's invite code, minting one if it has none.
//
// The upsert conflicts on the account, so an account that already has a code gets it back
// and the freshly drawn value is discarded.
func (s *Service) ensureCode(ctx context.Context, userID int64) (string, error) {
	var lastErr error
	for range mintAttempts {
		candidate, err := mintCode()
		if err != nil {
			return "", err
		}
		code, err := s.repo.EnsureInviteCode(ctx, userID, candidate)
		if err == nil {
			return code, nil
		}
		if !errors.Is(err, ErrCodeTaken) {
			return "", fmt.Errorf("promo: minting an invite code for user %d: %w", userID, err)
		}
		lastErr = err
	}
	return "", fmt.Errorf("promo: could not mint an invite code for user %d: %w", userID, lastErr)
}

// Attribute records that a newly created account arrived through an invite code.
//
// Every failure to attribute is a no-op, not an error: the code comes from a visitor-controlled
// cookie, and a referral must never cost somebody their account.
func (s *Service) Attribute(ctx context.Context, refereeID int64, code string) error {
	if !inviteCodeShape.MatchString(code) {
		return nil
	}

	referrerID, err := s.repo.ReferrerByCode(ctx, code)
	if errors.Is(err, ErrNotUsable) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("promo: resolving invite code: %w", err)
	}

	// Self-referral is a no-op here rather than a check-constraint error.
	if referrerID == refereeID {
		return nil
	}

	if _, err := s.repo.Attribute(ctx, referrerID, refereeID); err != nil {
		return fmt.Errorf("promo: attributing user %d to %d: %w", refereeID, referrerID, err)
	}
	return nil
}

// Stats is what this account's invite page reports.
func (s *Service) Stats(ctx context.Context, userID int64) (Stats, error) {
	return s.repo.Stats(ctx, userID)
}

// Discount is the one percentage this account's next checkout may carry.
//
// A session admits one coupon, so when both a redeemed code and a pending invite exist the
// larger wins; a tie goes to the typed code.
func (s *Service) Discount(ctx context.Context, userID int64) (Discount, error) {
	best := Discount{}

	invited, err := s.repo.HasPendingInvite(ctx, userID)
	if err != nil {
		return Discount{}, fmt.Errorf("promo: reading the invite state of user %d: %w", userID, err)
	}
	if invited {
		best = Discount{Percent: InvitePercent, Source: SourceInvite}
	}

	redeemed, err := s.repo.RedeemedPercent(ctx, userID)
	if err != nil {
		return Discount{}, fmt.Errorf("promo: reading the redeemed code of user %d: %w", userID, err)
	}
	if redeemed > best.Percent {
		best = Discount{Percent: redeemed, Source: SourcePromo}
	}
	return best, nil
}

// normaliseCode folds a typed code to the table's spelling and rejects what it cannot hold.
func normaliseCode(code string) (string, bool) {
	folded := strings.ToUpper(strings.TrimSpace(code))
	if !promoCodeShape.MatchString(folded) {
		return "", false
	}
	return folded, true
}

// mintCode draws an invite code from crypto/rand. URL-safe base64, so it needs no path
// escaping. Never derived from the account: the value is public once shared.
func mintCode() (string, error) {
	buf := make([]byte, inviteCodeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("promo: drawing an invite code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
