//go:build integration

package applejobs

import (
	"bytes"
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/strelov1/freehire/internal/identity/auth/apple"
	"github.com/strelov1/freehire/internal/identity/auth/mobileauth"
	"github.com/strelov1/freehire/internal/platform/testdb"
)

type recordingRevoker struct {
	token, clientID string
	calls           int
}

func (r *recordingRevoker) Revoke(_ context.Context, token, clientID string) error {
	r.calls++
	r.token, r.clientID = token, clientID
	return nil
}

func TestWorkerRevokesThenRemovesGrantAndIdentity(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email) VALUES('apple-worker@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	const subject, clientID, refresh = "apple-subject", "me.freehire.mobile", "refresh-secret"
	if _, err := pool.Exec(ctx, `INSERT INTO user_identities(provider,provider_user_id,user_id,status) VALUES('apple',$1,$2,'revocation_pending')`, subject, userID); err != nil {
		t.Fatal(err)
	}
	key := bytes.Repeat([]byte{9}, 32)
	ring, err := apple.NewKeyRing("v1", map[string][]byte{"v1": key})
	if err != nil {
		t.Fatal(err)
	}
	grantID := uuid.New()
	ciphertext, nonce, keyID, err := ring.Encrypt([]byte(refresh), apple.GrantAAD("apple", subject, clientID, grantID.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO apple_grants(id,user_id,provider_user_id,client_id,refresh_token_ciphertext,refresh_token_nonce,encryption_key_id,revocation_requested_at) VALUES($1,$2,$3,$4,$5,$6,$7,now())`, grantID, userID, subject, clientID, ciphertext, nonce, keyID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO apple_revocation_jobs(idempotency_key,reason,source_user_id,source_provider,source_provider_user_id,token_aad_row_id,client_id,token_ciphertext,token_nonce,encryption_key_id) VALUES($1,'unlink',$2,'apple',$3,$4,$5,$6,$7,$8)`, "unlink:apple:"+grantID.String(), userID, subject, grantID, clientID, ciphertext, nonce, keyID); err != nil {
		t.Fatal(err)
	}
	revoker := &recordingRevoker{}
	stats, err := New(pool, revoker, ring).Run(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Processed != 1 || stats.Revoked != 1 || stats.Failed != 0 || revoker.calls != 1 ||
		revoker.token != refresh || revoker.clientID != clientID {
		t.Fatalf("revocation mismatch: stats=%+v revoker=%+v", stats, revoker)
	}
	var grants, identities int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM apple_grants WHERE user_id=$1`, userID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM user_identities WHERE user_id=$1 AND provider='apple'`, userID).Scan(&identities); err != nil {
		t.Fatal(err)
	}
	var status string
	var retainedCiphertext, retainedNonce []byte
	var retainedKeyID *string
	if err = pool.QueryRow(ctx, `SELECT status,token_ciphertext,token_nonce,encryption_key_id FROM apple_revocation_jobs WHERE idempotency_key=$1`, "unlink:apple:"+grantID.String()).Scan(&status, &retainedCiphertext, &retainedNonce, &retainedKeyID); err != nil {
		t.Fatal(err)
	}
	if grants != 0 || identities != 0 || status != "completed" || retainedCiphertext != nil || retainedNonce != nil || retainedKeyID != nil {
		t.Fatalf("completion not durable: grants=%d identities=%d status=%s", grants, identities, status)
	}
}

func TestWorkerRevokesExchangeCompensationWithoutTouchingIdentity(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	ring, err := apple.NewKeyRing("v1", map[string][]byte{"v1": bytes.Repeat([]byte{6}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	store := mobileauth.NewStore(pool)
	var userID int64
	if err = pool.QueryRow(ctx, `INSERT INTO users(email,password_hash) VALUES('compensation@example.test','hash') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO user_identities(provider,provider_user_id,user_id) VALUES('apple','compensation-subject',$1)`, userID); err != nil {
		t.Fatal(err)
	}
	jobID, err := store.CreateAppleCompensation(ctx, ring, "compensation-subject", "me.freehire.mobile", "orphan-refresh")
	if err != nil {
		t.Fatal(err)
	}
	// Simulate a crash after grant finalization but before compensation disarm.
	if err = store.FinalizeAppleGrant(ctx, userID, "compensation-subject", "me.freehire.mobile", jobID); err != nil {
		t.Fatal(err)
	}
	if err = store.ActivateAppleCompensation(ctx, jobID); err != nil {
		t.Fatal(err)
	}
	revoker := &recordingRevoker{}
	stats, err := New(pool, revoker, ring).Run(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Processed != 1 || stats.Revoked != 1 || revoker.token != "orphan-refresh" {
		t.Fatalf("compensation not revoked: stats=%+v revoker=%+v", stats, revoker)
	}
	var status, identityStatus string
	var ciphertext []byte
	if err = pool.QueryRow(ctx, `SELECT status,token_ciphertext FROM apple_revocation_jobs WHERE id=$1`, jobID).Scan(&status, &ciphertext); err != nil {
		t.Fatal(err)
	}
	if status != "completed" || ciphertext != nil {
		t.Fatalf("compensation retained secret: status=%s ciphertext=%x", status, ciphertext)
	}
	var grants int
	if err = pool.QueryRow(ctx, `SELECT count(*) FROM apple_grants WHERE id=$1`, jobID).Scan(&grants); err != nil {
		t.Fatal(err)
	}
	if err = pool.QueryRow(ctx, `SELECT status FROM user_identities WHERE user_id=$1 AND provider='apple'`, userID).Scan(&identityStatus); err != nil {
		t.Fatal(err)
	}
	if grants != 0 || identityStatus != "active" {
		t.Fatalf("compensation cleanup grants=%d identity=%s", grants, identityStatus)
	}
}

// refusingRevoker stands in for Apple answering with something no retry can fix.
type refusingRevoker struct{}

func (refusingRevoker) Revoke(context.Context, string, string) error { return apple.ErrPermanent }

// A run that gave up on every revocation it claimed used to be byte-identical to a clean
// one: `done` counted jobs touched, not jobs revoked, so cmd/apple-revoke printed
// processed=1 and exited 0. Nothing else watches this queue, and the abandoned `unlink`
// leaves user_identities in revocation_pending — a state from which that identity can
// never be unlinked again.
func TestWorkerReportsAJobItGaveUpOn(t *testing.T) {
	pool := testdb.Pool(t)
	ctx := context.Background()
	var userID int64
	if err := pool.QueryRow(ctx, `INSERT INTO users(email) VALUES('apple-permanent@example.test') RETURNING id`).Scan(&userID); err != nil {
		t.Fatal(err)
	}
	const subject, clientID = "apple-permanent-subject", "me.freehire.mobile"
	ring, err := apple.NewKeyRing("v1", map[string][]byte{"v1": bytes.Repeat([]byte{3}, 32)})
	if err != nil {
		t.Fatal(err)
	}
	grantID := uuid.New()
	ciphertext, nonce, keyID, err := ring.Encrypt([]byte("refresh-secret"),
		apple.GrantAAD("apple", subject, clientID, grantID.String()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err = pool.Exec(ctx, `INSERT INTO apple_revocation_jobs(idempotency_key,reason,source_user_id,source_provider,source_provider_user_id,token_aad_row_id,client_id,token_ciphertext,token_nonce,encryption_key_id) VALUES($1,'unlink',$2,'apple',$3,$4,$5,$6,$7,$8)`,
		"unlink:apple:"+grantID.String(), userID, subject, grantID, clientID, ciphertext, nonce, keyID); err != nil {
		t.Fatal(err)
	}

	stats, err := New(pool, refusingRevoker{}, ring).Run(ctx, 1)
	if err != nil {
		t.Fatal(err)
	}
	if stats.Processed != 1 || stats.Failed != 1 || stats.Revoked != 0 || stats.Retried != 0 {
		t.Fatalf("stats = %+v, want one job given up on for good — that is what makes the run "+
			"exit non-zero", stats)
	}
	var status string
	if err = pool.QueryRow(ctx, `SELECT status FROM apple_revocation_jobs WHERE idempotency_key=$1`,
		"unlink:apple:"+grantID.String()).Scan(&status); err != nil {
		t.Fatal(err)
	}
	if status != "failed" {
		t.Fatalf("status = %s, want failed", status)
	}
}
