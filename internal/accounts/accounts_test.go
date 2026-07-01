package accounts

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// fakeRepo is a test double for Repository.
// It records calls and returns canned responses.
type fakeRepo struct {
	// UserIDByIdentity responses: looked up in order per call
	identityResults []idResult
	identityCallIdx int

	// LinkOrCreateByEmail responses: looked up in order per call
	linkResults []linkResult
	linkCallIdx int

	// recorded calls
	linkCalls []linkCall

	// CreateUser responses and calls
	createUserResults []createUserResult
	createUserCallIdx int
	createUserCalls   []createUserCall

	// UserByEmail responses
	userByEmailResults []userByEmailResult
	userByEmailCallIdx int

	// UserByID responses
	userByIDResults []userByIDResult
	userByIDCallIdx int
}

type idResult struct {
	id  int64
	err error
}

type linkResult struct {
	id  int64
	err error
}

type linkCall struct {
	provider       string
	providerUserID string
	email          string
}

type createUserResult struct {
	user User
	err  error
}

type createUserCall struct {
	email        string
	passwordHash string
}

type userByEmailResult struct {
	user         User
	passwordHash string
	hasPassword  bool
	err          error
}

type userByIDResult struct {
	user User
	err  error
}

func (f *fakeRepo) UserIDByIdentity(_ context.Context, provider, providerUserID string) (int64, error) {
	if f.identityCallIdx >= len(f.identityResults) {
		return 0, ErrIdentityNotFound
	}
	r := f.identityResults[f.identityCallIdx]
	f.identityCallIdx++
	return r.id, r.err
}

func (f *fakeRepo) LinkOrCreateByEmail(_ context.Context, provider, providerUserID, email string) (int64, error) {
	f.linkCalls = append(f.linkCalls, linkCall{provider, providerUserID, email})
	if f.linkCallIdx >= len(f.linkResults) {
		return 0, errors.New("fakeRepo: unexpected LinkOrCreateByEmail call")
	}
	r := f.linkResults[f.linkCallIdx]
	f.linkCallIdx++
	return r.id, r.err
}

func (f *fakeRepo) CreateUser(_ context.Context, email, passwordHash string) (User, error) {
	f.createUserCalls = append(f.createUserCalls, createUserCall{email, passwordHash})
	if f.createUserCallIdx >= len(f.createUserResults) {
		return User{}, errors.New("fakeRepo: unexpected CreateUser call")
	}
	r := f.createUserResults[f.createUserCallIdx]
	f.createUserCallIdx++
	return r.user, r.err
}

func (f *fakeRepo) UserByEmail(_ context.Context, email string) (User, string, bool, error) {
	if f.userByEmailCallIdx >= len(f.userByEmailResults) {
		return User{}, "", false, ErrUserNotFound
	}
	r := f.userByEmailResults[f.userByEmailCallIdx]
	f.userByEmailCallIdx++
	return r.user, r.passwordHash, r.hasPassword, r.err
}

func (f *fakeRepo) UserByID(_ context.Context, id int64) (User, error) {
	if f.userByIDCallIdx >= len(f.userByIDResults) {
		return User{}, ErrUserNotFound
	}
	r := f.userByIDResults[f.userByIDCallIdx]
	f.userByIDCallIdx++
	return r.user, r.err
}

// fakeHasher is a test double for PasswordHasher.
// Hash returns "hashed:"+plain; Check returns nil iff hash == "hashed:"+plain.
type fakeHasher struct {
	hashCalls  int
	checkCalls int
}

func (h *fakeHasher) Hash(plain string) (string, error) {
	h.hashCalls++
	return "hashed:" + plain, nil
}

func (h *fakeHasher) Check(hash, plain string) error {
	h.checkCalls++
	if hash == "hashed:"+plain {
		return nil
	}
	return errors.New("fakeHasher: password mismatch")
}

// ---------------------------------------------------------------------------
// OAuth tests (existing, updated to New(repo, hasher) signature)
// ---------------------------------------------------------------------------

// (a) identity found → returns that id; LinkOrCreateByEmail NOT called.
func TestResolveOAuthAccount_IdentityFound(t *testing.T) {
	repo := &fakeRepo{
		identityResults: []idResult{{id: 7, err: nil}},
	}
	svc := New(repo, &fakeHasher{})

	id, err := svc.ResolveOAuthAccount(context.Background(), "google", "gid-1", "user@example.com", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 7 {
		t.Errorf("want id=7, got %d", id)
	}
	if len(repo.linkCalls) != 0 {
		t.Errorf("LinkOrCreateByEmail should NOT be called, got %d calls", len(repo.linkCalls))
	}
}

// (b) not found + emailVerified=true + non-empty email → LinkOrCreateByEmail called
// with the LOWER/trimmed email, returns its id.
func TestResolveOAuthAccount_LinkOrCreate(t *testing.T) {
	repo := &fakeRepo{
		identityResults: []idResult{{id: 0, err: ErrIdentityNotFound}},
		linkResults:     []linkResult{{id: 99, err: nil}},
	}
	svc := New(repo, &fakeHasher{})

	id, err := svc.ResolveOAuthAccount(context.Background(), "github", "ghid-2", "  USER@Example.COM  ", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 99 {
		t.Errorf("want id=99, got %d", id)
	}
	if len(repo.linkCalls) != 1 {
		t.Fatalf("want 1 LinkOrCreateByEmail call, got %d", len(repo.linkCalls))
	}
	got := repo.linkCalls[0]
	if got.email != "user@example.com" {
		t.Errorf("email not normalized: want %q, got %q", "user@example.com", got.email)
	}
	if got.provider != "github" || got.providerUserID != "ghid-2" {
		t.Errorf("provider args wrong: %+v", got)
	}
}

// (c) not found + emailVerified=false → ErrNoVerifiedEmail; LinkOrCreateByEmail NOT called.
func TestResolveOAuthAccount_UnverifiedEmail(t *testing.T) {
	repo := &fakeRepo{
		identityResults: []idResult{{id: 0, err: ErrIdentityNotFound}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.ResolveOAuthAccount(context.Background(), "google", "gid-3", "user@example.com", false)
	if !errors.Is(err, ErrNoVerifiedEmail) {
		t.Errorf("want ErrNoVerifiedEmail, got %v", err)
	}
	if len(repo.linkCalls) != 0 {
		t.Errorf("LinkOrCreateByEmail should NOT be called, got %d calls", len(repo.linkCalls))
	}
}

// (d) not found + empty/whitespace email (even if emailVerified=true) → ErrNoVerifiedEmail; not called.
func TestResolveOAuthAccount_EmptyEmail(t *testing.T) {
	for _, email := range []string{"", "   ", "\t"} {
		repo := &fakeRepo{
			identityResults: []idResult{{id: 0, err: ErrIdentityNotFound}},
		}
		svc := New(repo, &fakeHasher{})

		_, err := svc.ResolveOAuthAccount(context.Background(), "google", "gid-4", email, true)
		if !errors.Is(err, ErrNoVerifiedEmail) {
			t.Errorf("email=%q: want ErrNoVerifiedEmail, got %v", email, err)
		}
		if len(repo.linkCalls) != 0 {
			t.Errorf("email=%q: LinkOrCreateByEmail should NOT be called", email)
		}
	}
}

// (e) race: LinkOrCreateByEmail returns ErrIdentityConflict,
// follow-up UserIDByIdentity returns id 42 → service returns 42.
func TestResolveOAuthAccount_Race_ReturnsWinner(t *testing.T) {
	repo := &fakeRepo{
		identityResults: []idResult{
			{id: 0, err: ErrIdentityNotFound}, // first lookup misses
			{id: 42, err: nil},                // retry after conflict finds the winner
		},
		linkResults: []linkResult{
			{id: 0, err: ErrIdentityConflict},
		},
	}
	svc := New(repo, &fakeHasher{})

	id, err := svc.ResolveOAuthAccount(context.Background(), "google", "gid-5", "winner@example.com", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 42 {
		t.Errorf("want id=42, got %d", id)
	}
}

// (f) race retry still ErrIdentityNotFound → service returns a non-nil error.
func TestResolveOAuthAccount_Race_RetryFails(t *testing.T) {
	repo := &fakeRepo{
		identityResults: []idResult{
			{id: 0, err: ErrIdentityNotFound}, // first lookup misses
			{id: 0, err: ErrIdentityNotFound}, // retry also misses
		},
		linkResults: []linkResult{
			{id: 0, err: ErrIdentityConflict},
		},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.ResolveOAuthAccount(context.Background(), "google", "gid-6", "ghost@example.com", true)
	if err == nil {
		t.Fatal("want non-nil error when race retry also returns ErrIdentityNotFound")
	}
}

// (g) email race: a concurrent callback for the SAME verified email under a
// DIFFERENT identity created the account first, so our LinkOrCreateByEmail lost on
// the user email index (ErrEmailRace) and never inserted our identity. The service
// retries the link — which now finds the account by email and attaches our identity
// — instead of failing the callback with ?auth_error=oauth.
func TestResolveOAuthAccount_EmailRace_LinksToWinner(t *testing.T) {
	repo := &fakeRepo{
		identityResults: []idResult{
			{id: 0, err: ErrIdentityNotFound}, // first identity lookup misses
		},
		linkResults: []linkResult{
			{id: 0, err: ErrEmailRace}, // lost the email-create race
			{id: 7, err: nil},          // retry links our identity to the winner's account
		},
	}
	svc := New(repo, &fakeHasher{})

	id, err := svc.ResolveOAuthAccount(context.Background(), "github", "ghid-9", "shared@example.com", true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != 7 {
		t.Errorf("want id=7 (identity linked to the winner's account), got %d", id)
	}
	if len(repo.linkCalls) != 2 {
		t.Errorf("want 2 LinkOrCreateByEmail calls (initial + retry), got %d", len(repo.linkCalls))
	}
}

// ---------------------------------------------------------------------------
// Register tests
// ---------------------------------------------------------------------------

// Register invalid email → ErrInvalidEmail; CreateUser NOT called.
func TestRegister_InvalidEmail(t *testing.T) {
	repo := &fakeRepo{}
	hasher := &fakeHasher{}
	svc := New(repo, hasher)

	_, err := svc.Register(context.Background(), "not-an-email", "password123")
	if !errors.Is(err, ErrInvalidEmail) {
		t.Errorf("want ErrInvalidEmail, got %v", err)
	}
	if len(repo.createUserCalls) != 0 {
		t.Errorf("CreateUser should NOT be called, got %d calls", len(repo.createUserCalls))
	}
}

// Register short password → ErrPasswordTooShort; CreateUser NOT called.
func TestRegister_ShortPassword(t *testing.T) {
	repo := &fakeRepo{}
	hasher := &fakeHasher{}
	svc := New(repo, hasher)

	_, err := svc.Register(context.Background(), "user@example.com", "short")
	if !errors.Is(err, ErrPasswordTooShort) {
		t.Errorf("want ErrPasswordTooShort, got %v", err)
	}
	if len(repo.createUserCalls) != 0 {
		t.Errorf("CreateUser should NOT be called, got %d calls", len(repo.createUserCalls))
	}
}

// Register long password → ErrPasswordTooLong; CreateUser NOT called. bcrypt silently
// truncates past 72 bytes, so accepting a longer password would ignore its tail.
func TestRegister_LongPassword(t *testing.T) {
	repo := &fakeRepo{}
	svc := New(repo, &fakeHasher{})

	long := strings.Repeat("a", 73)
	_, err := svc.Register(context.Background(), "user@example.com", long)
	if !errors.Is(err, ErrPasswordTooLong) {
		t.Errorf("want ErrPasswordTooLong, got %v", err)
	}
	if len(repo.createUserCalls) != 0 {
		t.Errorf("CreateUser should NOT be called, got %d calls", len(repo.createUserCalls))
	}
}

// Register happy path → hashes then calls CreateUser with the hash, returns user.
func TestRegister_Happy(t *testing.T) {
	wantUser := User{ID: 1, Email: "user@example.com"}
	repo := &fakeRepo{
		createUserResults: []createUserResult{{user: wantUser, err: nil}},
	}
	hasher := &fakeHasher{}
	svc := New(repo, hasher)

	got, err := svc.Register(context.Background(), "USER@Example.COM", "password123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUser {
		t.Errorf("want %+v, got %+v", wantUser, got)
	}
	if len(repo.createUserCalls) != 1 {
		t.Fatalf("want 1 CreateUser call, got %d", len(repo.createUserCalls))
	}
	call := repo.createUserCalls[0]
	if call.email != "user@example.com" {
		t.Errorf("email not normalized: want %q, got %q", "user@example.com", call.email)
	}
	if call.passwordHash != "hashed:password123" {
		t.Errorf("password not hashed: want %q, got %q", "hashed:password123", call.passwordHash)
	}
}

// Register when CreateUser returns ErrEmailTaken → propagated.
func TestRegister_EmailTaken(t *testing.T) {
	repo := &fakeRepo{
		createUserResults: []createUserResult{{user: User{}, err: ErrEmailTaken}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.Register(context.Background(), "user@example.com", "password123")
	if !errors.Is(err, ErrEmailTaken) {
		t.Errorf("want ErrEmailTaken, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// Login tests
// ---------------------------------------------------------------------------

// Login when UserByEmail returns ErrUserNotFound → ErrInvalidCredentials.
func TestLogin_UserNotFound(t *testing.T) {
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{err: ErrUserNotFound}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.Login(context.Background(), "user@example.com", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

// Login when hasPassword=false → ErrInvalidCredentials; Check NOT called.
func TestLogin_NoPassword(t *testing.T) {
	wantUser := User{ID: 5, Email: "user@example.com"}
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{user: wantUser, hasPassword: false}},
	}
	hasher := &fakeHasher{}
	svc := New(repo, hasher)

	_, err := svc.Login(context.Background(), "user@example.com", "password123")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
	if hasher.checkCalls != 0 {
		t.Errorf("Check should NOT be called when hasPassword=false, got %d calls", hasher.checkCalls)
	}
}

// Login when Check fails → ErrInvalidCredentials.
func TestLogin_WrongPassword(t *testing.T) {
	wantUser := User{ID: 5, Email: "user@example.com"}
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{user: wantUser, passwordHash: "hashed:correct", hasPassword: true}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.Login(context.Background(), "user@example.com", "wrongpassword")
	if !errors.Is(err, ErrInvalidCredentials) {
		t.Errorf("want ErrInvalidCredentials, got %v", err)
	}
}

// Login success → returns user.
func TestLogin_Happy(t *testing.T) {
	wantUser := User{ID: 5, Email: "user@example.com"}
	repo := &fakeRepo{
		userByEmailResults: []userByEmailResult{{user: wantUser, passwordHash: "hashed:correct", hasPassword: true}},
	}
	svc := New(repo, &fakeHasher{})

	got, err := svc.Login(context.Background(), "USER@Example.COM", "correct")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUser {
		t.Errorf("want %+v, got %+v", wantUser, got)
	}
}

// ---------------------------------------------------------------------------
// UserByID tests
// ---------------------------------------------------------------------------

// UserByID found → returns user.
func TestUserByID_Found(t *testing.T) {
	wantUser := User{ID: 10, Email: "someone@example.com"}
	repo := &fakeRepo{
		userByIDResults: []userByIDResult{{user: wantUser}},
	}
	svc := New(repo, &fakeHasher{})

	got, err := svc.UserByID(context.Background(), 10)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != wantUser {
		t.Errorf("want %+v, got %+v", wantUser, got)
	}
}

// UserByID when repo returns ErrUserNotFound → propagated.
func TestUserByID_NotFound(t *testing.T) {
	repo := &fakeRepo{
		userByIDResults: []userByIDResult{{err: ErrUserNotFound}},
	}
	svc := New(repo, &fakeHasher{})

	_, err := svc.UserByID(context.Background(), 99)
	if !errors.Is(err, ErrUserNotFound) {
		t.Errorf("want ErrUserNotFound, got %v", err)
	}
}
