package jobderive

import "testing"

func TestDerive_IsTech(t *testing.T) {
	tests := []struct {
		name string
		in   Input
		want *bool // nil = unknown
	}{
		{
			name: "recognized tech category via title → true",
			in:   Input{Title: "Senior Backend Developer"},
			want: boolp(true),
		},
		{
			name: "blacklist non-tech category via title → false",
			in:   Input{Title: "Sales Manager"},
			want: boolp(false),
		},
		{
			name: "detector-only non-tech title → false",
			in:   Input{Title: "Warehouse Janitorial Cleaner"},
			want: boolp(false),
		},
		{
			name: "unresolved title stays unknown → nil",
			in:   Input{Title: "Yard Coordinator"},
			want: nil,
		},
		{
			name: "tech wins over a non-tech noun in the same title",
			in:   Input{Title: "Backend Engineer, Nurse Scheduling Platform"},
			want: boolp(true),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := Derive(tt.in).IsTech
			if !eqBoolp(got, tt.want) {
				t.Errorf("IsTech = %s, want %s", showBoolp(got), showBoolp(tt.want))
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

func showBoolp(b *bool) string {
	if b == nil {
		return "nil"
	}
	if *b {
		return "true"
	}
	return "false"
}
