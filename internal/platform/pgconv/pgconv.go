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

// IntPtr maps a nullable DB int to an optional int: an invalid (NULL) value becomes
// nil, a valid one a pointer to its value. The read-side inverse of Int4.
func IntPtr(n pgtype.Int4) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int32)
	return &v
}

// Int8 maps an optional int64 to the pgtype the generated params expect: nil becomes
// the zero (NULL) value, a non-nil pointer a valid int64. The write-side adapter for a
// nullable bigint FK such as boards.submitted_by.
func Int8(n *int64) pgtype.Int8 {
	if n == nil {
		return pgtype.Int8{}
	}
	return pgtype.Int8{Int64: *n, Valid: true}
}

// Int8Ptr maps a nullable DB bigint to an optional int64: an invalid (NULL) value
// becomes nil, a valid one a pointer to its value. The read-side inverse of Int8.
func Int8Ptr(n pgtype.Int8) *int64 {
	if !n.Valid {
		return nil
	}
	v := n.Int64
	return &v
}

// Bool maps an optional bool to the pgtype the generated params expect: nil becomes
// the zero (NULL) value, a non-nil pointer a valid bool. It is the write-side
// adapter for a tri-state (true/false/NULL) column such as jobs.is_tech.
func Bool(b *bool) pgtype.Bool {
	if b == nil {
		return pgtype.Bool{}
	}
	return pgtype.Bool{Bool: *b, Valid: true}
}

// BoolPtr maps a nullable DB bool to an optional bool: an invalid (NULL) value becomes
// nil, a valid one a pointer to its value. The read-side inverse of Bool.
func BoolPtr(b pgtype.Bool) *bool {
	if !b.Valid {
		return nil
	}
	v := b.Bool
	return &v
}

// Text maps a string to the pgtype the generated params expect, treating the empty
// string as NULL. It is the write-side adapter for a nullable text column whose domain
// zero value is "" — e.g. link_contributions.source/board, unset on a review-queue row.
func Text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// TextPtr maps a nullable DB text to a *string, preserving NULL as nil. Use this where the
// distinction survives to the caller — a JSON response, where NULL must render as null and an
// empty value as "" — and TextString where the domain has no use for it. Same shape as TimePtr
// and IntPtr.
func TextPtr(t pgtype.Text) *string {
	if !t.Valid {
		return nil
	}
	return &t.String
}

// TextString maps a nullable DB text to a plain string: NULL becomes "", a valid value
// its content. The read-side inverse of Text, and lossy by design: a caller that needs to
// tell NULL from "" wants TextPtr.
func TextString(t pgtype.Text) string {
	if !t.Valid {
		return ""
	}
	return t.String
}

// Float4Ptr maps a nullable DB real to an optional float32: NULL becomes nil, a
// valid value a pointer to its content — the float32 analogue of TextPtr/IntPtr.
func Float4Ptr(f pgtype.Float4) *float32 {
	if !f.Valid {
		return nil
	}
	v := f.Float32
	return &v
}

// Duration maps an optional time-of-day to the pgtype a `time` column expects: nil
// becomes the zero (NULL) value, a non-nil pointer a valid time stored as
// microseconds since midnight (what the duration already counts from).
func Duration(d *time.Duration) pgtype.Time {
	if d == nil {
		return pgtype.Time{}
	}
	return pgtype.Time{Microseconds: int64(*d / time.Microsecond), Valid: true}
}

// DurationPtr maps a nullable DB `time` column to an optional time-of-day: an
// invalid (NULL) value becomes nil, a valid one a pointer to its value as a
// Duration since midnight. The read-side inverse of Duration.
func DurationPtr(t pgtype.Time) *time.Duration {
	if !t.Valid {
		return nil
	}
	d := time.Duration(t.Microseconds) * time.Microsecond
	return &d
}
