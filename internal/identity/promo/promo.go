// Package promo decides what discount an account is owed on its next subscription
// invoice: a promo code it redeemed, the first-month discount an invited account is
// offered, and the reward a referrer earns once the person they invited actually pays.
//
// It answers that question and does nothing else. It never talks to the payment provider
// and never learns what a subscription is — internal/identity/billing owns both, imports
// nothing from here, and is imported by nothing here. What holds the two together is the
// HTTP handler for a checkout and the worker for a reward, and both may import anything.
//
// The reason every value lives in a table rather than in this file is that the repository
// is public. A seat limit or a launch code readable from the source is drained the day
// somebody greps it; a rule for reading a table is not.
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

// InvitePercent is what somebody who arrives through an invite link gets off their first
// month, and equally what the person who invited them earns as credit. Half each way: the
// figure the offer is described by, and the one place it is written down.
const InvitePercent int16 = 50

// Where a discount came from, reported alongside the percentage so a page can say which
// offer it is showing rather than guessing from the number.
const (
	SourcePromo  = "promo"
	SourceInvite = "invite"
)

// AttributionCookie carries the referrer's code from the invite link to whichever
// registration path the visitor eventually takes.
//
// A cookie and not browser storage, because the majority signup path is OAuth: the visitor
// leaves for the provider and returns on a GET redirect, which has no body a value could
// travel in. It is set by the SERVER (see the web app's request hook) rather than by
// script, because Safari's tracking prevention caps script-written cookies at seven days
// and the attribution window is thirty.
//
// The name lives here rather than in the web app so the two halves of one mechanism cannot
// drift apart silently — a renamed cookie would simply stop attributing, and nothing would
// fail.
const AttributionCookie = "fh_ref"

// PromoCookie carries a promo code arriving in a link, so the pricing page can prefill the
// field rather than asking somebody to retype what they just clicked.
const PromoCookie = "fh_promo"

// inviteCodeBytes is how much randomness an invite code carries. This code appears in a
// URL people paste into chats, so it is a public identifier for an account — short enough
// to guess would turn the invite link into an account enumerator.
const inviteCodeBytes = 12

// mintAttempts bounds the retries when a freshly drawn invite code collides with one
// already held. At this width a collision is a lottery win, so the loop exists to make the
// failure explicit rather than because it is expected to run twice.
const mintAttempts = 3

// promoCodeShape is what the table's own CHECK constraint will accept. Applied before any
// query so a value that could never match a row is refused without spending a read on the
// rate-limited preview route.
var promoCodeShape = regexp.MustCompile(`^[A-Z0-9]{4,32}$`)

// inviteCodeShape is the same guard for the value that arrives in an attribution cookie,
// which is to say a value the visitor composed.
var inviteCodeShape = regexp.MustCompile(`^[A-Za-z0-9_-]{16,64}$`)

var (
	// ErrNotUsable is every refusal that is about the CODE: unknown, inactive, expired,
	// out of seats. They are deliberately one error. The preview route is authenticated
	// and rate limited but still reachable by anyone with an account, and a refusal that
	// distinguished "no such code" from "out of seats" would be an oracle for guessing.
	ErrNotUsable = errors.New("promo: code is not usable")

	// ErrAlreadyRedeemed is the one refusal that is about the CALLER rather than about a
	// code, which is why it is separate: telling somebody they have already used their
	// one redemption is useful and discloses nothing about any code.
	ErrAlreadyRedeemed = errors.New("promo: this account has already redeemed a code")

	// ErrCodeTaken is a minted invite code that collided with one already held. Never
	// reaches a caller — Link retries.
	ErrCodeTaken = errors.New("promo: invite code already taken")
)

// Stats is what an account's own invite page says. Counts and a total, and deliberately
// nothing else: naming who accepted an invite discloses that a particular person signed
// up for a job board, which is not the referrer's to know. The absence is enforced by this
// type having nowhere to put it, rather than by filtering at the edge.
type Stats struct {
	Invitees    int64 `json:"invitees"`
	Rewarded    int64 `json:"rewarded"`
	CreditCents int64 `json:"credit_cents"`
}

// Discount is the one percentage a checkout session may carry, and where it came from.
// The zero value means no discount, which must produce exactly the request checkout makes
// today.
type Discount struct {
	Percent int16  `json:"percent"`
	Source  string `json:"source"`
}

// Repository is the persistence this package needs. Its implementation over sqlc lives in
// repository.go; the shape is here because this package owns the questions.
type Repository interface {
	// PreviewCode returns the percentage a usable code carries, or ErrNotUsable.
	PreviewCode(ctx context.Context, code string) (int16, error)
	// Redeem claims a seat and records the redemption in one statement, returning the
	// percentage. ErrNotUsable covers both an unusable code and a caller who has already
	// redeemed one — the caller disambiguates with HasRedeemed.
	Redeem(ctx context.Context, userID int64, code string) (int16, error)
	// HasRedeemed reports whether this account has spent its one redemption.
	HasRedeemed(ctx context.Context, userID int64) (bool, error)
	// RedeemedPercent is what the code this account redeemed is worth, or zero when it has
	// redeemed none. Separate from HasRedeemed because the checkout path needs the number
	// and the refusal path needs only the fact.
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
	// Grant moves one reward to earned at the given amount, reporting whether this call
	// was the one that moved it. Guarded on the current status AND on the referrer being
	// below ceiling, both inside the one statement — a ceiling read before the write is a
	// suggestion rather than a bound, since two passes can read the same count.
	Grant(ctx context.Context, id, amountCents, ceiling int64) (bool, error)
	// UndeliveredRewards are earned rewards not yet placed on a referrer's balance.
	UndeliveredRewards(ctx context.Context, max int32) ([]EarnedReward, error)
	// MarkDelivered stamps a reward as placed, reporting whether this call stamped it.
	MarkDelivered(ctx context.Context, id int64) (bool, error)
}

// Config is what this package cannot derive.
type Config struct {
	// SiteURL is where an invite link points. Without it Link cannot produce one — a
	// relative path pasted into a chat is not a link.
	SiteURL string

	// RewardCeiling bounds how many rewards one referrer may earn. Zero or negative means
	// DefaultRewardCeiling; there is no way to configure "unlimited", because the whole
	// point of the bound is that the patient version of the abuse has an end.
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

// Preview reports what a code is worth to this account, without consuming a seat or
// writing anything. It is what the checkout page calls while somebody is typing.
func (s *Service) Preview(ctx context.Context, userID int64, code string) (int16, error) {
	normalised, ok := normaliseCode(code)
	if !ok {
		return 0, ErrNotUsable
	}

	// Asked first, because it is the answer that helps: a caller who has already redeemed
	// cannot use ANY code, so reporting the code's own status would only be confusing. It
	// also keeps a caller who can never redeem from spending reads on this route.
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
// The claim itself is a single statement in the repository, so a seat cannot be taken
// twice and a second concurrent redemption by one account cannot half-succeed. What this
// method adds is the reason for a refusal, which the statement deliberately does not
// report.
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

	// The claim refused, and it does not say why. Only one of the reasons is the caller's
	// own, so that is the only one worth a second read.
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
// The offered value is drawn fresh on every attempt because the repository's upsert
// conflicts on the ACCOUNT, not on the code: an account that already has one gets it back
// and the drawn value is simply discarded, which costs nothing.
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
// Every way this can fail to attribute is a no-op rather than an error, because the code
// arrives in a cookie and a cookie is whatever the visitor put in it. The one caller is a
// registration path, and a referral must never cost somebody their account.
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

	// Refused here rather than left to the table's check constraint, so that the ordinary
	// case of somebody opening their own link is a no-op instead of a database error the
	// registration path has to recognise and swallow.
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
// An account can hold both a redeemed promo code and a pending invite, and a session
// admits one coupon. The larger wins: the buyer gets the better of the two rather than
// whichever they happened to acquire first, and a tie goes to the code they chose to type.
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

// normaliseCode folds a typed code to the one spelling the table holds and rejects
// anything the table's own constraint could never accept.
func normaliseCode(code string) (string, bool) {
	folded := strings.ToUpper(strings.TrimSpace(code))
	if !promoCodeShape.MatchString(folded) {
		return "", false
	}
	return folded, true
}

// mintCode draws an invite code from crypto/rand.
//
// Base64 in its URL alphabet, so the value is safe in a path segment without escaping and
// matches the shape the table checks. Not derived from the account id or from anything
// else about the person: this value is public the moment it is shared.
func mintCode() (string, error) {
	buf := make([]byte, inviteCodeBytes)
	if _, err := rand.Read(buf); err != nil {
		return "", fmt.Errorf("promo: drawing an invite code: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}
