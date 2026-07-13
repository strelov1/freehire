// Package pgconv holds the small, pure adapters between Go's optional types
// (*time.Time, *int) and the nullable pgtype values sqlc generates. Repositories
// map their domain types across the persistence boundary through these helpers so
// the nil<->NULL and pgtype<->Go conversions live in exactly one place instead of
// being re-declared in every package that touches the database.
package pgconv

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TimePtr maps a nullable DB timestamp to an optional time: an invalid (NULL)
// timestamp becomes nil, a valid one a pointer to its value. It copies the time out
// of the pgtype so the pointer does not alias the source struct.
func TimePtr(ts pgtype.Timestamptz) *time.Time {
	if !ts.Valid {
		return nil
	}
	t := ts.Time
	return &t
}

// Timestamptz maps an optional time to the pgtype the generated params expect: nil
// becomes the zero (NULL) value, a non-nil pointer a valid timestamp.
func Timestamptz(t *time.Time) pgtype.Timestamptz {
	if t == nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: *t, Valid: true}
}

// Int4 maps an optional int to the pgtype the generated params expect: nil becomes
// the zero (NULL) value, a non-nil pointer a valid int32.
func Int4(n *int) pgtype.Int4 {
	if n == nil {
		return pgtype.Int4{}
	}
	return pgtype.Int4{Int32: int32(*n), Valid: true}
}
