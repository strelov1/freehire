package discordlink

import (
	"context"
	"errors"
	"testing"
	"time"
)

// fakeStore records what the service asked of it. Deliberately dumb: the SQL semantics are
// covered against a real Postgres in internal/platform/db, so what is worth checking here is
// the ORDER the service does things in and what it does when a step fails.
type fakeStore struct {
	link        *Link
	proUntil    time.Time
	ultraUntil  time.Time
	candidates  []Candidate
	linkErr     error
	granted     []bool
	unlinkedN   int
	setGrantErr error
}

func (f *fakeStore) Link(_ context.Context, userID int64, discordUserID string) (Link, error) {
	if f.linkErr != nil {
		return Link{}, f.linkErr
	}
	l := Link{UserID: userID, DiscordUserID: discordUserID}
	f.link = &l
	return l, nil
}

func (f *fakeStore) Get(_ context.Context, userID int64) (Link, error) {
	if f.link == nil {
		return Link{}, ErrNotLinked
	}
	return *f.link, nil
}

func (f *fakeStore) Unlink(_ context.Context, userID int64) (bool, error) {
	if f.link == nil {
		return false, nil
	}
	f.link = nil
	f.unlinkedN++
	return true, nil
}

func (f *fakeStore) Plan(_ context.Context, userID int64) (time.Time, time.Time, error) {
	return f.proUntil, f.ultraUntil, nil
}

func (f *fakeStore) ListToSync(_ context.Context, limit int32) ([]Candidate, error) {
	if int(limit) < len(f.candidates) {
		return f.candidates[:limit], nil
	}
	return f.candidates, nil
}

func (f *fakeStore) SetRoleGranted(_ context.Context, userID int64, granted bool) error {
	if f.setGrantErr != nil {
		return f.setGrantErr
	}
	f.granted = append(f.granted, granted)
	return nil
}

// fakeDiscord stands in for the REST API.
type fakeDiscord struct {
	token     string
	userID    string
	calls     []string
	addErr    error
	grantErr  error
	revokeErr error
}

func (f *fakeDiscord) ExchangeCode(context.Context, string, string) (string, error) {
	f.calls = append(f.calls, "exchange")
	return f.token, nil
}

func (f *fakeDiscord) CurrentUserID(context.Context, string) (string, error) {
	f.calls = append(f.calls, "whoami")
	return f.userID, nil
}

func (f *fakeDiscord) AddGuildMember(context.Context, string, string) error {
	f.calls = append(f.calls, "join")
	return f.addErr
}

func (f *fakeDiscord) GrantPaidRole(context.Context, string) error {
	f.calls = append(f.calls, "grant")
	return f.grantErr
}

func (f *fakeDiscord) RevokePaidRole(context.Context, string) error {
	f.calls = append(f.calls, "revoke")
	return f.revokeErr
}

func newTestService(store *fakeStore, discord *fakeDiscord) *Service {
	return NewService(store, discord, func() time.Time { return time.Unix(1_800_000_000, 0) })
}

func aMonthOn() time.Time { return time.Unix(1_800_000_000, 0).Add(30 * 24 * time.Hour) }

func TestLinkPutsAPayingUserOnTheServerWithTheRole(t *testing.T) {
	store := &fakeStore{proUntil: aMonthOn()}
	discord := &fakeDiscord{token: "user-token", userID: "1000000000000000001"}
	svc := newTestService(store, discord)

	got, err := svc.Link(context.Background(), 7, "the-code", "https://freehire.me/cb")
	if err != nil {
		t.Fatalf("Link: %v", err)
	}
	if got.DiscordUserID != "1000000000000000001" {
		t.Errorf("discord id = %q", got.DiscordUserID)
	}
	// The order matters: the binding is stored BEFORE the role is granted, so a process
	// that dies mid-way leaves a link the reconciliation will finish rather than a role
	// nothing knows about.
	want := []string{"exchange", "whoami", "join", "grant"}
	if len(discord.calls) != len(want) {
		t.Fatalf("calls = %v, want %v", discord.calls, want)
	}
	for i := range want {
		if discord.calls[i] != want[i] {
			t.Fatalf("calls = %v, want %v", discord.calls, want)
		}
	}
	if len(store.granted) != 1 || !store.granted[0] {
		t.Errorf("grant record = %v, want [true]", store.granted)
	}
}

func TestLinkOfAFreeUserJoinsWithoutTheRole(t *testing.T) {
	store := &fakeStore{}
	discord := &fakeDiscord{token: "user-token", userID: "1"}
	svc := newTestService(store, discord)

	if _, err := svc.Link(context.Background(), 7, "c", "https://freehire.me/cb"); err != nil {
		t.Fatalf("Link: %v", err)
	}
	for _, c := range discord.calls {
		if c == "grant" {
			t.Fatal("a free account was given the paid role")
		}
	}
	if len(store.granted) != 0 {
		t.Errorf("grant record = %v, want nothing recorded", store.granted)
	}
}

// The store refuses a Discord account another freehire account already holds. The service
// must pass that through as a conflict, not retry it — retrying would either loop or, worse,
// succeed by taking the binding away from its owner.
func TestLinkReportsAConflictingBinding(t *testing.T) {
	store := &fakeStore{linkErr: ErrAlreadyLinkedElsewhere}
	discord := &fakeDiscord{token: "t", userID: "1"}
	svc := newTestService(store, discord)

	_, err := svc.Link(context.Background(), 7, "c", "https://freehire.me/cb")
	if !errors.Is(err, ErrAlreadyLinkedElsewhere) {
		t.Fatalf("error = %v, want ErrAlreadyLinkedElsewhere", err)
	}
	for _, c := range discord.calls {
		if c == "join" || c == "grant" {
			t.Fatalf("acted on Discord after a refused binding: %v", discord.calls)
		}
	}
}

func TestUnlinkRevokesAndDeletes(t *testing.T) {
	store := &fakeStore{link: &Link{UserID: 7, DiscordUserID: "1", RoleGranted: true}}
	discord := &fakeDiscord{}
	svc := newTestService(store, discord)

	if err := svc.Unlink(context.Background(), 7); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if len(discord.calls) != 1 || discord.calls[0] != "revoke" {
		t.Errorf("calls = %v, want one revoke", discord.calls)
	}
	if store.link != nil {
		t.Error("the binding survived the unlink")
	}
}

// A user must always be able to undo a link they made. An orphaned role is an operator's
// problem; a link that cannot be removed is the user's, every time they look at the page.
func TestUnlinkDeletesEvenWhenDiscordRefuses(t *testing.T) {
	store := &fakeStore{link: &Link{UserID: 7, DiscordUserID: "1", RoleGranted: true}}
	discord := &fakeDiscord{revokeErr: errors.New("discord is down")}
	svc := newTestService(store, discord)

	if err := svc.Unlink(context.Background(), 7); err != nil {
		t.Fatalf("Unlink: %v", err)
	}
	if store.link != nil {
		t.Error("the binding survived a failed revoke — the user is trapped in it")
	}
}

func TestUnlinkOfSomethingNotLinkedIsHarmless(t *testing.T) {
	store := &fakeStore{}
	discord := &fakeDiscord{}
	svc := newTestService(store, discord)

	if err := svc.Unlink(context.Background(), 7); err != nil {
		t.Fatalf("Unlink with no binding: %v", err)
	}
	if len(discord.calls) != 0 {
		t.Errorf("calls = %v, want none", discord.calls)
	}
}

func TestSyncGrantsRevokesAndRests(t *testing.T) {
	for _, tc := range []struct {
		name       string
		candidate  Candidate
		wantCall   string
		wantRecord []bool
	}{
		{
			name:       "paying without the role",
			candidate:  Candidate{UserID: 1, DiscordUserID: "1", ProUntil: aMonthOn()},
			wantCall:   "grant",
			wantRecord: []bool{true},
		},
		{
			name:       "lapsed with the role",
			candidate:  Candidate{UserID: 2, DiscordUserID: "2", RoleGranted: true},
			wantCall:   "revoke",
			wantRecord: []bool{false},
		},
		{
			name:       "paying with the role",
			candidate:  Candidate{UserID: 3, DiscordUserID: "3", ProUntil: aMonthOn(), RoleGranted: true},
			wantRecord: []bool{true},
		},
		{
			name:       "lapsed without the role",
			candidate:  Candidate{UserID: 4, DiscordUserID: "4"},
			wantRecord: []bool{false},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			store := &fakeStore{candidates: []Candidate{tc.candidate}}
			discord := &fakeDiscord{}
			svc := newTestService(store, discord)

			if _, err := svc.Sync(context.Background(), 10); err != nil {
				t.Fatalf("Sync: %v", err)
			}
			if tc.wantCall == "" {
				if len(discord.calls) != 0 {
					t.Errorf("calls = %v, want none — an unchanged account must cost no request",
						discord.calls)
				}
			} else if len(discord.calls) != 1 || discord.calls[0] != tc.wantCall {
				t.Errorf("calls = %v, want one %s", discord.calls, tc.wantCall)
			}

			// EVERY row is stamped, including the two that need no Discord call: the stamp is
			// what moves a row to the back of the reconciliation queue, and skipping it for
			// the settled accounts would pin them at the front forever and starve the rest.
			// What must not move for those two is the VALUE — the record is rewritten as it
			// already stood.
			if len(store.granted) != len(tc.wantRecord) {
				t.Fatalf("grant records = %v, want %v", store.granted, tc.wantRecord)
			}
			for i := range tc.wantRecord {
				if store.granted[i] != tc.wantRecord[i] {
					t.Errorf("grant records = %v, want %v", store.granted, tc.wantRecord)
				}
			}
		})
	}
}

// Ultra pays too. The one-role rule is what this protects: a tier added later must not
// silently lose its access because the check named the tiers it knew.
func TestSyncTreatsUltraAsPaying(t *testing.T) {
	store := &fakeStore{candidates: []Candidate{{UserID: 1, DiscordUserID: "1", UltraUntil: aMonthOn()}}}
	discord := &fakeDiscord{}
	svc := newTestService(store, discord)

	if _, err := svc.Sync(context.Background(), 10); err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if len(discord.calls) != 1 || discord.calls[0] != "grant" {
		t.Errorf("calls = %v, want one grant", discord.calls)
	}
}

// Somebody who left the server still has a row. The run must stamp it and carry on, or one
// departed member pins the queue and everybody behind them stops being reconciled.
func TestSyncTreatsAMissingMemberAsAnAbsence(t *testing.T) {
	store := &fakeStore{candidates: []Candidate{
		{UserID: 1, DiscordUserID: "1", ProUntil: aMonthOn()},
		{UserID: 2, DiscordUserID: "2", ProUntil: aMonthOn()},
	}}
	discord := &fakeDiscord{grantErr: ErrUnknownMember}
	svc := newTestService(store, discord)

	stats, err := svc.Sync(context.Background(), 10)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if stats.Examined != 2 {
		t.Errorf("examined = %d, want 2 — a departed member must not stop the run", stats.Examined)
	}
	if stats.Failed != 0 {
		t.Errorf("failed = %d, want 0 — leaving the server is not a failure", stats.Failed)
	}
	for _, g := range store.granted {
		if g {
			t.Error("recorded the role as granted for a member who is not in the guild")
		}
	}
}

// A real failure must be counted and must not stop the rest of the run, so one broken
// account cannot cost everybody else their hourly reconciliation.
func TestSyncCountsFailuresAndContinues(t *testing.T) {
	store := &fakeStore{candidates: []Candidate{
		{UserID: 1, DiscordUserID: "1", ProUntil: aMonthOn()},
		{UserID: 2, DiscordUserID: "2", ProUntil: aMonthOn()},
	}}
	discord := &fakeDiscord{grantErr: errors.New("discord is down")}
	svc := newTestService(store, discord)

	stats, err := svc.Sync(context.Background(), 10)
	if err != nil {
		t.Fatalf("Sync must not abort on a per-account failure: %v", err)
	}
	if stats.Examined != 2 || stats.Failed != 2 {
		t.Errorf("stats = %+v, want 2 examined and 2 failed", stats)
	}
}

func TestSyncHonoursTheBound(t *testing.T) {
	store := &fakeStore{candidates: []Candidate{
		{UserID: 1, DiscordUserID: "1", ProUntil: aMonthOn()},
		{UserID: 2, DiscordUserID: "2", ProUntil: aMonthOn()},
		{UserID: 3, DiscordUserID: "3", ProUntil: aMonthOn()},
	}}
	discord := &fakeDiscord{}
	svc := newTestService(store, discord)

	stats, err := svc.Sync(context.Background(), 2)
	if err != nil {
		t.Fatalf("Sync: %v", err)
	}
	if stats.Examined != 2 {
		t.Errorf("examined = %d, want the bound of 2", stats.Examined)
	}
}
