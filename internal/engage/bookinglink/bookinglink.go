// Package bookinglink is the candidate's booking link: one unguessable public page per
// application where a recruiter, with no account, books a 30-minute slot from the
// candidate's default hours. See AGENTS.md for the rules every change here keeps.
package bookinglink

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"slices"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	aiplan "github.com/strelov1/freehire/internal/ai/plan"
	"github.com/strelov1/freehire/internal/engage/slots"
)

// The fixed shape of every link until the schedule editor (#4918): weekdays 10:00-18:00 in
// the candidate's zone, 30-minute slots, bookable from 12 hours to 14 days ahead.
const (
	SlotDuration  = 30 * time.Minute
	MinimumNotice = 12 * time.Hour
	Horizon       = 14 * 24 * time.Hour
	dayStartHour  = 10
	dayEndHour    = 18
	// seedBytes is the random part of a link: 256 bits from crypto/rand.
	seedBytes = 32
	// maxNameRunes and maxEmailBytes match booking_link_bookings' CHECKs.
	maxNameRunes  = 200
	maxEmailBytes = 254
)

var session = slots.SessionParams{Duration: SlotDuration, MinimumNotice: MinimumNotice, Horizon: Horizon}

// defaultRules is Mon-Fri dayStartHour-dayEndHour, built once; the inputs are constants, so
// a failure is a programming error.
var defaultRules = func() []slots.Rule {
	start, err1 := slots.NewTimeOfDay(dayStartHour, 0)
	end, err2 := slots.NewTimeOfDay(dayEndHour, 0)
	if err := errors.Join(err1, err2); err != nil {
		panic(err)
	}
	rules := make([]slots.Rule, 0, 5)
	for d := time.Monday; d <= time.Friday; d++ {
		r, err := slots.NewWeeklyRule(d, start, end)
		if err != nil {
			panic(err)
		}
		rules = append(rules, r)
	}
	return rules
}()

var (
	// ErrNotFound → 404: an application that is not the caller's, an unknown token, or a link
	// whose owner is no longer paid. One answer, so a stranger learns nothing from it.
	ErrNotFound = errors.New("bookinglink: not found")
	// ErrSlotUnavailable → 409: every ordinary refusal of a booking (off the grid, taken,
	// inside the notice, past the horizon, a lost race) is this one answer.
	ErrSlotUnavailable = errors.New("bookinglink: that time is no longer available")
	// ErrInvalidBooking → 400: an empty or overlong name, or an address that is not one.
	ErrInvalidBooking = errors.New("bookinglink: invalid booking")
	// ErrInvalidZone → 400: turning a link on with a zone that is not an IANA place.
	ErrInvalidZone = errors.New("bookinglink: invalid timezone")
)

// Link is one stored link. TokenSeed and TokenHash never go on the wire.
type Link struct {
	ID            int64
	UserID        int64
	ApplicationID int64
	TokenSeed     []byte
	TokenHash     []byte
	Zone          string
}

// NewBooking is a validated booking, ready to insert with its interview.
type NewBooking struct {
	Link           Link
	StartsAt       time.Time
	EndsAt         time.Time
	RecruiterName  string
	RecruiterEmail string
}

// Repository is the storage the service needs; QueriesRepository is the Postgres one.
type Repository interface {
	// Upsert turns the link on, keeping an existing seed. ok is false when the application
	// is not the user's.
	Upsert(ctx context.Context, userID, applicationID int64, seed, hash []byte, zone string) (l Link, ok bool, err error)
	ForApplication(ctx context.Context, userID, applicationID int64) (Link, bool, error)
	ByHash(ctx context.Context, hash []byte) (Link, bool, error)
	SetHash(ctx context.Context, linkID int64, hash []byte) error
	Busy(ctx context.Context, userID int64, from, to time.Time) ([]slots.Interval, error)
	// Book writes the booking, the application's interview and its ledger event in one
	// transaction, answering ErrSlotUnavailable when the EXCLUDE refuses it.
	Book(ctx context.Context, b NewBooking) error
}

// Entitlement is the plan question; *plan.Store satisfies it.
type Entitlement interface {
	RequirePaid(ctx context.Context, userID int64) (aiplan.Tier, error)
}

// Service turns links on and serves the public slot list and booking.
type Service struct {
	repo  Repository
	plans Entitlement
	// key derives a link's token from its seed, so the candidate can be shown the same URL
	// again while the database holds only the seed and the token's hash.
	key []byte
	now func() time.Time
}

// New builds the service. key must be a server secret.
func New(repo Repository, plans Entitlement, key []byte) *Service {
	return &Service{repo: repo, plans: plans, key: key, now: time.Now}
}

func (s *Service) token(seed []byte) string {
	m := hmac.New(sha256.New, s.key)
	m.Write([]byte("booking-link:"))
	m.Write(seed)
	return base64.RawURLEncoding.EncodeToString(m.Sum(nil))
}

func hashOf(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}

// tokenFor is the link's token, re-pointing the stored hash when the server key changed
// since it was written, so the URL shown is always one that resolves.
func (s *Service) tokenFor(ctx context.Context, l Link) (string, error) {
	tok := s.token(l.TokenSeed)
	if h := hashOf(tok); !hmac.Equal(h, l.TokenHash) {
		if err := s.repo.SetHash(ctx, l.ID, h); err != nil {
			return "", err
		}
	}
	return tok, nil
}

// Enable turns on the caller's link for one of their applications and returns its token.
// Calling it again returns the same token. A free account gets aiplan.ErrPaymentRequired
// and nothing is written.
func (s *Service) Enable(ctx context.Context, userID, applicationID int64, zone string) (string, error) {
	if _, err := s.plans.RequirePaid(ctx, userID); err != nil {
		return "", err
	}
	if _, ok := ianaZone(zone); !ok {
		return "", ErrInvalidZone
	}
	seed := make([]byte, seedBytes)
	if _, err := rand.Read(seed); err != nil {
		return "", err
	}
	l, ok, err := s.repo.Upsert(ctx, userID, applicationID, seed, hashOf(s.token(seed)), zone)
	if err != nil {
		return "", err
	}
	if !ok {
		return "", ErrNotFound
	}
	return s.tokenFor(ctx, l)
}

// TokenFor is the caller's existing link token for an application; ok is false when the link
// was never turned on.
func (s *Service) TokenFor(ctx context.Context, userID, applicationID int64) (string, bool, error) {
	l, ok, err := s.repo.ForApplication(ctx, userID, applicationID)
	if err != nil || !ok {
		return "", false, err
	}
	tok, err := s.tokenFor(ctx, l)
	return tok, err == nil, err
}

// live resolves a public token to its link, refusing one whose owner is no longer paid with
// the same ErrNotFound an unknown token gets.
func (s *Service) live(ctx context.Context, token string) (Link, *time.Location, error) {
	l, ok, err := s.repo.ByHash(ctx, hashOf(token))
	if err != nil {
		return Link{}, nil, err
	}
	if !ok {
		return Link{}, nil, ErrNotFound
	}
	if _, err := s.plans.RequirePaid(ctx, l.UserID); err != nil {
		if errors.Is(err, aiplan.ErrPaymentRequired) {
			return Link{}, nil, ErrNotFound
		}
		return Link{}, nil, err
	}
	zone, ok := ianaZone(l.Zone)
	if !ok {
		return Link{}, nil, fmt.Errorf("%w: booking link %d has timezone %q", slots.ErrNoOwnerZone, l.ID, l.Zone)
	}
	return l, zone, nil
}

// slotsFor runs the engine over the default hours and the owner's booked meetings. Busy is
// read a day wider than the window on both sides, as buddy's hostSlots does.
func (s *Service) slotsFor(ctx context.Context, l Link, zone *time.Location, from, to time.Time, viewerZone string) (slots.SlotResult, error) {
	busy, err := s.repo.Busy(ctx, l.UserID, from.AddDate(0, 0, -1), to.AddDate(0, 0, 1))
	if err != nil {
		return slots.SlotResult{}, err
	}
	return slots.Slots(slots.SlotRequest{
		Rules: defaultRules, OwnerZone: zone, Params: session, Busy: busy,
		From: from, To: to, Now: s.now(), ViewerZone: viewerZone,
	})
}

// Slots is the link's free slots from now to the horizon, in the viewer's zone (UTC when
// unknown; the result says which).
func (s *Service) Slots(ctx context.Context, token, viewerZone string) (slots.SlotResult, error) {
	l, zone, err := s.live(ctx, token)
	if err != nil {
		return slots.SlotResult{}, err
	}
	now := s.now()
	return s.slotsFor(ctx, l, zone, now, now.Add(Horizon+SlotDuration), viewerZone)
}

// BookInput is a recruiter's request for one slot.
type BookInput struct {
	Token string
	// StartsAt is the slot's start as the page showed it. The end is never taken from the
	// client.
	StartsAt time.Time
	Name     string
	Email    string
}

// Book takes one slot. The slot is re-derived from the live hours and busy time; the
// EXCLUDE, not this re-check, is what wins a race, and losing one is the same
// ErrSlotUnavailable a stale page gets.
func (s *Service) Book(ctx context.Context, in BookInput) (NewBooking, error) {
	name, email, err := validRecruiter(in.Name, in.Email)
	if err != nil {
		return NewBooking{}, err
	}
	l, zone, err := s.live(ctx, in.Token)
	if err != nil {
		return NewBooking{}, err
	}
	res, err := s.slotsFor(ctx, l, zone, in.StartsAt.AddDate(0, 0, -1), in.StartsAt.AddDate(0, 0, 1), "UTC")
	if err != nil {
		return NewBooking{}, err
	}
	if !slices.ContainsFunc(res.Slots, func(iv slots.Interval) bool { return iv.Start.Equal(in.StartsAt) }) {
		return NewBooking{}, ErrSlotUnavailable
	}
	b := NewBooking{
		Link: l, StartsAt: in.StartsAt.UTC(), EndsAt: in.StartsAt.UTC().Add(SlotDuration),
		RecruiterName: name, RecruiterEmail: email,
	}
	if err := s.repo.Book(ctx, b); err != nil {
		return NewBooking{}, err
	}
	return b, nil
}

// validRecruiter trims and checks the recruiter's name and address. The address must be a
// bare one (no display name), since it is shown to the candidate as given.
func validRecruiter(name, email string) (string, string, error) {
	name, email = strings.TrimSpace(name), strings.TrimSpace(email)
	if name == "" || utf8.RuneCountInString(name) > maxNameRunes || hasControl(name) {
		return "", "", fmt.Errorf("%w: a name is required", ErrInvalidBooking)
	}
	addr, err := mail.ParseAddress(email)
	if err != nil || addr.Address != email || len(email) > maxEmailBytes || hasControl(email) {
		return "", "", fmt.Errorf("%w: a valid email is required", ErrInvalidBooking)
	}
	return name, email, nil
}

// hasControl reports a control character (NUL among them), which Postgres text refuses or
// a header would split on.
func hasControl(s string) bool { return strings.IndexFunc(s, unicode.IsControl) >= 0 }

// ianaZone loads an IANA zone name, refusing what time.LoadLocation accepts but is not a
// place ("Local" is the SERVER's zone; "EST" and "Factory" are nowhere), as buddy's does.
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
