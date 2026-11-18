// Package pgerr classifies PostgreSQL errors by SQLSTATE so callers can branch on a specific
// condition without re-deriving the *pgconn.PgError unwrap. It is the single home for the
// SQLSTATE constants the repositories and the central error handler share.
package pgerr

import (
	"errors"

	"github.com/jackc/pgx/v5/pgconn"
)

// IsPostgresError reports whether err wraps a server-side PostgreSQL error.
// Diagnostic logs use this coarse classification without exposing SQL or messages.
func IsPostgresError(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr)
}

// SQLSTATE codes the app branches on.
const (
	codeUniqueViolation      = "23505"
	codeForeignKeyViolation  = "23503"
	codeSerializationFailure = "40001"
	// codeDeadlockDetected is 40P01: PostgreSQL killed one transaction in a lock cycle. Like
	// 40001 it says nothing was wrong with the work, so run it again.
	codeDeadlockDetected = "40P01"
	// codeDataCorrupted is XX001: a row cannot be read because its on-disk storage is
	// damaged (e.g. "missing chunk number N for toast value").
	codeDataCorrupted = "XX001"
	// codeExclusionViolation is 23P01: an EXCLUDE constraint refused the row. It is NOT
	// 23505, so a caller checking only for a unique violation misses it and returns a 500.
	codeExclusionViolation = "23P01"
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
// It is transient: the same work succeeds on a retry. Treating it as a hard failure silently
// stalled cmd/rollup-views.
func IsDeadlock(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeDeadlockDetected
}

// IsDataCorrupted reports whether err is (or wraps) a Postgres data-corruption error
// (SQLSTATE XX001). Deliberately narrow: this package recognizes the condition, the caller
// decides what to do (internal/platform/worker's resilient scan skips such a row, and only
// XX001 opts a read into that path).
func IsDataCorrupted(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == codeDataCorrupted
}

// ExclusionViolationConstraint reports the name of the violated constraint when err is
// (or wraps) an EXCLUDE-constraint violation. ok is false for anything else.
//
// SQLSTATE 23P01, NOT a unique violation's 23505: a caller reaching for IsUniqueViolation
// will not match it and the refusal reaches the handler unclassified. The first user is
// mentorship's no-double-booking guarantee, where the violation is an ordinary outcome
// that must become a domain error rather than a 500.
//
// There is deliberately no bare IsExclusionViolation: every caller needs to know WHICH
// constraint fired, and an unused one fails the dead-code guard.
func ExclusionViolationConstraint(err error) (name string, ok bool) {
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != codeExclusionViolation {
		return "", false
	}
	return pgErr.ConstraintName, true
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
