package job

import (
	"testing"

	"github.com/jackc/pgx/v5/pgtype"
)

func TestNew_IsTechThreadedToFieldsAndParams(t *testing.T) {
	tests := []struct {
		name  string
		title string
		want  *bool // nil = unknown
	}{
		{"tech title", "Senior Backend Developer", boolp(true)},
		{"non-tech title", "Warehouse Janitorial Cleaner", boolp(false)},
		{"unknown title", "Yard Coordinator", nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j, err := New(Draft{Source: "src", ExternalID: "ext", Title: tt.title})
			if err != nil {
				t.Fatalf("New: %v", err)
			}
			if got := j.Fields().IsTech; !eqBoolp(got, tt.want) {
				t.Errorf("Fields().IsTech = %v, want %v", got, tt.want)
			}
			// UpsertParams must carry the same signal as a nullable pgtype.Bool.
			wantPg := pgtype.Bool{}
			if tt.want != nil {
				wantPg = pgtype.Bool{Bool: *tt.want, Valid: true}
			}
			if got := j.Fields().UpsertParams().IsTech; got != wantPg {
				t.Errorf("UpsertParams().IsTech = %+v, want %+v", got, wantPg)
			}
		})
	}
}

func boolp(b bool) *bool { return &b }

func eqBoolp(a, b *bool) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}
