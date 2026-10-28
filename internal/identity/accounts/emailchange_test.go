package accounts

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func emailChangeService(user User) (*Service, *fakeRepo, *fakeCodes, *fakeMailer) {
	repo := &fakeRepo{
		userByIDResults:    []userByIDResult{{user: user}, {user: user}},
		setPasswordVersion: 7,
	}
	codes, mailer := newFakeCodes(), &fakeMailer{}
	return verifyService(repo, codes, mailer, time.Now()), repo, codes, mailer
}

// The code is mailed to the NEW address and remembers it; confirming applies exactly that
// address and tells the old one.
func TestEmailChangeAppliesTheAddressTheCodeWasMailedTo(t *testing.T) {
	s, repo, _, mailer := emailChangeService(User{ID: 3, Email: "old@example.test", EmailVerified: true})
	ctx := context.Background()

	if err := s.RequestEmailChange(ctx, 3, " New@Example.test "); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	if len(mailer.change) != 1 || !strings.HasPrefix(mailer.change[0], "new@example.test:") {
		t.Fatalf("change mails = %v, want one to new@example.test", mailer.change)
	}
	code := strings.TrimPrefix(mailer.change[0], "new@example.test:")

	version, err := s.ConfirmEmailChange(ctx, 3, code)
	if err != nil {
		t.Fatalf("ConfirmEmailChange: %v", err)
	}
	if version != 7 || repo.changedEmail != "new@example.test" {
		t.Errorf("changed to %q at version %d, want new@example.test at 7", repo.changedEmail, version)
	}
	if len(mailer.changedNotice) != 1 || mailer.changedNotice[0] != "old@example.test" {
		t.Errorf("notices = %v, want one to old@example.test", mailer.changedNotice)
	}
}

// An address that was never proven may belong to somebody who never had this account, so
// it is not told where the account went — or that it existed.
func TestEmailChangeDoesNotNotifyAnUnverifiedOldAddress(t *testing.T) {
	s, _, _, mailer := emailChangeService(User{ID: 3, Email: "typo@example.test"})
	ctx := context.Background()

	if err := s.RequestEmailChange(ctx, 3, "right@example.test"); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	code := strings.TrimPrefix(mailer.change[0], "right@example.test:")
	if _, err := s.ConfirmEmailChange(ctx, 3, code); err != nil {
		t.Fatalf("ConfirmEmailChange: %v", err)
	}
	if len(mailer.changedNotice) != 0 {
		t.Errorf("notified %v, want nobody", mailer.changedNotice)
	}
}

func TestEmailChangeRefusals(t *testing.T) {
	ctx := context.Background()

	s, _, _, _ := emailChangeService(User{ID: 3, Email: "old@example.test"})
	if err := s.RequestEmailChange(ctx, 3, "OLD@example.test"); !errors.Is(err, ErrSameEmail) {
		t.Errorf("same address = %v, want ErrSameEmail", err)
	}

	s, repo, _, mailer := emailChangeService(User{ID: 3, Email: "old@example.test"})
	repo.userByEmailResults = []userByEmailResult{{user: User{ID: 9, Email: "taken@example.test"}}}
	if err := s.RequestEmailChange(ctx, 3, "taken@example.test"); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("taken address = %v, want ErrEmailTaken", err)
	}
	if len(mailer.change) != 0 {
		t.Errorf("a refused request mailed %v", mailer.change)
	}

	// The address was free when the code went out and is claimed by the time it returns.
	s, repo, _, mailer = emailChangeService(User{ID: 3, Email: "old@example.test", EmailVerified: true})
	if err := s.RequestEmailChange(ctx, 3, "raced@example.test"); err != nil {
		t.Fatalf("RequestEmailChange: %v", err)
	}
	repo.changeEmailErr = ErrEmailTaken
	code := strings.TrimPrefix(mailer.change[0], "raced@example.test:")
	if _, err := s.ConfirmEmailChange(ctx, 3, code); !errors.Is(err, ErrEmailTaken) {
		t.Errorf("raced confirm = %v, want ErrEmailTaken", err)
	}
	if len(mailer.changedNotice) != 0 {
		t.Errorf("a failed move notified %v", mailer.changedNotice)
	}
}
