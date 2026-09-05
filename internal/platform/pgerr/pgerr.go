// Package pgerr classifies PostgreSQL errors by SQLSTATE so callers can branch on a
// specific database condition (a unique or foreign-key violation) without each one
// re-deriving the *pgconn.PgError unwrap. It is the single home for the SQLSTATE
// constants the repositories and the central error handler share.
package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// SQLSTATE codes the app branches on.
const (
	codeUniqueViolation      = "23505"
	codeForeignKeyViolation  = "23503"
	codeSerializationFailure = "40001"
	// codeDeadlockDetected is 40P01: PostgreSQL broke a lock cycle by killing one of
	// the transactions in it. Like 40001 it says nothing was wrong with the work —
	// only that two writers wanted the same rows in opposite orders — so the answer is
	// to run it again.
	codeDeadlockDetected = "40P01"
	// codeDataCorrupted is XX001 (data_corrupted): a row cannot be read because its
	// on-disk storage is damaged — most visibly a "missing chunk number N for toast
	// value ..." on a broken TOAST pointer.
	codeDataCorrupted = "XX001"
)

// IsUniqueViolation reports whether err is (or wraps) a unique-constraint violation
// (SQLSTATE 23505) — e.g. an INSERT colliding with an existing row.
func IsUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeUniqueViolation
}

// IsForeignKeyViolation reports whether err is (or wraps) a foreign-key violation
// (SQLSTATE 23503) — e.g. a write referencing a missing parent row.
func IsForeignKeyViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeForeignKeyViolation
}

// IsSerializationFailure reports whether a serializable transaction must be
// retried because PostgreSQL detected a concurrent-update anomaly (SQLSTATE
// 40001).
func IsSerializationFailure(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeSerializationFailure
}

// IsDeadlock reports whether err is (or wraps) a deadlock PostgreSQL resolved by
// aborting this transaction (SQLSTATE 40P01).
//
// It is a transient condition, not a defect in the statement: the same work
// succeeds on a retry once the other writer has moved on. A caller that treats it
// as a hard failure stops doing its job for as long as nobody reads the log — which
// is what happened to cmd/rollup-views, silently, for two days.
func IsDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeDeadlockDetected
}

// IsDataCorrupted reports whether err is (or wraps) a Postgres data-corruption error
// (SQLSTATE XX001). It is deliberately narrow: recognizing the condition is this package's
// job, but deciding what to do about it is the caller's — internal/platform/worker's resilient scan
// is what chooses to skip such a row, and only XX001 opts a read into that path, so every
// other failure still surfaces unchanged.
func IsDataCorrupted(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeDataCorrupted
}

// UniqueViolationConstraint reports the name of the violated constraint when err is
// (or wraps) a unique-constraint violation, so a caller with more than one UNIQUE on
// the same table can map each to its own sentinel error instead of conflating them —
// e.g. saved_searches' name uniqueness vs. its "at most one profile-derived row per
// user" partial index. ok is false for anything else, including a non-unique pg error.
func UniqueViolationConstraint(err error) (name string, ok bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != codeUniqueViolation {
		return "", false
	}
	return pgErr.ConstraintName, true
}
