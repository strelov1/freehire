// Package applejobs drains durable Sign in with Apple revocations. Workers are
// run-once and safe to retry; grant material remains encrypted at rest.
package applejobs

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/strelov1/freehire/internal/auth/apple"
)

type revoker interface {
	Revoke(context.Context, string, string) error
}

type Worker struct {
	pool  *pgxpool.Pool
	apple revoker
	keys  *apple.KeyRing
	now   func() time.Time
}

func New(pool *pgxpool.Pool, client revoker, keys *apple.KeyRing) *Worker {
	return &Worker{pool: pool, apple: client, keys: keys, now: time.Now}
}

type job struct {
	id, grantID              uuid.UUID
	userID                   *int64
	reason                   string
	subject, clientID, keyID string
	ciphertext, nonce        []byte
	attempts                 int32
}

func (w *Worker) claim(ctx context.Context) (job, error) {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return job{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var j job
	err = tx.QueryRow(ctx, `SELECT id,token_aad_row_id,source_user_id,reason,source_provider_user_id,client_id,token_ciphertext,token_nonce,encryption_key_id,attempts FROM apple_revocation_jobs WHERE (status IN ('pending','retry') AND next_attempt_at<=now()) OR (status='processing' AND updated_at < now()-interval '10 minutes') ORDER BY next_attempt_at,created_at FOR UPDATE SKIP LOCKED LIMIT 1`).Scan(&j.id, &j.grantID, &j.userID, &j.reason, &j.subject, &j.clientID, &j.ciphertext, &j.nonce, &j.keyID, &j.attempts)
	if err != nil {
		return job{}, err
	}
	_, err = tx.Exec(ctx, `UPDATE apple_revocation_jobs SET status='processing',attempts=attempts+1,updated_at=now() WHERE id=$1`, j.id)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return j, err
}

func (w *Worker) Run(ctx context.Context, limit int) (int, error) {
	if w.apple == nil || w.keys == nil {
		return 0, errors.New("apple revocation: not configured")
	}
	if limit <= 0 {
		limit = 100
	}
	done := 0
	for done < limit {
		j, err := w.claim(ctx)
		if errors.Is(err, pgx.ErrNoRows) {
			return done, nil
		}
		if err != nil {
			return done, err
		}
		done++
		aad := apple.GrantAAD("apple", j.subject, j.clientID, j.grantID.String())
		plain, err := w.keys.Decrypt(j.ciphertext, j.nonce, j.keyID, aad)
		if err == nil {
			err = w.apple.Revoke(ctx, string(plain), j.clientID)
		}
		for i := range plain {
			plain[i] = 0
		}
		if err == nil {
			if err = w.complete(ctx, j); err != nil {
				return done, err
			}
			continue
		}
		if err = w.retry(ctx, j, err); err != nil {
			return done, err
		}
	}
	return done, nil
}

func (w *Worker) complete(ctx context.Context, j job) error {
	tx, err := w.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if j.reason == "unlink" && j.userID != nil {
		if _, err = tx.Exec(ctx, `DELETE FROM apple_grants WHERE id=$1`, j.grantID); err != nil {
			return err
		}
		if _, err = tx.Exec(ctx, `DELETE FROM user_identities WHERE user_id=$1 AND provider='apple' AND provider_user_id=$2 AND status='revocation_pending'`, *j.userID, j.subject); err != nil {
			return err
		}
	} else if j.reason == "exchange_compensation" {
		// Finalization uses the compensation job ID as the grant ID. A crash
		// before disarm can therefore remove exactly the revoked grant, while a
		// later successful retry has a different ID and is never touched.
		if _, err = tx.Exec(ctx, `DELETE FROM apple_grants WHERE id=$1`, j.grantID); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE apple_revocation_jobs SET status='completed',completed_at=now(),updated_at=now(),last_error_class=NULL,token_ciphertext=NULL,token_nonce=NULL,encryption_key_id=NULL WHERE id=$1`, j.id)
	if err == nil {
		err = tx.Commit(ctx)
	}
	return err
}

func (w *Worker) retry(ctx context.Context, j job, cause error) error {
	attempt := j.attempts + 1
	status := "retry"
	class := "provider_unavailable"
	delay := time.Minute*time.Duration(1<<min(int(attempt), 8)) + time.Duration(rand.IntN(30))*time.Second
	if errors.Is(cause, apple.ErrPermanent) {
		status = "failed"
		class = "provider_configuration"
	} else if errors.Is(cause, apple.ErrGrantDecrypt) {
		status = "failed"
		class = "grant_decryption"
	} else if attempt >= 10 {
		status = "failed"
		class = "retry_exhausted"
	}
	_, err := w.pool.Exec(ctx, `UPDATE apple_revocation_jobs SET status=$2,next_attempt_at=$3,last_error_class=$4,updated_at=now() WHERE id=$1`, j.id, status, w.now().Add(delay), class)
	if err != nil {
		return err
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
