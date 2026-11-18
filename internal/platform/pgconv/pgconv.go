// Package pgconv holds the small, pure adapters between Go's optional types (*time.Time,
// *int) and the nullable pgtype values sqlc generates, so the nil<->NULL conversions live
// in one place.
package pgconv

import (
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

// TimePtr maps a nullable DB timestamp to an optional time (NULL becomes nil). It copies the
// time so the pointer does not alias the source struct.
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

// Instant maps a time to a valid (non-NULL) timestamptz, for a column that always has a value.
func Instant(t time.Time) pgtype.Timestamptz {
	return pgtype.Timestamptz{Time: t, Valid: true}
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

// Int8 maps an optional int64 to the pgtype the generated params expect: nil becomes the
// zero (NULL) value, a non-nil pointer a valid int64.
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

// Bool maps an optional bool to the pgtype the generated params expect: nil becomes the
// zero (NULL) value, a non-nil pointer a valid bool (for tri-state columns).
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

// Text maps a string to the pgtype the generated params expect, treating the empty string
// as NULL (for nullable text columns whose domain zero value is "").
func Text(s string) pgtype.Text {
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}

// TextValid maps a string to a non-NULL text, the empty string included. Text is the one for a
// nullable column whose zero value means "none".
func TextValid(s string) pgtype.Text {
	return pgtype.Text{String: s, Valid: true}
}

// TextFromPtr maps an optional string to the pgtype a COALESCE-guarded param expects: nil is
// NULL ("leave the stored value alone"), any non-nil pointer a valid text, "" included.
func TextFromPtr(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return TextValid(*s)
}

// TextPtr maps a nullable DB text to a *string, preserving NULL as nil. Use it where the
// NULL/"" distinction survives to the caller (a JSON response), and TextString where it does
// not.
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

// Date maps an optional calendar date to its nullable database value.
func Date(t *time.Time) pgtype.Date {
	if t == nil {
		return pgtype.Date{}
	}
	return pgtype.Date{Time: *t, Valid: true}
}

// DatePtr maps a nullable calendar date to an optional time without shifting its day.
func DatePtr(d pgtype.Date) *time.Time {
	if !d.Valid {
		return nil
	}
	t := d.Time
	return &t
}
