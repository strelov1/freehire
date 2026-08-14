package pgconv

import (
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
