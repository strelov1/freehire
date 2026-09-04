package accounts

import (
	"context"
	"errors"
	"testing"
	"time"
)

// --- fakeRepo username methods ---

func (f *fakeRepo) UsernameByUser(_ context.Context, userID int64) (string, *time.Time, bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.usernameByUserCalls++
	if f.usernameByUserErrOnRecheck != nil && f.usernameByUserCalls > 1 {
		return "", nil, false, f.usernameByUserErrOnRecheck
	}
	row, ok := f.usernameByUser[userID]
	if !ok {
		return "", nil, false, nil
	}
	return row.name, row.updatedAt, true, nil
}

func (f *fakeRepo) SetUsernameIfAbsent(_ context.Context, userID int64, uname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.usernameByUser == nil {
		f.usernameByUser = map[int64]fakeUsernameRow{}
	}
	if f.takenUsernames == nil {
		f.takenUsernames = map[string]int64{}
	}
	if _, ok := f.usernameByUser[userID]; ok {
		// Already has one — the caller resolves this by re-reading.
		return ErrUsernameTaken
	}
	if owner, taken := f.takenUsernames[uname]; taken && owner != userID {
		return ErrUsernameTaken
	}
	f.usernameByUser[userID] = fakeUsernameRow{name: uname}
	f.takenUsernames[uname] = userID
	return nil
}

func (f *fakeRepo) UsernameTaken(_ context.Context, uname string) (bool, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	_, taken := f.takenUsernames[uname]
	return taken, nil
}

func (f *fakeRepo) SetUsername(_ context.Context, userID int64, uname string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.usernameByUser == nil {
		f.usernameByUser = map[int64]fakeUsernameRow{}
	}
	if f.takenUsernames == nil {
		f.takenUsernames = map[string]int64{}
	}
	if owner, taken := f.takenUsernames[uname]; taken && owner != userID {
		return ErrUsernameTaken
	}
	if old, ok := f.usernameByUser[userID]; ok {
		delete(f.takenUsernames, old.name)
	}
	now := time.Now()
	f.usernameByUser[userID] = fakeUsernameRow{name: uname, updatedAt: &now}
	f.takenUsernames[uname] = userID
	return nil
}

// --- tests ---

func TestEnsureUsernameAllocatesADefaultOnFirstNeed(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	got, err := s.EnsureUsername(context.Background(), 1, "ivan@example.test")
	if err != nil {
		t.Fatalf("EnsureUsername: %v", err)
	}
	if got != "ivan" {
		t.Errorf("EnsureUsername = %q, want %q", got, "ivan")
	}
}

func TestEnsureUsernameIsIdempotent(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})
	ctx := context.Background()

	first, err := s.EnsureUsername(ctx, 1, "ivan@example.test")
	if err != nil {
		t.Fatalf("EnsureUsername (first): %v", err)
	}
	second, err := s.EnsureUsername(ctx, 1, "ivan@example.test")
	if err != nil {
		t.Fatalf("EnsureUsername (second): %v", err)
	}
	if first != second {
		t.Errorf("EnsureUsername reallocated: first=%q second=%q", first, second)
	}
}

func TestEnsureUsernameDoesNotStartTheCooldownClock(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	if _, err := s.EnsureUsername(context.Background(), 1, "ivan@example.test"); err != nil {
		t.Fatalf("EnsureUsername: %v", err)
	}
	_, updatedAt, ok, err := repo.UsernameByUser(context.Background(), 1)
	if err != nil || !ok {
		t.Fatalf("UsernameByUser: ok=%v err=%v", ok, err)
	}
	if updatedAt != nil {
		t.Errorf("EnsureUsername set username_updated_at = %v, want nil (lazy default must not start the cooldown)", updatedAt)
	}
}

func TestEnsureUsernameResolvesCollisionWithASuffix(t *testing.T) {
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{2: {name: "ivan"}},
		takenUsernames: map[string]int64{"ivan": 2},
	}
	s := New(repo, &fakeHasher{})

	got, err := s.EnsureUsername(context.Background(), 1, "ivan@example.test")
	if err != nil {
		t.Fatalf("EnsureUsername: %v", err)
	}
	if got != "ivan-2" {
		t.Errorf("EnsureUsername = %q, want %q", got, "ivan-2")
	}
}

func TestEnsureUsernamePropagatesARecheckError(t *testing.T) {
	wantErr := errors.New("boom")
	repo := &fakeRepo{
		usernameByUser:             map[int64]fakeUsernameRow{2: {name: "ivan"}},
		takenUsernames:             map[string]int64{"ivan": 2},
		usernameByUserErrOnRecheck: wantErr,
	}
	s := New(repo, &fakeHasher{})

	// user 1's suggested base ("ivan") collides with user 2's, forcing the
	// post-collision re-read — which is rigged to fail here. That failure must
	// propagate, not be swallowed into another suffix attempt.
	_, err := s.EnsureUsername(context.Background(), 1, "ivan@example.test")
	if !errors.Is(err, wantErr) {
		t.Fatalf("EnsureUsername error = %v, want %v", err, wantErr)
	}
}

func TestEnsureUsernameSkipsAReservedBaseForm(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	// "admin" is reserved, but a suffixed form is not.
	got, err := s.EnsureUsername(context.Background(), 1, "admin@example.test")
	if err != nil {
		t.Fatalf("EnsureUsername: %v", err)
	}
	if got == "admin" {
		t.Errorf("EnsureUsername allocated the reserved bare form %q", got)
	}
}

func TestEnsureUsernameFromBaseAllocatesTheGivenBase(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	got, err := s.EnsureUsernameFromBase(context.Background(), 1, "legacy-name")
	if err != nil {
		t.Fatalf("EnsureUsernameFromBase: %v", err)
	}
	if got != "legacy-name" {
		t.Errorf("EnsureUsernameFromBase = %q, want %q", got, "legacy-name")
	}
}

func TestEnsureUsernameFromBaseResolvesCollisionWithASuffix(t *testing.T) {
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{2: {name: "legacy-name"}},
		takenUsernames: map[string]int64{"legacy-name": 2},
	}
	s := New(repo, &fakeHasher{})

	got, err := s.EnsureUsernameFromBase(context.Background(), 1, "legacy-name")
	if err != nil {
		t.Fatalf("EnsureUsernameFromBase: %v", err)
	}
	if got != "legacy-name-2" {
		t.Errorf("EnsureUsernameFromBase = %q, want %q", got, "legacy-name-2")
	}
}

func TestEnsureUsernameFromBaseSanitizesAnUnsanitizedBase(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	// A raw legacy mailbox local-part, e.g. from an email containing a dot, is not
	// itself a valid username — EnsureUsernameFromBase must sanitize it, not just
	// pass it straight to Candidate (a caller must not have to pre-sanitize).
	got, err := s.EnsureUsernameFromBase(context.Background(), 1, "legacy.name")
	if err != nil {
		t.Fatalf("EnsureUsernameFromBase: %v", err)
	}
	if got != "legacy-name" {
		t.Errorf("EnsureUsernameFromBase(%q) = %q, want %q", "legacy.name", got, "legacy-name")
	}
}

func TestEnsureUsernameFromBaseIsIdempotent(t *testing.T) {
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{1: {name: "already-set"}},
		takenUsernames: map[string]int64{"already-set": 1},
	}
	s := New(repo, &fakeHasher{})

	got, err := s.EnsureUsernameFromBase(context.Background(), 1, "some-other-base")
	if err != nil {
		t.Fatalf("EnsureUsernameFromBase: %v", err)
	}
	if got != "already-set" {
		t.Errorf("EnsureUsernameFromBase = %q, want existing %q", got, "already-set")
	}
}

func TestClaimUsernameSucceedsOnFirstClaim(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	if err := s.ClaimUsername(context.Background(), 1, "ivan-petrov"); err != nil {
		t.Fatalf("ClaimUsername: %v", err)
	}
	name, updatedAt, ok, err := repo.UsernameByUser(context.Background(), 1)
	if err != nil || !ok || name != "ivan-petrov" {
		t.Fatalf("UsernameByUser = %q, ok=%v, err=%v", name, ok, err)
	}
	if updatedAt == nil {
		t.Error("ClaimUsername left username_updated_at nil, want it set")
	}
}

func TestClaimUsernameRejectsATakenNameWithoutSuffixing(t *testing.T) {
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{2: {name: "ivan"}},
		takenUsernames: map[string]int64{"ivan": 2},
	}
	s := New(repo, &fakeHasher{})

	err := s.ClaimUsername(context.Background(), 1, "ivan")
	if !errors.Is(err, ErrUsernameTaken) {
		t.Fatalf("ClaimUsername error = %v, want ErrUsernameTaken", err)
	}
	if _, _, ok, _ := repo.UsernameByUser(context.Background(), 1); ok {
		t.Error("ClaimUsername allocated a suffixed variant instead of rejecting outright")
	}
}

func TestClaimUsernameRejectsAReservedName(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	err := s.ClaimUsername(context.Background(), 1, "admin")
	if !errors.Is(err, ErrUsernameReserved) {
		t.Fatalf("ClaimUsername error = %v, want ErrUsernameReserved", err)
	}
}

func TestClaimUsernameRejectsAnInvalidFormat(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	err := s.ClaimUsername(context.Background(), 1, "AB")
	if !errors.Is(err, ErrUsernameInvalid) {
		t.Fatalf("ClaimUsername error = %v, want ErrUsernameInvalid", err)
	}
}

func TestClaimUsernameRejectsWithinTheCooldown(t *testing.T) {
	tenDaysAgo := time.Now().Add(-10 * 24 * time.Hour)
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{1: {name: "old-name", updatedAt: &tenDaysAgo}},
		takenUsernames: map[string]int64{"old-name": 1},
	}
	s := New(repo, &fakeHasher{})

	err := s.ClaimUsername(context.Background(), 1, "new-name")
	if !errors.Is(err, ErrUsernameCooldown) {
		t.Fatalf("ClaimUsername error = %v, want ErrUsernameCooldown", err)
	}
}

func TestClaimUsernameSucceedsAfterTheCooldownElapses(t *testing.T) {
	fortyDaysAgo := time.Now().Add(-40 * 24 * time.Hour)
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{1: {name: "old-name", updatedAt: &fortyDaysAgo}},
		takenUsernames: map[string]int64{"old-name": 1},
	}
	s := New(repo, &fakeHasher{})

	if err := s.ClaimUsername(context.Background(), 1, "new-name"); err != nil {
		t.Fatalf("ClaimUsername: %v", err)
	}
}

func TestClaimUsernameFirstExplicitClaimIsNeverRateLimitedAfterALazyDefault(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})
	ctx := context.Background()

	// A lazy default was allocated moments ago (e.g. the user just opened their inbox).
	if _, err := s.EnsureUsername(ctx, 1, "ivan@example.test"); err != nil {
		t.Fatalf("EnsureUsername: %v", err)
	}

	// The user's first EXPLICIT claim must succeed regardless of how recently the
	// lazy default was set, since username_updated_at is still nil.
	if err := s.ClaimUsername(ctx, 1, "ivan-petrov"); err != nil {
		t.Fatalf("ClaimUsername after a lazy default: %v", err)
	}
}

func TestUsernameReadsBackWhatWasClaimed(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})
	ctx := context.Background()

	if err := s.ClaimUsername(ctx, 1, "ivan-petrov"); err != nil {
		t.Fatalf("ClaimUsername: %v", err)
	}
	name, updatedAt, ok, err := s.Username(ctx, 1)
	if err != nil {
		t.Fatalf("Username: %v", err)
	}
	if !ok || name != "ivan-petrov" {
		t.Errorf("Username = %q, ok=%v, want %q, true", name, ok, "ivan-petrov")
	}
	if updatedAt == nil {
		t.Error("Username updatedAt = nil, want set")
	}
}

func TestUsernameReportsNoneForAnAccountThatHasNotClaimed(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	_, _, ok, err := s.Username(context.Background(), 1)
	if err != nil {
		t.Fatalf("Username: %v", err)
	}
	if ok {
		t.Error("Username ok = true for an account with none claimed")
	}
}

func TestUsernameAvailableForAFreeValidName(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	available, err := s.UsernameAvailable(context.Background(), "ivan-petrov")
	if err != nil {
		t.Fatalf("UsernameAvailable: %v", err)
	}
	if !available {
		t.Error("UsernameAvailable = false, want true for a free, valid name")
	}
}

func TestUsernameAvailableIsFalseForATakenName(t *testing.T) {
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{2: {name: "ivan"}},
		takenUsernames: map[string]int64{"ivan": 2},
	}
	s := New(repo, &fakeHasher{})

	available, err := s.UsernameAvailable(context.Background(), "ivan")
	if err != nil {
		t.Fatalf("UsernameAvailable: %v", err)
	}
	if available {
		t.Error("UsernameAvailable = true for a name another account already holds")
	}
}

func TestUsernameAvailableIsFalseForAReservedName(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	available, err := s.UsernameAvailable(context.Background(), "admin")
	if err != nil {
		t.Fatalf("UsernameAvailable: %v", err)
	}
	if available {
		t.Error("UsernameAvailable = true for a reserved name")
	}
}

func TestUsernameAvailableIsFalseForAnInvalidFormat(t *testing.T) {
	repo := &fakeRepo{}
	s := New(repo, &fakeHasher{})

	available, err := s.UsernameAvailable(context.Background(), "AB")
	if err != nil {
		t.Fatalf("UsernameAvailable: %v", err)
	}
	if available {
		t.Error("UsernameAvailable = true for an invalid format")
	}
}

func TestClaimUsernameFreesThePreviousNameForOthers(t *testing.T) {
	fortyDaysAgo := time.Now().Add(-40 * 24 * time.Hour)
	repo := &fakeRepo{
		usernameByUser: map[int64]fakeUsernameRow{1: {name: "ivan", updatedAt: &fortyDaysAgo}},
		takenUsernames: map[string]int64{"ivan": 1},
	}
	s := New(repo, &fakeHasher{})
	ctx := context.Background()

	if err := s.ClaimUsername(ctx, 1, "ivan-petrov"); err != nil {
		t.Fatalf("ClaimUsername (change away): %v", err)
	}
	// A different account can now claim the released name.
	if err := s.ClaimUsername(ctx, 2, "ivan"); err != nil {
		t.Fatalf("ClaimUsername (reclaim by another account): %v", err)
	}
}
