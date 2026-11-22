package pgconv

import (
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestBoolPtr(t *testing.T) {
	if got := BoolPtr(pgtype.Bool{}); got != nil {
		t.Errorf("BoolPtr(invalid) = %v, want nil", got)
	}
	got := BoolPtr(pgtype.Bool{Bool: true, Valid: true})
	if got == nil || *got != true {
		t.Errorf("BoolPtr(true) = %v, want pointer to true", got)
	}
}

func TestInt8Ptr(t *testing.T) {
	if got := Int8Ptr(pgtype.Int8{}); got != nil {
		t.Errorf("Int8Ptr(invalid) = %v, want nil", got)
	}
	got := Int8Ptr(pgtype.Int8{Int64: 42, Valid: true})
	if got == nil || *got != 42 {
		t.Errorf("Int8Ptr(42) = %v, want pointer to 42", got)
	}
}

func TestInt8(t *testing.T) {
	if got := Int8(nil); got.Valid {
		t.Errorf("Int8(nil) = %+v, want invalid", got)
	}
	n := int64(7)
	got := Int8(&n)
	if !got.Valid || got.Int64 != 7 {
		t.Errorf("Int8(&7) = %+v, want valid 7", got)
	}
}

func TestDurationPtr(t *testing.T) {
	if got := DurationPtr(pgtype.Time{Valid: false}); got != nil {
		t.Errorf("DurationPtr(invalid) = %v, want nil", got)
	}
	want := 9*time.Hour + 30*time.Minute
	got := DurationPtr(pgtype.Time{Microseconds: int64(want / time.Microsecond), Valid: true})
	if got == nil || *got != want {
		t.Errorf("DurationPtr(09:30) = %v, want %v", got, want)
	}
}

func TestDuration(t *testing.T) {
	if got := Duration(nil); got.Valid {
		t.Errorf("Duration(nil) = %+v, want invalid (NULL)", got)
	}
	want := 9*time.Hour + 30*time.Minute
	got := Duration(&want)
	if !got.Valid || got.Microseconds != int64(want/time.Microsecond) {
		t.Errorf("Duration(9h30m) = %+v, want valid time at 09:30", got)
	}
}

func TestInstantAndTextValidAreNeverNull(t *testing.T) {
	if got := Instant(time.Time{}); !got.Valid {
		t.Error("Instant of the zero time must still be a value")
	}
	if got := TextValid(""); !got.Valid || got.String != "" {
		t.Errorf("TextValid(\"\") = %+v, want a valid empty string", got)
	}
}

func TestTextFromPtr(t *testing.T) {
	if got := TextFromPtr(nil); got.Valid {
		t.Errorf("TextFromPtr(nil) = %+v, want NULL", got)
	}
	empty := ""
	if got := TextFromPtr(&empty); !got.Valid || got.String != "" {
		t.Errorf("TextFromPtr(\"\") = %+v, want a valid empty string", got)
	}
}

func TestInt4(t *testing.T) {
	if got := Int4(nil); got.Valid {
		t.Errorf("Int4(nil) = %+v, want invalid", got)
	}
	n := 7
	if got := Int4(&n); !got.Valid || got.Int32 != 7 {
		t.Errorf("Int4(&7) = %+v, want valid 7", got)
	}
	// A value outside int32 would wrap into an unrelated number; it is unknown, not that.
	for _, v := range []int{math.MaxInt32 + 1, math.MinInt32 - 1} {
		if got := Int4(&v); got.Valid {
			t.Errorf("Int4(&%d) = %+v, want invalid", v, got)
		}
	}
}
