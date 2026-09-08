package linkedinauth

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"
)

// fakeStore is the credential store as a value. The renewal rules are a decision table over
// clocks, and a table that can only be exercised against a live database is a table nobody
// re-reads.
type fakeStore struct {
	token   Token
	present bool
	loadErr error

	renewed     Token
	renewCalls  int
	renewResult bool
	renewErr    error

	saved Token
}

func (f *fakeStore) Load(context.Context) (Token, bool, error) {
	if f.loadErr != nil {
		return Token{}, false, f.loadErr
	}
	return f.token, f.present, nil
}

func (f *fakeStore) Save(_ context.Context, t Token) error {
	f.saved = t
	return nil
}

func (f *fakeStore) Renew(_ context.Context, _ string, t Token) (bool, error) {
	f.renewCalls++
	f.renewed = t
	if f.renewErr != nil {
		return false, f.renewErr
	}
	return f.renewResult, nil
}

var testNow = time.Date(2026, 9, 8, 12, 0, 0, 0, time.UTC)

func at(d time.Duration) time.Time { return testNow.Add(d) }

// renewerOver builds a Renewer whose clock is pinned and whose token endpoint, when reached,
// answers with a fresh 60-day grant.
func renewerOver(t *testing.T, store Store) *Renewer {
	t.Helper()
	pinClock(t, testNow)
	tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `{"access_token":"FRESH","expires_in":5184000,`+
			`"refresh_token":"AQW","refresh_token_expires_in":26000000}`)
	})
	return NewRenewer(testCredentials(), store, nil, func() time.Time { return testNow })
}

func TestRenewDecisionTable(t *testing.T) {
	day := 24 * time.Hour

	t.Run("no credential is a state, not a failure", func(t *testing.T) {
		store := &fakeStore{present: false}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateMissing {
			t.Errorf("state = %q, want %q", out.State, StateMissing)
		}
	})

	t.Run("a token with weeks left is left alone", func(t *testing.T) {
		store := &fakeStore{present: true, token: Token{
			AccessToken: "AQV", ExpiresAt: at(40 * day), RefreshToken: "AQW", RefreshExpiresAt: at(300 * day),
		}}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateHealthy {
			t.Errorf("state = %q, want %q", out.State, StateHealthy)
		}
		if store.renewCalls != 0 {
			t.Errorf("a healthy token was renewed %d times", store.renewCalls)
		}
	})

	t.Run("inside the window a renewable token is renewed", func(t *testing.T) {
		store := &fakeStore{present: true, renewResult: true, token: Token{
			AccessToken: "AQV", ExpiresAt: at(10 * day), RefreshToken: "AQW", RefreshExpiresAt: at(300 * day),
		}}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateRenewed {
			t.Fatalf("state = %q, want %q", out.State, StateRenewed)
		}
		if store.renewed.AccessToken != "FRESH" {
			t.Errorf("stored access token = %q, want the renewed one", store.renewed.AccessToken)
		}
		if !out.ExpiresAt.After(at(50 * day)) {
			t.Errorf("the reported expiry is the OLD one: %s", out.ExpiresAt)
		}
	})

	// The expected case outside the Marketing Developer Platform: nothing can be renewed, so
	// the job is to ask for a person while there is still a fortnight to answer.
	t.Run("inside the window a token with no refresh warns", func(t *testing.T) {
		store := &fakeStore{present: true, token: Token{AccessToken: "AQV", ExpiresAt: at(10 * day)}}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateWarn {
			t.Fatalf("state = %q, want %q", out.State, StateWarn)
		}
		if out.Reason == "" {
			t.Error("a warning with no reason tells nobody what to do")
		}
		if store.renewCalls != 0 {
			t.Errorf("a renewal was attempted without a refresh token")
		}
	})

	t.Run("an expired token with no refresh is broken now", func(t *testing.T) {
		store := &fakeStore{present: true, token: Token{AccessToken: "AQV", ExpiresAt: at(-2 * day)}}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateExpired {
			t.Errorf("state = %q, want %q", out.State, StateExpired)
		}
	})

	// An expired ACCESS token is still renewable while the refresh token lives — that is what a
	// refresh token is for, and treating this as terminal would send somebody to sign in for a
	// credential the worker could have repaired itself.
	t.Run("an expired token with a live refresh is renewed", func(t *testing.T) {
		store := &fakeStore{present: true, renewResult: true, token: Token{
			AccessToken: "AQV", ExpiresAt: at(-2 * day), RefreshToken: "AQW", RefreshExpiresAt: at(200 * day),
		}}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateRenewed {
			t.Errorf("state = %q, want %q", out.State, StateRenewed)
		}
	})

	// A renewal extends the access token and never the grant behind it. Without this branch the
	// yearly sign-in would arrive as a channel that stopped, a year after anybody thought about it.
	t.Run("a healthy token whose grant is running out still warns", func(t *testing.T) {
		store := &fakeStore{present: true, token: Token{
			AccessToken: "AQV", ExpiresAt: at(40 * day), RefreshToken: "AQW", RefreshExpiresAt: at(5 * day),
		}}
		out, err := renewerOver(t, store).Run(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		if out.State != StateWarn {
			t.Fatalf("state = %q, want %q", out.State, StateWarn)
		}
		if store.renewCalls != 0 {
			t.Error("the access token was renewed although it had weeks left")
		}
	})
}

// A failed renewal is a warning, not a silent pass: there may be days left, and those days are
// exactly when a person can still act.
func TestFailedRenewalWarnsRatherThanHidesTheProblem(t *testing.T) {
	pinClock(t, testNow)
	tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		writeBody(t, w, `{"error":"invalid_request","error_description":"refresh token revoked"}`)
	})
	store := &fakeStore{present: true, token: Token{
		AccessToken: "AQV", ExpiresAt: at(10 * 24 * time.Hour), RefreshToken: "AQW",
	}}

	out, err := NewRenewer(testCredentials(), store, nil, func() time.Time { return testNow }).
		Run(context.Background())
	if err != nil {
		t.Fatalf("a failed renewal became a run error: %v", err)
	}
	if out.State != StateWarn {
		t.Fatalf("state = %q, want %q", out.State, StateWarn)
	}
	if out.Reason == "" {
		t.Error("the warning does not say what failed")
	}
}

// Somebody signing in while a renewal is in flight holds a NEWER credential. Overwriting it
// would replace a fresh 60-day grant with one derived from the token it replaced.
func TestARenewalThatLostTheRaceChangesNothing(t *testing.T) {
	store := &fakeStore{present: true, renewResult: false, token: Token{
		AccessToken: "AQV", ExpiresAt: at(10 * 24 * time.Hour), RefreshToken: "AQW",
	}}
	out, err := renewerOver(t, store).Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if out.State != StateHealthy {
		t.Errorf("state = %q, want %q", out.State, StateHealthy)
	}
}

// A response that omits the refresh token must not blank the stored one — that would silently
// turn a renewable credential into one that warns tomorrow and dies in sixty days.
func TestARenewalKeepsTheOldRefreshTokenWhenNoneComesBack(t *testing.T) {
	pinClock(t, testNow)
	tokenServerAt(t, func(w http.ResponseWriter, _ *http.Request) {
		writeBody(t, w, `{"access_token":"FRESH","expires_in":5184000}`)
	})
	store := &fakeStore{present: true, renewResult: true, token: Token{
		AccessToken: "AQV", ExpiresAt: at(10 * 24 * time.Hour),
		RefreshToken: "AQW", RefreshExpiresAt: at(200 * 24 * time.Hour),
	}}

	if _, err := NewRenewer(testCredentials(), store, nil, func() time.Time { return testNow }).
		Run(context.Background()); err != nil {
		t.Fatal(err)
	}
	if store.renewed.RefreshToken != "AQW" {
		t.Errorf("stored refresh token = %q, want the previous one kept", store.renewed.RefreshToken)
	}
	if !store.renewed.RefreshExpiresAt.Equal(at(200 * 24 * time.Hour)) {
		t.Errorf("stored grant expiry = %s, want the previous one kept", store.renewed.RefreshExpiresAt)
	}
}

// Failing to READ the credential is the one failure that is the run's own, and it must not be
// mistaken for "no credential stored" — which is a clean exit.
func TestAnUnreadableStoreIsAnError(t *testing.T) {
	store := &fakeStore{loadErr: errors.New("connection refused")}
	if _, err := renewerOver(t, store).Run(context.Background()); err == nil {
		t.Fatal("an unreadable store was reported as a clean run")
	}
}
