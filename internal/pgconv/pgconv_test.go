package pgconv

import (
	"testing"

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
