package employer

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"net/mail"
	"net/url"
	"strings"
	"time"
)

// Membership grants access to the company inbox. Legacy members also manage the
// company, while new invitations grant the recruiter role.

// Team sentinels, mapped to HTTP statuses by the handler.
var (
	ErrManagementRequired = errors.New("employer: company management access required")
	ErrInvitationEmail    = errors.New("employer: confirm and use the invited email to join")
	// ErrInvitationInvalid is a link that names no live invitation: never issued, already
	// accepted, withdrawn, replaced by a resend, or expired. One error for all of them — the
	// holder of a dead link can do nothing different about any one.
	ErrInvitationInvalid = errors.New("employer: this invitation link is invalid or has expired")
	// ErrMemberNotFound is a removal of somebody who is not a member of the caller's team.
	ErrMemberNotFound = errors.New("employer: no such team member")
	// ErrInvitationNotFound is a withdrawal of an invitation the caller's team did not send.
	ErrInvitationNotFound = errors.New("employer: no such invitation")
	// ErrTooManyInvitations is a new invitation past maxLiveInvitations.
	ErrTooManyInvitations = errors.New("employer: too many open invitations — withdraw one first")
)

const (
	// invitationTTL is how long an invitation link works.
	invitationTTL = 7 * 24 * time.Hour
	// maxLiveInvitations bounds the open invitations of one company: each is a letter
	// freehire sends to an address the account typed.
	maxLiveInvitations = 20
)

// Member is one person who acts for a company. Holder marks the claimant, who cannot be
// removed.
type Member struct {
	UserID    int64
	WorkEmail string
	JoinedAt  time.Time
	Holder    bool
	Role      string
}

// Activity is a company audit event. It contains no candidate content.
type Activity struct {
	ID            int64
	Action        string
	ActorEmail    string
	ApplicationID *int64
	PreviousStage string
	Stage         string
	SubjectEmail  string
	OccurredAt    time.Time
}

// Invitation is an invitation that can still be accepted.
type Invitation struct {
	ID        int64
	Email     string
	CreatedAt time.Time
	ExpiresAt time.Time
}

// Team is everyone who acts for a company, and who has been invited to.
type Team struct {
	Members     []Member
	Invitations []Invitation
}

// InvitationPreview is what an invitation link says before it is accepted.
type InvitationPreview struct {
	CompanyName string
	Email       string
	Role        string
}

// NewInvitation is an invitation to store: the token's hash, never the token.
type NewInvitation struct {
	CompanySlug string
	Email       string
	TokenHash   []byte
	InvitedBy   int64
	ExpiresAt   time.Time
}

// TeamRepository is the persistence of a company's team.
type TeamRepository interface {
	Activity(ctx context.Context, companySlug string, actorID, beforeID int64) ([]Activity, error)
	Members(ctx context.Context, companySlug string) ([]Member, error)
	Invitations(ctx context.Context, companySlug string) ([]Invitation, error)
	// Invite stores an invitation, replacing one already sent to the same address. It
	// returns ErrTooManyInvitations when a new address would pass maxLive open ones.
	Invite(ctx context.Context, inv NewInvitation, maxLive int64) (Invitation, error)
	// Withdraw deletes one of the company's invitations; ErrInvitationNotFound otherwise.
	// ErrNotFound when actorID no longer acts for the company, checked under its lock.
	Withdraw(ctx context.Context, companySlug string, actorID, id int64) error
	// WithdrawToken deletes the invitation only while it still carries tokenHash.
	WithdrawToken(ctx context.Context, companySlug string, tokenHash []byte) error
	// Preview reads the invitation a token hash names; ErrInvitationInvalid when none, or
	// when it has expired. status is the inviting account's.
	Preview(ctx context.Context, tokenHash []byte) (preview InvitationPreview, status string, err error)
	// Accept consumes a live invitation to an active account and makes userID a member.
	// ErrInvitationInvalid when it cannot be accepted, ErrAlreadyHasAccount when userID
	// already acts for a company.
	Accept(ctx context.Context, userID int64, tokenHash []byte) error
	// Remove takes a member out of the company's team and hands the vacancies they
	// published to the claimant; ErrMemberNotFound when they are not a member of it.
	// ErrNotFound when actorID no longer acts for the company, checked under its lock, so a
	// request queued by a member removed meanwhile changes nothing.
	Remove(ctx context.Context, companySlug string, actorID, memberID int64) error
}

// InviteMailer delivers a team invitation. A port like ClaimMailer, exported for the same
// typed-nil reason (see ClaimMailer).
type InviteMailer interface {
	SendTeamInvitation(ctx context.Context, m TeamInvitationMail) error
}

// TeamInvitationMail is the letter an invited colleague receives.
type TeamInvitationMail struct {
	To          string
	CompanyName string
	InvitedBy   string // the inviting member's work email
	Link        string // the join page, carrying the token
	ExpiresAt   time.Time
}

// WithTeam enables the team: its persistence, the mailer of its invitations (nil when no
// mail transport is configured — inviting then answers ErrMailUnavailable), and the join
// page each invitation links to, which receives the token as `?token=`.
func (s *Service) WithTeam(repo TeamRepository, mailer InviteMailer, joinURL string) *Service {
	s.team, s.invites, s.joinURL = repo, mailer, joinURL
	return s
}

// Team lists everyone who acts for the caller's company and the invitations still open.
func (s *Service) Team(ctx context.Context, userID int64) (Team, error) {
	acc, err := s.ActiveAccount(ctx, userID)
	if err != nil {
		return Team{}, err
	}
	members, err := s.team.Members(ctx, acc.CompanySlug)
	if err != nil {
		return Team{}, err
	}
	if acc.Role == RoleRecruiter {
		for _, member := range members {
			if member.UserID == userID {
				return Team{Members: []Member{member}}, nil
			}
		}
		return Team{}, ErrNotFound
	}
	invitations, err := s.team.Invitations(ctx, acc.CompanySlug)
	if err != nil {
		return Team{}, err
	}
	return Team{Members: members, Invitations: invitations}, nil
}

// Activity returns the newest company events before the optional cursor.
func (s *Service) Activity(ctx context.Context, userID, beforeID int64) ([]Activity, error) {
	acc, err := s.ActiveAccount(ctx, userID)
	if err != nil {
		return nil, err
	}
	return s.team.Activity(ctx, acc.CompanySlug, userID, beforeID)
}

// Invite mails a colleague a link that adds them to the caller's company team. The address
// must be a work address, as for a claim, and not one somebody in the team already acts
// under. Inviting an address again replaces its link and restarts its expiry.
func (s *Service) Invite(ctx context.Context, userID int64, email string) (Invitation, error) {
	acc, err := s.managementAccount(ctx, userID)
	if err != nil {
		return Invitation{}, err
	}
	email = strings.TrimSpace(email)
	if addr, err := mail.ParseAddress(email); err != nil || addr.Address != email {
		return Invitation{}, fmt.Errorf("%w: %q is not an email address", ErrInvalid, email)
	}
	if isPublicWebmailDomain(email) {
		return Invitation{}, ErrPublicWebmailDomain
	}
	if s.invites == nil {
		return Invitation{}, ErrMailUnavailable
	}
	members, err := s.team.Members(ctx, acc.CompanySlug)
	if err != nil {
		return Invitation{}, err
	}
	for _, m := range members {
		if strings.EqualFold(m.WorkEmail, email) {
			return Invitation{}, fmt.Errorf("%w: %s is already in the team", ErrInvalid, email)
		}
	}

	token, hash, err := newInvitationToken()
	if err != nil {
		return Invitation{}, err
	}
	inv, err := s.team.Invite(ctx, NewInvitation{
		CompanySlug: acc.CompanySlug,
		Email:       email,
		TokenHash:   hash,
		InvitedBy:   userID,
		ExpiresAt:   time.Now().Add(invitationTTL),
	}, maxLiveInvitations)
	if err != nil {
		return Invitation{}, err
	}
	link := s.joinURL + "?" + url.Values{"token": {token}}.Encode()
	if err := s.invites.SendTeamInvitation(ctx, TeamInvitationMail{
		To: email, CompanyName: acc.CompanyName, InvitedBy: acc.WorkEmail, Link: link, ExpiresAt: inv.ExpiresAt,
	}); err != nil {
		// The stored invitation names a token nobody received; withdraw it so the list
		// does not show a colleague as invited when no letter reached them — but only while
		// it still carries this token: a concurrent resend may have replaced it and mailed a
		// link that must keep working.
		if wErr := s.team.WithdrawToken(ctx, acc.CompanySlug, hash); wErr != nil {
			return Invitation{}, fmt.Errorf("send invitation: %w (and withdrawing it failed: %v)", err, wErr)
		}
		return Invitation{}, fmt.Errorf("send invitation: %w", err)
	}
	return inv, nil
}

// WithdrawInvitation cancels one of the caller's company's invitations.
func (s *Service) WithdrawInvitation(ctx context.Context, userID, id int64) error {
	acc, err := s.managementAccount(ctx, userID)
	if err != nil {
		return err
	}
	return s.team.Withdraw(ctx, acc.CompanySlug, userID, id)
}

// PreviewInvitation says which company a link invites to and at which address, for the
// join page to show before the visitor accepts. It needs no account of the visitor's.
func (s *Service) PreviewInvitation(ctx context.Context, token string) (InvitationPreview, error) {
	p, status, err := s.team.Preview(ctx, hashInvitationToken(token))
	if err != nil {
		return InvitationPreview{}, err
	}
	if status != StatusActive {
		return InvitationPreview{}, ErrNotActive
	}
	return p, nil
}

// AcceptInvitation makes a user a member of the company named by the link. New
// recruiter invitations require the matching verified sign-in address.
func (s *Service) AcceptInvitation(ctx context.Context, userID int64, token string) (Account, error) {
	if _, err := s.repo.GetByUserID(ctx, userID); err == nil {
		return Account{}, ErrAlreadyHasAccount
	} else if !errors.Is(err, ErrNotFound) {
		return Account{}, err
	}
	if _, err := s.PreviewInvitation(ctx, token); err != nil {
		return Account{}, err
	}
	if err := s.team.Accept(ctx, userID, hashInvitationToken(token)); err != nil {
		return Account{}, err
	}
	return s.repo.GetByUserID(ctx, userID)
}

// RemoveMember takes memberID out of the caller's company team; a member may remove
// themselves (leave). The vacancies they published stay with the team, handed to the
// claimant. The claimant is the account and cannot be removed.
func (s *Service) RemoveMember(ctx context.Context, userID, memberID int64) error {
	acc, err := s.ActiveAccount(ctx, userID)
	if err != nil {
		return err
	}
	if acc.Role == RoleRecruiter && memberID != userID {
		return ErrManagementRequired
	}
	if memberID == acc.UserID {
		return fmt.Errorf("%w: the account holder cannot be removed from the team", ErrInvalid)
	}
	return s.team.Remove(ctx, acc.CompanySlug, userID, memberID)
}

// newInvitationToken mints a 256-bit link token and the hash stored for it.
func newInvitationToken() (token string, hash []byte, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", nil, err
	}
	token = base64.RawURLEncoding.EncodeToString(b)
	return token, hashInvitationToken(token), nil
}

func hashInvitationToken(token string) []byte {
	h := sha256.Sum256([]byte(token))
	return h[:]
}
