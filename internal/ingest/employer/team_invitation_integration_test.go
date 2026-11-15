//go:build integration

// Integration tests for the company team's invitations against Postgres: the single-use
// link, one company per user across the two tables, the open-invitation cap, and removal.
// Run with: go test -tags=integration ./internal/ingest/employer/
package employer_test

import (
	"context"
	"crypto/sha256"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/ingest/employer"
)

func (f teamFixture) invite(t *testing.T, email string, expires time.Time) []byte {
	t.Helper()
	h := sha256.Sum256([]byte("token-" + email + expires.String()))
	if _, err := f.repo.Invite(context.Background(), employer.NewInvitation{
		CompanySlug: "acme", Email: email, TokenHash: h[:], InvitedBy: f.holder, ExpiresAt: expires,
	}, 20); err != nil {
		t.Fatalf("Invite %s: %v", email, err)
	}
	return h[:]
}

func TestTeam_InvitationsThatCannotBeAccepted(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()

	expired := f.invite(t, "late@acme.test", time.Now().Add(-time.Minute))
	if _, _, err := f.repo.Preview(ctx, expired); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("Preview of an expired link err = %v", err)
	}
	if err := f.repo.Accept(ctx, seedUser(t, f.pool, "late@example.test"), expired); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("Accept of an expired link err = %v", err)
	}

	// The holder of another account cannot become a member.
	other := seedUser(t, f.pool, "hr@globex.test")
	if _, err := f.pool.Exec(ctx, `INSERT INTO company_accounts (user_id, company_slug, company_name, work_email, status) VALUES ($1, 'globex', 'Globex', 'hr@globex.test', 'active')`, other); err != nil {
		t.Fatalf("seed globex: %v", err)
	}
	live := f.invite(t, "hr2@acme.test", time.Now().Add(time.Hour))
	if err := f.repo.Accept(ctx, other, live); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("Accept by another account's holder err = %v, want a refusal", err)
	}

	// A revoked account takes no new members.
	if _, err := f.pool.Exec(ctx, `UPDATE company_accounts SET status = 'revoked' WHERE company_slug = 'acme'`); err != nil {
		t.Fatalf("revoke: %v", err)
	}
	if err := f.repo.Accept(ctx, seedUser(t, f.pool, "new@example.test"), live); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("Accept into a revoked account err = %v", err)
	}
}

func TestTeam_InvitationCapAndResend(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	until := time.Now().Add(time.Hour)
	for _, e := range []string{"a@acme.test", "b@acme.test"} {
		h := sha256.Sum256([]byte(e))
		if _, err := f.repo.Invite(ctx, employer.NewInvitation{CompanySlug: "acme", Email: e, TokenHash: h[:], InvitedBy: f.holder, ExpiresAt: until}, 2); err != nil {
			t.Fatalf("Invite %s: %v", e, err)
		}
	}
	h := sha256.Sum256([]byte("c"))
	if _, err := f.repo.Invite(ctx, employer.NewInvitation{CompanySlug: "acme", Email: "c@acme.test", TokenHash: h[:], InvitedBy: f.holder, ExpiresAt: until}, 2); !errors.Is(err, employer.ErrTooManyInvitations) {
		t.Errorf("a third address past the cap err = %v, want ErrTooManyInvitations", err)
	}
	// A resend to an invited address passes the cap and replaces the link.
	h2 := sha256.Sum256([]byte("a-again"))
	if _, err := f.repo.Invite(ctx, employer.NewInvitation{CompanySlug: "acme", Email: "A@acme.test", TokenHash: h2[:], InvitedBy: f.holder, ExpiresAt: until}, 2); err != nil {
		t.Fatalf("resend: %v", err)
	}
	old := sha256.Sum256([]byte("a@acme.test"))
	if _, _, err := f.repo.Preview(ctx, old[:]); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("the replaced link still works: %v", err)
	}
	invs, err := f.repo.Invitations(ctx, "acme")
	if err != nil || len(invs) != 2 {
		t.Fatalf("Invitations = %+v, %v", invs, err)
	}
	if err := f.repo.Withdraw(ctx, "acme", f.holder, invs[0].ID); err != nil {
		t.Errorf("Withdraw: %v", err)
	}
	if err := f.repo.Withdraw(ctx, "globex", f.holder, invs[1].ID); !errors.Is(err, employer.ErrNotFound) {
		t.Errorf("a Withdraw for a company the caller does not act for err = %v, want ErrNotFound", err)
	}
}

func TestTeam_RemovalWithdrawsTheMembersInvitations(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := seedUser(t, f.pool, "dev.personal@example.test")
	if err := f.repo.Accept(ctx, member, f.invite(t, "dev@acme.test", time.Now().Add(time.Hour))); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	h := sha256.Sum256([]byte("member-invites-themselves"))
	if _, err := f.repo.Invite(ctx, employer.NewInvitation{
		CompanySlug: "acme", Email: "me-again@acme.test", TokenHash: h[:], InvitedBy: member, ExpiresAt: time.Now().Add(time.Hour),
	}, 20); err != nil {
		t.Fatalf("member's Invite: %v", err)
	}
	if err := f.repo.Remove(ctx, "acme", f.holder, member); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	if err := f.repo.Accept(ctx, member, h[:]); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("a removed member re-joining through their own invitation err = %v, want ErrInvitationInvalid", err)
	}
}

func TestTeam_ReviewFixes(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	until := time.Now().Add(time.Hour)
	inv := func(email string, hash []byte, by int64, exp time.Time, maxLive int64) error {
		_, err := f.repo.Invite(ctx, employer.NewInvitation{CompanySlug: "acme", Email: email, TokenHash: hash, InvitedBy: by, ExpiresAt: exp}, maxLive)
		return err
	}
	h := func(s string) []byte { b := sha256.Sum256([]byte(s)); return b[:] }

	t.Run("reviving an expired invitation needs a free slot", func(t *testing.T) {
		if err := inv("old@acme.test", h("old"), f.holder, time.Now().Add(-time.Minute), 2); err != nil {
			t.Fatalf("seed expired: %v", err)
		}
		for _, e := range []string{"a@acme.test", "b@acme.test"} {
			if err := inv(e, h(e), f.holder, until, 2); err != nil {
				t.Fatalf("Invite %s: %v", e, err)
			}
		}
		if err := inv("old@acme.test", h("old-again"), f.holder, until, 2); !errors.Is(err, employer.ErrTooManyInvitations) {
			t.Errorf("reviving an expired invitation at the cap err = %v, want ErrTooManyInvitations", err)
		}
		// Renewing one that is still live passes.
		if err := inv("a@acme.test", h("a-again"), f.holder, until, 2); err != nil {
			t.Errorf("renewing a live invitation at the cap: %v", err)
		}
	})

	t.Run("a removed member can no longer invite, withdraw or remove", func(t *testing.T) {
		member := seedUser(t, f.pool, "m.personal@example.test")
		if err := inv("m@acme.test", h("m"), f.holder, until, 20); err != nil {
			t.Fatalf("Invite m: %v", err)
		}
		if err := f.repo.Accept(ctx, member, h("m")); err != nil {
			t.Fatalf("Accept: %v", err)
		}
		if err := f.repo.Remove(ctx, "acme", f.holder, member); err != nil {
			t.Fatalf("Remove: %v", err)
		}
		if err := inv("me-again@acme.test", h("me-again"), member, until, 20); !errors.Is(err, employer.ErrNotFound) {
			t.Errorf("Invite by a removed member err = %v, want ErrNotFound", err)
		}
		// A request the member queued before their removal, carried out after it, changes
		// nothing: the actor is rechecked under the company's lock.
		colleague := seedUser(t, f.pool, "c.personal@example.test")
		if err := inv("c@acme.test", h("c"), f.holder, until, 20); err != nil {
			t.Fatalf("Invite c: %v", err)
		}
		if err := f.repo.Accept(ctx, colleague, h("c")); err != nil {
			t.Fatalf("Accept c: %v", err)
		}
		if err := f.repo.Remove(ctx, "acme", member, colleague); !errors.Is(err, employer.ErrNotFound) {
			t.Errorf("Remove by a removed member err = %v, want ErrNotFound", err)
		}
		if _, err := f.repo.GetByUserID(ctx, colleague); err != nil {
			t.Errorf("the colleague after a removed member's Remove: %v, want still a member", err)
		}
		invs, err := f.repo.Invitations(ctx, "acme")
		if err != nil || len(invs) == 0 {
			t.Fatalf("Invitations = %v, %v", invs, err)
		}
		if err := f.repo.Withdraw(ctx, "acme", member, invs[0].ID); !errors.Is(err, employer.ErrNotFound) {
			t.Errorf("Withdraw by a removed member err = %v, want ErrNotFound", err)
		}
	})

	t.Run("withdrawing by token spares a resend that replaced it", func(t *testing.T) {
		if err := inv("r@acme.test", h("first"), f.holder, until, 20); err != nil {
			t.Fatalf("Invite: %v", err)
		}
		if err := inv("r@acme.test", h("second"), f.holder, until, 20); err != nil {
			t.Fatalf("resend: %v", err)
		}
		if err := f.repo.WithdrawToken(ctx, "acme", h("first")); err != nil {
			t.Fatalf("WithdrawToken: %v", err)
		}
		if _, _, err := f.repo.Preview(ctx, h("second")); err != nil {
			t.Errorf("the resend's link after the first token was withdrawn: %v", err)
		}
		if err := f.repo.WithdrawToken(ctx, "acme", h("second")); err != nil {
			t.Fatalf("WithdrawToken(second): %v", err)
		}
		if _, _, err := f.repo.Preview(ctx, h("second")); !errors.Is(err, employer.ErrInvitationInvalid) {
			t.Errorf("after withdrawing its own token err = %v, want ErrInvitationInvalid", err)
		}
	})
}

func TestTeam_AnInvitationMakesAMember(t *testing.T) {
	f := newTeamFixture(t)
	ctx := context.Background()
	member := seedUser(t, f.pool, "someone@gmail.com")
	hash := f.invite(t, "dev@acme.test", time.Now().Add(time.Hour))

	if p, status, err := f.repo.Preview(ctx, hash); err != nil || p.CompanyName != "Acme" || p.Email != "dev@acme.test" || status != employer.StatusActive {
		t.Fatalf("Preview = %+v, %q, %v", p, status, err)
	}
	if err := f.repo.Accept(ctx, member, hash); err != nil {
		t.Fatalf("Accept: %v", err)
	}
	if err := f.repo.Accept(ctx, seedUser(t, f.pool, "other@example.test"), hash); !errors.Is(err, employer.ErrInvitationInvalid) {
		t.Errorf("second Accept of one link err = %v, want ErrInvitationInvalid", err)
	}

	acc, err := f.repo.GetByUserID(ctx, member)
	if err != nil {
		t.Fatalf("GetByUserID(member): %v", err)
	}
	if acc.UserID != f.holder || acc.CompanySlug != "acme" || acc.WorkEmail != "dev@acme.test" || !acc.Member || !acc.Active() {
		t.Errorf("member's account = %+v, want the company's, under the invited address", acc)
	}
	if holder, err := f.repo.GetByUserID(ctx, f.holder); err != nil || holder.Member || holder.WorkEmail != "hr@acme.test" {
		t.Errorf("holder's account = %+v, %v", holder, err)
	}

	members, err := f.repo.Members(ctx, "acme")
	if err != nil || len(members) != 2 || !members[0].Holder || members[1].UserID != member {
		t.Errorf("Members = %+v, %v", members, err)
	}

	// Vacancies are the team's: the holder sees and edits what the member published.
	f.job(t, "member-vacancy", member)
	jobs, _, err := f.repo.ListMine(ctx, f.holder)
	if err != nil || len(jobs) != 1 {
		t.Fatalf("holder's ListMine = %d, %v; want the member's vacancy", len(jobs), err)
	}
	if _, _, err := f.repo.BySlug(ctx, f.holder, "member-vacancy"); err != nil {
		t.Errorf("holder's BySlug of the member's vacancy: %v", err)
	}
	stranger := seedUser(t, f.pool, "x@globex.test")
	if _, _, err := f.repo.BySlug(ctx, stranger, "member-vacancy"); !errors.Is(err, employer.ErrJobNotFound) {
		t.Errorf("a stranger's BySlug err = %v, want ErrJobNotFound", err)
	}

	// One company per user: a member cannot claim another.
	if _, err := f.repo.InsertPending(ctx, member, "globex", "Globex", "dev@globex.test"); !errors.Is(err, employer.ErrAlreadyHasAccount) {
		t.Errorf("a member's claim err = %v, want ErrAlreadyHasAccount", err)
	}

	// Removing the member hands their vacancy to the holder.
	if err := f.repo.Remove(ctx, "acme", f.holder, member); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	var createdBy int64
	if err := f.pool.QueryRow(ctx, `SELECT created_by FROM jobs WHERE public_slug = 'member-vacancy'`).Scan(&createdBy); err != nil || createdBy != f.holder {
		t.Errorf("vacancy created_by after removal = %d, %v; want the holder", createdBy, err)
	}
	if _, err := f.repo.GetByUserID(ctx, member); !errors.Is(err, employer.ErrNotFound) {
		t.Errorf("removed member's account err = %v, want ErrNotFound", err)
	}
	if err := f.repo.Remove(ctx, "acme", f.holder, f.holder); !errors.Is(err, employer.ErrMemberNotFound) {
		t.Errorf("removing the holder err = %v, want ErrMemberNotFound", err)
	}
}
