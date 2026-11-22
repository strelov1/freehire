package employer

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
)

// fakeTeam holds one company's members and invitations, keyed by the token hash.
type fakeTeam struct {
	members     []Member
	invitations map[string]NewInvitation // string(token hash) -> invitation
	nextID      int64
	ids         map[string]int64
	accepted    []int64
	removed     []int64
	// actors is who each Withdraw and Remove was made by, in order.
	actors []int64
	status string
}

func newFakeTeam(holder int64) *fakeTeam {
	return &fakeTeam{
		members:     []Member{{UserID: holder, WorkEmail: "hr@acme.test", Holder: true}},
		invitations: map[string]NewInvitation{},
		ids:         map[string]int64{},
		status:      StatusActive,
	}
}

func (f *fakeTeam) Activity(context.Context, string, int64, int64) ([]Activity, error) {
	panic("unexpected activity read")
}

func (f *fakeTeam) Members(context.Context, string) ([]Member, error) { return f.members, nil }

func (f *fakeTeam) Invitations(context.Context, string) ([]Invitation, error) {
	var out []Invitation
	for h, inv := range f.invitations {
		out = append(out, Invitation{ID: f.ids[h], Email: inv.Email})
	}
	return out, nil
}

func (f *fakeTeam) Invite(_ context.Context, inv NewInvitation, maxLive int64) (Invitation, error) {
	for h, old := range f.invitations {
		if strings.EqualFold(old.Email, inv.Email) {
			id := f.ids[h]
			delete(f.invitations, h)
			delete(f.ids, h)
			f.invitations[string(inv.TokenHash)], f.ids[string(inv.TokenHash)] = inv, id
			return Invitation{ID: id, Email: inv.Email, ExpiresAt: inv.ExpiresAt}, nil
		}
	}
	if int64(len(f.invitations)) >= maxLive {
		return Invitation{}, ErrTooManyInvitations
	}
	f.nextID++
	f.invitations[string(inv.TokenHash)], f.ids[string(inv.TokenHash)] = inv, f.nextID
	return Invitation{ID: f.nextID, Email: inv.Email, ExpiresAt: inv.ExpiresAt}, nil
}

func (f *fakeTeam) Withdraw(_ context.Context, _ string, actorID, id int64) error {
	f.actors = append(f.actors, actorID)
	for h, i := range f.ids {
		if i == id {
			delete(f.invitations, h)
			delete(f.ids, h)
			return nil
		}
	}
	return ErrInvitationNotFound
}

func (f *fakeTeam) WithdrawToken(_ context.Context, _ string, hash []byte) error {
	delete(f.invitations, string(hash))
	delete(f.ids, string(hash))
	return nil
}

func (f *fakeTeam) Preview(_ context.Context, hash []byte) (InvitationPreview, string, error) {
	inv, ok := f.invitations[string(hash)]
	if !ok {
		return InvitationPreview{}, "", ErrInvitationInvalid
	}
	return InvitationPreview{CompanyName: "Acme", Email: inv.Email}, f.status, nil
}

func (f *fakeTeam) Accept(_ context.Context, userID int64, hash []byte) error {
	inv, ok := f.invitations[string(hash)]
	if !ok {
		return ErrInvitationInvalid
	}
	delete(f.invitations, string(hash))
	f.accepted = append(f.accepted, userID)
	f.members = append(f.members, Member{UserID: userID, WorkEmail: inv.Email})
	return nil
}

func (f *fakeTeam) Remove(_ context.Context, _ string, actorID, memberID int64) error {
	f.actors = append(f.actors, actorID)
	for i, m := range f.members {
		if m.UserID == memberID && !m.Holder {
			f.members = append(f.members[:i], f.members[i+1:]...)
			f.removed = append(f.removed, memberID)
			return nil
		}
	}
	return ErrMemberNotFound
}

type fakeInviteMailer struct {
	sent []TeamInvitationMail
	err  error
}

func (m *fakeInviteMailer) SendTeamInvitation(_ context.Context, mail TeamInvitationMail) error {
	if m.err != nil {
		return m.err
	}
	m.sent = append(m.sent, mail)
	return nil
}

const holderID = int64(1)

func teamService(t *testing.T) (*Service, *fakeTeam, *fakeInviteMailer, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	s := New(repo, newFakeCodeIssuer(), &fakeClaimMailer{}, newFakeJobRepo(), &fakeMinter{})
	repo.companies["acme"] = struct{ name, website string }{name: "Acme", website: "https://acme.test"}
	if _, err := s.Claim(context.Background(), holderID, "Acme", "hr@acme.test"); err != nil {
		t.Fatalf("Claim: %v", err)
	}
	if _, err := s.ConfirmClaim(context.Background(), holderID, "654321"); err != nil {
		t.Fatalf("ConfirmClaim: %v", err)
	}
	team, mailer := newFakeTeam(holderID), &fakeInviteMailer{}
	s.WithTeam(team, mailer, "https://freehire.me/my/employer/join")
	return s, team, mailer, repo
}

// tokenOf reads the token out of the link a letter carries.
func tokenOf(t *testing.T, link string) string {
	t.Helper()
	u, err := url.Parse(link)
	if err != nil {
		t.Fatalf("parse link %q: %v", link, err)
	}
	return u.Query().Get("token")
}

func TestInvite_MailsALinkAndStoresOnlyItsHash(t *testing.T) {
	s, team, mailer, _ := teamService(t)
	inv, err := s.Invite(context.Background(), holderID, " dev@acme.test ")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}
	if inv.Email != "dev@acme.test" || len(mailer.sent) != 1 {
		t.Fatalf("invitation %+v, letters %d", inv, len(mailer.sent))
	}
	m := mailer.sent[0]
	if m.To != "dev@acme.test" || m.CompanyName != "Acme" || m.InvitedBy != "hr@acme.test" ||
		!strings.HasPrefix(m.Link, "https://freehire.me/my/employer/join?token=") {
		t.Errorf("letter = %+v", m)
	}
	token := tokenOf(t, m.Link)
	if _, ok := team.invitations[token]; ok {
		t.Error("the raw token was stored")
	}
	if _, ok := team.invitations[string(hashInvitationToken(token))]; !ok {
		t.Error("the token's hash was not stored")
	}
}

func TestInvite_Refusals(t *testing.T) {
	cases := map[string]struct {
		email string
		setup func(s *Service, team *fakeTeam, m *fakeInviteMailer)
		want  error
	}{
		"not an address":      {email: "dev at acme", want: ErrInvalid},
		"a public webmail":    {email: "dev@gmail.com", want: ErrPublicWebmailDomain},
		"already in the team": {email: "HR@acme.test", want: ErrInvalid},
		"no mail transport":   {email: "dev@acme.test", setup: func(s *Service, _ *fakeTeam, _ *fakeInviteMailer) { s.invites = nil }, want: ErrMailUnavailable},
		"the open invitation cap": {
			email: "dev@acme.test",
			setup: func(_ *Service, team *fakeTeam, _ *fakeInviteMailer) {
				for i := range maxLiveInvitations {
					team.invitations[string(rune('a'+i))] = NewInvitation{Email: string(rune('a'+i)) + "@acme.test"}
				}
			},
			want: ErrTooManyInvitations,
		},
	}
	for name, c := range cases {
		s, team, mailer, _ := teamService(t)
		if c.setup != nil {
			c.setup(s, team, mailer)
		}
		if _, err := s.Invite(context.Background(), holderID, c.email); !errors.Is(err, c.want) {
			t.Errorf("%s: err = %v, want %v", name, err, c.want)
		}
		if len(mailer.sent) != 0 {
			t.Errorf("%s: a refused invitation was mailed", name)
		}
	}
}

func TestInvite_AFailedLetterLeavesNoInvitation(t *testing.T) {
	s, team, mailer, _ := teamService(t)
	mailer.err = errors.New("ses down")
	if _, err := s.Invite(context.Background(), holderID, "dev@acme.test"); err == nil {
		t.Fatal("Invite succeeded with a failed letter")
	}
	if len(team.invitations) != 0 {
		t.Error("an invitation nobody received is still listed")
	}
}

func TestInvite_AResendReplacesTheLink(t *testing.T) {
	s, team, mailer, _ := teamService(t)
	for range 2 {
		if _, err := s.Invite(context.Background(), holderID, "dev@acme.test"); err != nil {
			t.Fatalf("Invite: %v", err)
		}
	}
	if len(team.invitations) != 1 {
		t.Fatalf("invitations = %d, want the resend to replace the first", len(team.invitations))
	}
	if _, err := s.PreviewInvitation(context.Background(), tokenOf(t, mailer.sent[0].Link)); !errors.Is(err, ErrInvitationInvalid) {
		t.Errorf("the first link still works: err = %v", err)
	}
}

func TestAcceptInvitation(t *testing.T) {
	s, team, mailer, repo := teamService(t)
	if _, err := s.Invite(context.Background(), holderID, "dev@acme.test"); err != nil {
		t.Fatalf("Invite: %v", err)
	}
	token := tokenOf(t, mailer.sent[0].Link)

	p, err := s.PreviewInvitation(context.Background(), token)
	if err != nil || p.CompanyName != "Acme" || p.Email != "dev@acme.test" {
		t.Fatalf("Preview = %+v, %v", p, err)
	}

	// The claimant of another account cannot join: one company per user.
	repo.accounts[7] = Account{UserID: 7, CompanySlug: "globex", Status: StatusActive}
	if _, err := s.AcceptInvitation(context.Background(), 7, token); !errors.Is(err, ErrAlreadyHasAccount) {
		t.Errorf("accept by a claimant err = %v, want ErrAlreadyHasAccount", err)
	}

	team.status = StatusRevoked
	if _, err := s.AcceptInvitation(context.Background(), 9, token); !errors.Is(err, ErrNotActive) {
		t.Errorf("accept into a revoked account err = %v, want ErrNotActive", err)
	}
	team.status = StatusActive

	if _, err := s.AcceptInvitation(context.Background(), 9, "not-a-token"); !errors.Is(err, ErrInvitationInvalid) {
		t.Errorf("accept of an unknown link err = %v, want ErrInvitationInvalid", err)
	}
	if len(team.accepted) != 0 {
		t.Fatalf("accepted = %v, want every refusal to leave the invitation unused", team.accepted)
	}
	// A successful acceptance, and the account it reads back, is covered against Postgres
	// (team_integration_test.go): it depends on the membership the database records.
}

func TestRemoveMember(t *testing.T) {
	s, team, _, _ := teamService(t)
	team.members = append(team.members, Member{UserID: 5, WorkEmail: "dev@acme.test"})

	if err := s.RemoveMember(context.Background(), holderID, holderID); !errors.Is(err, ErrInvalid) {
		t.Errorf("removing the holder err = %v, want ErrInvalid", err)
	}
	if err := s.RemoveMember(context.Background(), holderID, 42); !errors.Is(err, ErrMemberNotFound) {
		t.Errorf("removing a stranger err = %v, want ErrMemberNotFound", err)
	}
	if err := s.RemoveMember(context.Background(), holderID, 5); err != nil || len(team.removed) != 1 {
		t.Errorf("RemoveMember = %v, removed %v", err, team.removed)
	}
}

// A member's withdrawal and removal are made as the member, not as the claimant the
// company's Account names: the repository rechecks that actor under the company's lock, and
// the claimant's id would pass it for a member removed meanwhile.
func TestTeam_AMembersChangesActAsTheMember(t *testing.T) {
	s, team, _, repo := teamService(t)
	const memberID, otherID = 5, 6
	acc := repo.accounts[holderID]
	acc.Member = true
	repo.accounts[memberID] = acc
	team.members = append(team.members, Member{UserID: memberID, WorkEmail: "dev@acme.test"}, Member{UserID: otherID, WorkEmail: "ops@acme.test"})
	inv, err := s.Invite(context.Background(), memberID, "qa@acme.test")
	if err != nil {
		t.Fatalf("Invite: %v", err)
	}

	if err := s.WithdrawInvitation(context.Background(), memberID, inv.ID); err != nil {
		t.Fatalf("WithdrawInvitation: %v", err)
	}
	if err := s.RemoveMember(context.Background(), memberID, otherID); err != nil {
		t.Fatalf("RemoveMember: %v", err)
	}
	if len(team.actors) != 2 || team.actors[0] != memberID || team.actors[1] != memberID {
		t.Errorf("actors = %v, want both changes made as the member (%d), not the claimant (%d)", team.actors, memberID, holderID)
	}
}

func TestTeam_RefusesAnAccountThatIsNotActive(t *testing.T) {
	s, _, _, repo := teamService(t)
	acc := repo.accounts[holderID]
	acc.Status = StatusRevoked
	repo.accounts[holderID] = acc
	if _, err := s.Team(context.Background(), holderID); !errors.Is(err, ErrNotActive) {
		t.Errorf("Team err = %v, want ErrNotActive", err)
	}
	if _, err := s.Invite(context.Background(), holderID, "dev@acme.test"); !errors.Is(err, ErrNotActive) {
		t.Errorf("Invite err = %v, want ErrNotActive", err)
	}
}
