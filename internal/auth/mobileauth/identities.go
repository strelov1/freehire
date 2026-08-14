package mobileauth

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/strelov1/freehire/internal/auth/apple"
	"github.com/strelov1/freehire/internal/pgerr"
)

var (
	ErrLastMethod = errors.New("mobile auth: last sign-in method")
	ErrState      = errors.New("mobile auth: identity state conflict")
)

type Identity struct {
	Provider  string    `json:"provider"`
	LinkedAt  time.Time `json:"linked_at"`
	Status    string    `json:"status"`
	CanUnlink bool      `json:"can_unlink"`
}

type IdentityList struct {
	HasPassword bool       `json:"has_password"`
	Identities  []Identity `json:"identities"`
}

func (s *Store) ListIdentities(ctx context.Context, userID int64) (IdentityList, error) {
	var out IdentityList
	if err := s.pool.QueryRow(ctx, `SELECT password_hash IS NOT NULL FROM users WHERE id=$1`, userID).Scan(&out.HasPassword); err != nil {
		return out, err
	}
	// bool_or, not bool_and: unlinkIdentityOnce (store.go) counts a provider as active if
	// ANY of its identity rows is active, so a provider with one active row and one
	// revocation_pending row (e.g. mid-drain of an earlier unlink) is a usable sign-in
	// method there. Aggregating with bool_and instead would report that provider as
	// non-active here, undercounting active providers and showing CanUnlink=false for a
	// last-method check the backend would actually pass.
	rows, err := s.pool.Query(ctx, `
		SELECT provider,min(created_at),
		       CASE WHEN bool_or(status='active') THEN 'active' ELSE 'revocation_pending' END
		FROM user_identities WHERE user_id=$1
		GROUP BY provider ORDER BY provider`, userID)
	if err != nil {
		return out, err
	}
	defer rows.Close()
	for rows.Next() {
		var i Identity
		if err := rows.Scan(&i.Provider, &i.LinkedAt, &i.Status); err != nil {
			return out, err
		}
		out.Identities = append(out.Identities, i)
	}
	if err := rows.Err(); err != nil {
		return out, err
	}
	active := 0
	for _, i := range out.Identities {
		if i.Status == "active" {
			active++
		}
	}
	for i := range out.Identities {
		out.Identities[i].CanUnlink = out.Identities[i].Status == "active" && (out.HasPassword || active > 1)
	}
	return out, nil
}

const appleCompensationDelay = 15 * time.Minute

// FinalizeAppleGrant serializes grant persistence with identity unlink. Both
// paths lock the user and then the identity, so a login that resolves before an
// unlink cannot overwrite the queued grant after the identity becomes pending.
func (s *Store) FinalizeAppleGrant(ctx context.Context, userID int64, subject, clientID string, compensationID uuid.UUID) error {
	if subject == "" || clientID == "" || compensationID == uuid.Nil {
		return errors.New("mobile auth: incomplete Apple grant")
	}
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.finalizeAppleGrantOnce(ctx, userID, subject, clientID, compensationID)
		if !pgerr.IsSerializationFailure(err) {
			return err
		}
	}
	return err
}

func (s *Store) finalizeAppleGrantOnce(ctx context.Context, userID int64, subject, clientID string, compensationID uuid.UUID) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var lockedUser int64
	if err = tx.QueryRow(ctx, `SELECT id FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&lockedUser); err != nil {
		return err
	}
	var status string
	if err = tx.QueryRow(ctx, `SELECT status FROM user_identities WHERE user_id=$1 AND provider='apple' AND provider_user_id=$2 FOR UPDATE`, userID, subject).Scan(&status); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrIdentity
		}
		return err
	}
	if status != "active" {
		return ErrState
	}
	var ciphertext, nonce []byte
	var keyID string
	if err = tx.QueryRow(ctx, `
		SELECT token_ciphertext,token_nonce,encryption_key_id
		FROM apple_revocation_jobs
		WHERE id=$1 AND reason='exchange_compensation' AND status='pending'
		  AND source_provider='apple' AND source_provider_user_id=$2 AND client_id=$3
		FOR UPDATE`, compensationID, subject, clientID).Scan(&ciphertext, &nonce, &keyID); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrState
		}
		return err
	}
	tag, err := tx.Exec(ctx, `
		INSERT INTO apple_grants
			(id,user_id,provider,provider_user_id,client_id,refresh_token_ciphertext,
			 refresh_token_nonce,encryption_key_id)
		VALUES($1,$2,'apple',$3,$4,$5,$6,$7)
		ON CONFLICT(provider,provider_user_id) DO UPDATE SET
			id=EXCLUDED.id,user_id=EXCLUDED.user_id,client_id=EXCLUDED.client_id,
			refresh_token_ciphertext=EXCLUDED.refresh_token_ciphertext,
			refresh_token_nonce=EXCLUDED.refresh_token_nonce,
			encryption_key_id=EXCLUDED.encryption_key_id,updated_at=now(),
			revocation_requested_at=NULL
		WHERE apple_grants.user_id=EXCLUDED.user_id`, compensationID, userID, subject, clientID, ciphertext, nonce, keyID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrIdentity
	}
	return tx.Commit(ctx)
}

// CreateAppleCompensation takes durable custody of a newly exchanged refresh
// token before any later database finalization can fail. The delayed job is
// disarmed after a successful grant write; a crash leaves it safely revocable.
func (s *Store) CreateAppleCompensation(ctx context.Context, ring *apple.KeyRing, subject, clientID, refreshToken string) (uuid.UUID, error) {
	if ring == nil || subject == "" || clientID == "" || refreshToken == "" {
		return uuid.Nil, errors.New("mobile auth: incomplete Apple compensation")
	}
	jobID := uuid.New()
	aad := apple.GrantAAD("apple", subject, clientID, jobID.String())
	ciphertext, nonce, keyID, err := ring.Encrypt([]byte(refreshToken), aad)
	if err != nil {
		return uuid.Nil, err
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO apple_revocation_jobs
			(id,idempotency_key,reason,source_provider,source_provider_user_id,
			 token_aad_row_id,client_id,token_ciphertext,token_nonce,
			 encryption_key_id,next_attempt_at)
		VALUES($1,$2,'exchange_compensation','apple',$3,$1,$4,$5,$6,$7,$8)`,
		jobID, "exchange-compensation:apple:"+jobID.String(), subject, clientID,
		ciphertext, nonce, keyID, s.now().Add(appleCompensationDelay))
	if err != nil {
		return uuid.Nil, err
	}
	return jobID, nil
}

// DisarmAppleCompensation retains only a secret-free idempotency tombstone.
func (s *Store) DisarmAppleCompensation(ctx context.Context, jobID uuid.UUID) error {
	tag, err := s.pool.Exec(ctx, `
		UPDATE apple_revocation_jobs
		SET status='completed',completed_at=now(),updated_at=now(),
		    token_ciphertext=NULL,token_nonce=NULL,encryption_key_id=NULL
		WHERE id=$1 AND reason='exchange_compensation' AND status IN ('pending','retry')`, jobID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() != 1 {
		return ErrState
	}
	return nil
}

// ActivateAppleCompensation makes failed finalization eligible for immediate
// worker pickup. Failure to activate is non-fatal because the delayed pending
// job already remains durable.
func (s *Store) ActivateAppleCompensation(ctx context.Context, jobID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE apple_revocation_jobs SET next_attempt_at=now(),updated_at=now()
		WHERE id=$1 AND reason='exchange_compensation' AND status='pending'`, jobID)
	return err
}

func proofDigest(raw string) []byte { sum := sha256.Sum256([]byte(raw)); return sum[:] }

// ReleaseAppleGrantsForDeletion queues Apple revocation for every grant the
// account holds and clears the apple_grants rows, freeing the FK
// (ON DELETE RESTRICT) that would otherwise block deleting the user. The
// ciphertext is carried into the revocation job before the grant row is
// dropped, so the background worker can still revoke it with Apple even
// though the account — and the grant's own row — are already gone.
// Idempotent: a retried account deletion finds no grants left and no-ops.
func (s *Store) ReleaseAppleGrantsForDeletion(ctx context.Context, userID int64) error {
	var err error
	for attempt := 0; attempt < 3; attempt++ {
		err = s.releaseAppleGrantsForDeletionOnce(ctx, userID)
		if !pgerr.IsSerializationFailure(err) {
			return err
		}
	}
	return err
}

func (s *Store) releaseAppleGrantsForDeletionOnce(ctx context.Context, userID int64) error {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	type appleGrant struct {
		id                       uuid.UUID
		subject, clientID, keyID string
		ciphertext, nonce        []byte
	}
	rows, err := tx.Query(ctx, `SELECT id,provider_user_id,client_id,refresh_token_ciphertext,refresh_token_nonce,encryption_key_id FROM apple_grants WHERE user_id=$1 AND provider='apple' FOR UPDATE`, userID)
	if err != nil {
		return err
	}
	var grants []appleGrant
	for rows.Next() {
		var g appleGrant
		if err = rows.Scan(&g.id, &g.subject, &g.clientID, &g.ciphertext, &g.nonce, &g.keyID); err != nil {
			rows.Close()
			return err
		}
		grants = append(grants, g)
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return err
	}
	for _, g := range grants {
		idempotency := "account_deletion:apple:" + g.id.String()
		if _, err = tx.Exec(ctx, `INSERT INTO apple_revocation_jobs(idempotency_key,reason,source_user_id,source_provider,source_provider_user_id,token_aad_row_id,client_id,token_ciphertext,token_nonce,encryption_key_id) VALUES($1,'account_deletion',$2,'apple',$3,$4,$5,$6,$7,$8) ON CONFLICT(idempotency_key) DO NOTHING`, idempotency, userID, g.subject, g.id, g.clientID, g.ciphertext, g.nonce, g.keyID); err != nil {
			return err
		}
	}
	if len(grants) > 0 {
		if _, err = tx.Exec(ctx, `DELETE FROM apple_grants WHERE user_id=$1 AND provider='apple'`, userID); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}

// UnlinkIdentity locks every sign-in method, consumes recent-auth, and either
// removes a grantless identity or durably queues Apple revocation in one serializable transaction.
func (s *Store) UnlinkIdentity(ctx context.Context, userID int64, tokenVersion int32, sessionHash []byte, provider, recentProof string) (pending bool, err error) {
	for attempt := 0; attempt < 3; attempt++ {
		pending, err = s.unlinkIdentityOnce(ctx, userID, tokenVersion, sessionHash, provider, recentProof)
		if !pgerr.IsSerializationFailure(err) {
			return pending, err
		}
	}
	return false, err
}

func (s *Store) unlinkIdentityOnce(ctx context.Context, userID int64, tokenVersion int32, sessionHash []byte, provider, recentProof string) (pending bool, err error) {
	tx, err := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.Serializable})
	if err != nil {
		return false, err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	var hasPassword bool
	if err = tx.QueryRow(ctx, `SELECT password_hash IS NOT NULL FROM users WHERE id=$1 FOR UPDATE`, userID).Scan(&hasPassword); err != nil {
		return false, err
	}
	if len(sessionHash) != sha256.Size {
		return false, ErrSession
	}
	tag, err := tx.Exec(ctx, `UPDATE recent_auth_proofs SET consumed_at=now() WHERE proof_hash=$1 AND user_id=$2 AND token_version=$3 AND session_hash=$4 AND consumed_at IS NULL AND expires_at>now()`, proofDigest(recentProof), userID, tokenVersion, sessionHash)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() != 1 {
		return false, fmt.Errorf("%w", ErrSession)
	}
	rows, err := tx.Query(ctx, `SELECT provider,provider_user_id,status FROM user_identities WHERE user_id=$1 ORDER BY provider,provider_user_id FOR UPDATE`, userID)
	if err != nil {
		return false, err
	}
	activeProviders := map[string]bool{}
	found := false
	stateConflict := false
	for rows.Next() {
		var p, subject, st string
		if err = rows.Scan(&p, &subject, &st); err != nil {
			rows.Close()
			return false, err
		}
		if st == "active" {
			activeProviders[p] = true
		}
		if p == provider {
			found = true
			stateConflict = stateConflict || st != "active"
		}
	}
	rows.Close()
	if err = rows.Err(); err != nil {
		return false, err
	}
	if !found {
		return false, pgx.ErrNoRows
	}
	if stateConflict {
		return false, ErrState
	}
	if !hasPassword && len(activeProviders) <= 1 {
		return false, ErrLastMethod
	}
	if provider != "apple" {
		_, err = tx.Exec(ctx, `DELETE FROM user_identities WHERE user_id=$1 AND provider=$2`, userID, provider)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return false, err
	}
	type appleGrant struct {
		id                       uuid.UUID
		subject, clientID, keyID string
		ciphertext, nonce        []byte
	}
	grantRows, err := tx.Query(ctx, `SELECT id,provider_user_id,client_id,refresh_token_ciphertext,refresh_token_nonce,encryption_key_id FROM apple_grants WHERE user_id=$1 AND provider='apple' ORDER BY provider_user_id FOR UPDATE`, userID)
	if err != nil {
		return false, err
	}
	var grants []appleGrant
	for grantRows.Next() {
		var grant appleGrant
		if err = grantRows.Scan(&grant.id, &grant.subject, &grant.clientID, &grant.ciphertext, &grant.nonce, &grant.keyID); err != nil {
			grantRows.Close()
			return false, err
		}
		grants = append(grants, grant)
	}
	grantRows.Close()
	if err = grantRows.Err(); err != nil {
		return false, err
	}
	if len(grants) == 0 {
		_, err = tx.Exec(ctx, `DELETE FROM user_identities WHERE user_id=$1 AND provider='apple'`, userID)
		if err == nil {
			err = tx.Commit(ctx)
		}
		return false, err
	}
	_, err = tx.Exec(ctx, `DELETE FROM user_identities ui WHERE ui.user_id=$1 AND ui.provider='apple' AND NOT EXISTS (SELECT 1 FROM apple_grants ag WHERE ag.user_id=ui.user_id AND ag.provider=ui.provider AND ag.provider_user_id=ui.provider_user_id)`, userID)
	if err != nil {
		return false, err
	}
	_, err = tx.Exec(ctx, `UPDATE user_identities SET status='revocation_pending',status_changed_at=now() WHERE user_id=$1 AND provider='apple'`, userID)
	if err != nil {
		return false, err
	}
	for _, grant := range grants {
		idempotency := "unlink:apple:" + grant.id.String()
		_, err = tx.Exec(ctx, `INSERT INTO apple_revocation_jobs(idempotency_key,reason,source_user_id,source_provider,source_provider_user_id,token_aad_row_id,client_id,token_ciphertext,token_nonce,encryption_key_id) VALUES($1,'unlink',$2,'apple',$3,$4,$5,$6,$7,$8) ON CONFLICT(idempotency_key) DO NOTHING`, idempotency, userID, grant.subject, grant.id, grant.clientID, grant.ciphertext, grant.nonce, grant.keyID)
		if err != nil {
			return false, err
		}
		if _, err = tx.Exec(ctx, `UPDATE apple_grants SET revocation_requested_at=now() WHERE id=$1`, grant.id); err != nil {
			return false, err
		}
	}
	if err == nil {
		err = tx.Commit(ctx)
	}
	return true, err
}
