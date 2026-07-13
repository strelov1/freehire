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
	codeUniqueViolation     = "23505"
	codeForeignKeyViolation = "23503"
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
