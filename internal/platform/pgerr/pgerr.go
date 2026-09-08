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
	// codeExclusionViolation is 23P01: an EXCLUDE constraint refused the row. It is NOT
	// 23505 — a caller checking only for a unique violation will miss it entirely, and
	// the refusal will surface as a 500 for what is usually an ordinary outcome.
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

// ExclusionViolationConstraint reports the name of the violated constraint when err is
// (or wraps) an EXCLUDE-constraint violation — a row that overlaps one already there, on
// whatever operator the constraint names. ok is false for anything else.
//
// SQLSTATE 23P01, and that is the trap this exists for: an EXCLUDE constraint is the
// natural way to say "these two cannot overlap", and its violation is NOT a unique
// violation's 23505. A caller reaching for IsUniqueViolation, because that is the
// familiar one, will not match it — and the refusal reaches the handler unclassified.
// The first user is mentorship's no-double-booking guarantee, where the violation is an
// ORDINARY outcome (somebody else took the hour) that must become a domain error rather
// than a 500.
//
// There is deliberately no bare IsExclusionViolation beside it. Every caller so far has
// to know WHICH constraint fired before it can say what happened, so a boolean would be
// an answer nobody can act on — and the dead-code guard caught it as unreachable the
// moment it was written for symmetry with the classifiers above.
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
