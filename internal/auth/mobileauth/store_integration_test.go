//go:build integration

package mobileauth

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/auth/apple"
	"github.com/strelov1/freehire/internal/auth/recentauth"
	"github.com/strelov1/freehire/internal/testdb"
)

func seedAuthUser(t *testing.T, password bool) (context.Context, int64, *Store) {
	t.Helper()
	pool := testdb.Pool(t)
	ctx := context.Background()
	var userID int64
	var err error
	if password {
		err = pool.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES('mobile-auth@example.test','hash') RETURNING id`).Scan(&userID)
	} else {
		err = pool.QueryRow(ctx, `INSERT INTO users(email) VALUES('mobile-auth@example.test') RETURNING id`).Scan(&userID)
	}
	if err != nil {
		t.Fatal(err)
	}
	return ctx, userID, NewStore(pool)
}

func persistAppleGrant(t *testing.T, ctx context.Context, store *Store, ring *apple.KeyRing, userID int64, subject, clientID, refreshToken string) {
	t.Helper()
	jobID, err := store.CreateAppleCompensation(ctx, ring, subject, clientID, refreshToken)
	if err != nil {
		t.Fatal(err)
	}
	if err = store.FinalizeAppleGrant(ctx, userID, subject, clientID, jobID); err != nil {
		t.Fatal(err)
	}
	if err = store.DisarmAppleCompensation(ctx, jobID); err != nil {
		t.Fatal(err)
	}
}

func TestReauthCodeCarriesExactSessionBindingAndBurnsOnce(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"
	sessionHash := bytes.Repeat([]byte{7}, 32)
	binding := &Binding{UserID: userID, TokenVersion: 1, SessionHash: sessionHash}
	attempt, state, err := store.CreateBrowserAttempt(ctx, "google", "web", "web", "reauth", Challenge(verifier), binding)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err = store.ConsumeBrowserAttempt(ctx, state, "google")
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(attempt.ExpectedSessionHash, sessionHash) {
		t.Fatal("attempt lost its session binding")
	}
	code, err := store.MintCode(ctx, attempt, userID)
	if err != nil {
		t.Fatal(err)
	}
	exchange, err := store.ConsumeCode(ctx, code, verifier)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(exchange.ExpectedSessionHash, sessionHash) {
		t.Fatal("exchange code lost its session binding")
	}
	if _, err = store.ConsumeCode(ctx, code, verifier); !errors.Is(err, ErrInvalid) {
		t.Fatalf("replayed code accepted: %v", err)
	}
}

func TestAppleWebReauthUsesBrowserAttemptNamespace(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"
	binding := &Binding{UserID: userID, TokenVersion: 1, SessionHash: bytes.Repeat([]byte{8}, 32)}
	_, state, err := store.CreateBrowserAttempt(ctx, "apple", "web", "web", "reauth", Challenge(verifier), binding)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.ConsumeBrowserAttempt(ctx, state, "apple"); err != nil {
		t.Fatalf("Apple Services-ID reauth could not consume browser attempt: %v", err)
	}
}

func TestUnlinkCountsProviderAsOneUsableMethod(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, false)
	pool := store.pool
	for _, subject := range []string{"google-one", "google-two"} {
		if _, err := pool.Exec(ctx, `INSERT INTO user_identities(provider,provider_user_id,user_id) VALUES('google',$1,$2)`, subject, userID); err != nil {
			t.Fatal(err)
		}
	}
	sessionHash := bytes.Repeat([]byte{3}, 32)
	proof, _, err := recentauth.NewStore(pool, 0).Issue(ctx, userID, 1, sessionHash)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.UnlinkIdentity(ctx, userID, 1, sessionHash, "google", proof); !errors.Is(err, ErrLastMethod) {
		t.Fatalf("same-provider rows bypassed last-method guard: %v", err)
	}
	var count int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id=$1`, userID).Scan(&count); err != nil || count != 2 {
		t.Fatalf("failed unlink changed identities: count=%d err=%v", count, err)
	}
}

func TestFinalizeAppleGrantCannotOverwritePendingUnlink(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	const subject, clientID = "apple-race-subject", "me.freehire.mobile"
	if _, err := store.pool.Exec(ctx, `INSERT INTO user_identities(provider,provider_user_id,user_id) VALUES('apple',$1,$2)`, subject, userID); err != nil {
		t.Fatal(err)
	}
	ring, err := apple.NewKeyRing("v1", map[string][]byte{"v1": bytes.Repeat([]byte{4}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	persistAppleGrant(t, ctx, store, ring, userID, subject, clientID, "old-refresh")
	var originalID string
	if err = store.pool.QueryRow(ctx, `SELECT id::text FROM apple_grants WHERE provider_user_id=$1`, subject).Scan(&originalID); err != nil {
		t.Fatal(err)
	}

	// Hold the same first lock UnlinkIdentity takes. FinalizeAppleGrant must wait,
	// then observe the pending identity rather than replacing its queued grant.
	blocker, err := store.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err = blocker.Exec(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	newCompensation, err := store.CreateAppleCompensation(ctx, ring, subject, clientID, "new-refresh")
	if err != nil {
		t.Fatal(err)
	}
	go func() { result <- store.FinalizeAppleGrant(ctx, userID, subject, clientID, newCompensation) }()
	if _, err = blocker.Exec(ctx, `UPDATE user_identities SET status='revocation_pending',status_changed_at=now() WHERE user_id=$1 AND provider='apple'`, userID); err != nil {
		t.Fatal(err)
	}
	if err = blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		if !errors.Is(err, ErrState) {
			t.Fatalf("finalize err=%v, want ErrState", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("grant finalization remained blocked")
	}
	var finalID string
	if err = store.pool.QueryRow(ctx, `SELECT id::text FROM apple_grants WHERE provider_user_id=$1`, subject).Scan(&finalID); err != nil {
		t.Fatal(err)
	}
	if finalID != originalID {
		t.Fatalf("pending unlink grant replaced: before=%s after=%s", originalID, finalID)
	}
}

func TestConcurrentAppleFinalizeAndUnlinkPreserveQueuedGrant(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	const subject, clientID = "apple-concurrent-subject", "me.freehire.mobile"
	ring, err := apple.NewKeyRing("v1", map[string][]byte{"v1": bytes.Repeat([]byte{5}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 10; round++ {
		if _, err = store.pool.Exec(ctx, `DELETE FROM apple_revocation_jobs`); err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `DELETE FROM apple_grants`); err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `DELETE FROM user_identities WHERE user_id=$1`, userID); err != nil {
			t.Fatal(err)
		}
		if _, err = store.pool.Exec(ctx, `INSERT INTO user_identities(provider,provider_user_id,user_id) VALUES('apple',$1,$2)`, subject, userID); err != nil {
			t.Fatal(err)
		}
		persistAppleGrant(t, ctx, store, ring, userID, subject, clientID, "old-refresh")
		sessionHash := bytes.Repeat([]byte{byte(round + 1)}, 32)
		proof, _, issueErr := recentauth.NewStore(store.pool, 0).Issue(ctx, userID, 1, sessionHash)
		if issueErr != nil {
			t.Fatal(issueErr)
		}
		start := make(chan struct{})
		unlinkResult := make(chan error, 1)
		finalizeResult := make(chan error, 1)
		newCompensation, createErr := store.CreateAppleCompensation(ctx, ring, subject, clientID, "new-refresh")
		if createErr != nil {
			t.Fatal(createErr)
		}
		go func() {
			<-start
			_, unlinkErr := store.UnlinkIdentity(ctx, userID, 1, sessionHash, "apple", proof)
			unlinkResult <- unlinkErr
		}()
		go func() {
			<-start
			finalizeResult <- store.FinalizeAppleGrant(ctx, userID, subject, clientID, newCompensation)
		}()
		close(start)
		unlinkErr, finalizeErr := <-unlinkResult, <-finalizeResult

		var status string
		if err = store.pool.QueryRow(ctx, `SELECT status FROM user_identities WHERE user_id=$1 AND provider='apple'`, userID).Scan(&status); err != nil {
			t.Fatalf("round %d identity: unlink=%v finalize=%v query=%v", round, unlinkErr, finalizeErr, err)
		}
		if status != "revocation_pending" {
			continue
		}
		var matching int
		if err = store.pool.QueryRow(ctx, `
			SELECT count(*) FROM apple_grants grant_row
			JOIN apple_revocation_jobs job ON job.token_aad_row_id=grant_row.id
			WHERE grant_row.user_id=$1 AND job.reason='unlink'
			  AND job.status IN ('pending','processing','retry')`, userID).Scan(&matching); err != nil {
			t.Fatal(err)
		}
		if matching != 1 {
			t.Fatalf("round %d pending identity lost queued grant: unlink=%v finalize=%v matching=%d", round, unlinkErr, finalizeErr, matching)
		}
	}
}

// A user who signed in with native Apple leaves an apple_grants row that
// references users with ON DELETE RESTRICT. Account deletion must be able to
// clear it — by queuing a revocation job carrying its own encrypted copy of
// the token and dropping the row — rather than have Postgres reject the
// DELETE FROM users underneath it.
func TestReleaseAppleGrantsForDeletionUnblocksUserDelete(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	const subject, clientID = "apple-deletion-subject", "me.freehire.mobile"
	if _, err := store.pool.Exec(ctx, `INSERT INTO user_identities(provider,provider_user_id,user_id) VALUES('apple',$1,$2)`, subject, userID); err != nil {
		t.Fatal(err)
	}
	ring, err := apple.NewKeyRing("v1", map[string][]byte{"v1": bytes.Repeat([]byte{6}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	persistAppleGrant(t, ctx, store, ring, userID, subject, clientID, "refresh-token")

	if err = store.ReleaseAppleGrantsForDeletion(ctx, userID); err != nil {
		t.Fatalf("ReleaseAppleGrantsForDeletion: %v", err)
	}

	var grants int
	if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM apple_grants WHERE user_id=$1`, userID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if grants != 0 {
		t.Fatalf("apple_grants rows left = %d, want 0", grants)
	}
	var jobs int
	if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM apple_revocation_jobs WHERE source_user_id=$1 AND reason='account_deletion' AND status='pending'`, userID).Scan(&jobs); err != nil {
		t.Fatal(err)
	}
	if jobs != 1 {
		t.Fatalf("account_deletion revocation jobs = %d, want 1", jobs)
	}

	if _, err = store.pool.Exec(ctx, `DELETE FROM users WHERE id=$1`, userID); err != nil {
		t.Fatalf("DELETE FROM users still blocked after releasing apple grants: %v", err)
	}
}

// Calling it again after everything is already released — the retry path an
// account-deletion request takes if a prior attempt failed downstream — must
// not error just because there is nothing left to do.
func TestReleaseAppleGrantsForDeletionIsIdempotent(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	if err := store.ReleaseAppleGrantsForDeletion(ctx, userID); err != nil {
		t.Fatalf("ReleaseAppleGrantsForDeletion on a user with no grants: %v", err)
	}
	if err := store.ReleaseAppleGrantsForDeletion(ctx, userID); err != nil {
		t.Fatalf("ReleaseAppleGrantsForDeletion called twice: %v", err)
	}
}

func TestCleanupPreservesAttemptWithLiveExchangeCode(t *testing.T) {
	ctx, userID, store := seedAuthUser(t, true)
	verifier := "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQ"
	attempt, state, err := store.CreateBrowserAttempt(ctx, "google", "ios", "ios", "sign_in", Challenge(verifier), nil)
	if err != nil {
		t.Fatal(err)
	}
	attempt, err = store.ConsumeBrowserAttempt(ctx, state, "google")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = store.MintCode(ctx, attempt, userID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.pool.Exec(ctx, `UPDATE oauth_auth_attempts SET expires_at=now()-interval '1 second' WHERE id=$1`, attempt.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = store.Cleanup(ctx, 100); err != nil {
		t.Fatal(err)
	}
	var attempts, codes int
	if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM oauth_auth_attempts WHERE id=$1`, attempt.ID).Scan(&attempts); err != nil {
		t.Fatal(err)
	}
	if err = store.pool.QueryRow(ctx, `SELECT count(*) FROM oauth_exchange_codes WHERE source_attempt_id=$1 AND expires_at>now()`, attempt.ID).Scan(&codes); err != nil {
		t.Fatal(err)
	}
	if attempts != 1 || codes != 1 {
		t.Fatalf("cleanup burned live exchange: attempts=%d codes=%d", attempts, codes)
	}
}
