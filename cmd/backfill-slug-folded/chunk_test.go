package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/platform/worker"
)

// The chunk width is an id RANGE, and on prod the ids are spread over 200x more space
// than there are rows — so this knob is the difference between a pass that finishes
// inside its unit timeout and one that does not.
//
// It used to fall back to the default for anything it could not read. That was wrong for
// the same reason the other one-off passes' knobs are strict: an operator who widens the
// chunk and mistypes it gets a run at the DEFAULT width, printed nowhere, and only the
// wall clock says so hours later. Unset still means the default; a value that is set and
// unreadable now fails the run.
func TestChunkSize(t *testing.T) {
	tests := []struct {
		env     string
		want    int64
		wantErr bool
	}{
		{env: "", want: defaultChunkSize},
		{env: "2000000", want: 2_000_000},
		{env: "1", want: 1},
		// A zero would turn `from += step` into an infinite loop; a negative would walk
		// backwards forever. Neither may be read as "not configured" and quietly ignored.
		{env: "0", wantErr: true},
		{env: "-50000", wantErr: true},
		{env: "lots", wantErr: true},
		{env: "1e6", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("BACKFILL_SLUG_CHUNK="+tt.env, func(t *testing.T) {
			t.Setenv("BACKFILL_SLUG_CHUNK", tt.env)
			got, err := worker.EnvInt64("BACKFILL_SLUG_CHUNK", defaultChunkSize)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("EnvInt64(%q) = %d, nil; want the run to fail rather than take the default", tt.env, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("EnvInt64(%q): %v", tt.env, err)
			}
			if got != tt.want {
				t.Errorf("EnvInt64(%q) = %d, want %d", tt.env, got, tt.want)
			}
		})
	}
}
