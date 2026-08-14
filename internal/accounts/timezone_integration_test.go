//go:build integration

// Integration test for task 6.4 of notification-frequency-quiet-hours:
// Register's captured timezone and UpdateTimezone's edit both round-trip
// through a real Postgres. Run with: go test -tags=integration ./internal/accounts/
package accounts_test

import (
	"context"
	"testing"

	"github.com/strelov1/freehire/internal/accounts"
)

// plainHasher is a trivial PasswordHasher — these tests exercise timezone
// persistence, not password hashing, so a real bcrypt cost is unnecessary work.
type plainHasher struct{}

func (plainHasher) Hash(plain string) (string, error) { return "hashed:" + plain, nil }
func (plainHasher) Check(hash, plain string) error {
	if hash == "hashed:"+plain {
		return nil
	}
	return context.DeadlineExceeded // any non-nil error; never asserted on in these tests
}

func TestRegister_TimezoneRoundTrips(t *testing.T) {
	pool := startPostgres(t)
	repo := oauthRepo(pool)
	svc := accounts.New(repo, plainHasher{})

	tz := "Europe/Moscow"
	user, err := svc.Register(context.Background(), "tz-register@example.test", "password123", &tz)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Timezone == nil || *user.Timezone != tz {
		t.Errorf("Timezone = %v, want %q", user.Timezone, tz)
	}

	got, err := svc.UserByID(context.Background(), user.ID)
	if err != nil {
		t.Fatalf("UserByID: %v", err)
	}
	if got.Timezone == nil || *got.Timezone != tz {
		t.Errorf("re-read Timezone = %v, want %q", got.Timezone, tz)
	}
}

func TestUpdateTimezone_RoundTrips(t *testing.T) {
	pool := startPostgres(t)
	repo := oauthRepo(pool)
	svc := accounts.New(repo, plainHasher{})

	user, err := svc.Register(context.Background(), "tz-update@example.test", "password123", nil)
	if err != nil {
		t.Fatalf("Register: %v", err)
	}
	if user.Timezone != nil {
		t.Fatalf("Timezone = %v, want nil before any update", user.Timezone)
	}

	updated, err := svc.UpdateTimezone(context.Background(), user.ID, "America/New_York")
	if err != nil {
		t.Fatalf("UpdateTimezone: %v", err)
	}
	if updated.Timezone == nil || *updated.Timezone != "America/New_York" {
		t.Errorf("Timezone = %v, want %q", updated.Timezone, "America/New_York")
	}
}
