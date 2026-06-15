package worker

import "testing"

func TestExitCode(t *testing.T) {
	tests := []struct {
		name         string
		failed       int
		deadLettered int
		want         int
	}{
		{"clean run exits zero", 0, 0, 0},
		{"failures exit non-zero", 1, 0, 1},
		{"dead-letters exit non-zero", 0, 1, 1},
		{"both failures and dead-letters exit non-zero", 3, 2, 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ExitCode(tt.failed, tt.deadLettered); got != tt.want {
				t.Errorf("ExitCode(%d, %d) = %d, want %d", tt.failed, tt.deadLettered, got, tt.want)
			}
		})
	}
}
