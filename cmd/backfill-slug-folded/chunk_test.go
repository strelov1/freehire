package main

import "testing"

// The chunk width is an id RANGE, and on prod the ids are spread over 200x more space
// than there are rows — so this knob is the difference between a pass that finishes
// inside its unit timeout and one that does not. Everything unparseable falls back to
// the default rather than to zero, which would make the loop never advance.
func TestChunkSize(t *testing.T) {
	tests := []struct {
		env  string
		want int64
	}{
		{"", defaultChunkSize},
		{"2000000", 2_000_000},
		{"1", 1},
		// A zero would turn `from += step` into an infinite loop; a negative would walk
		// backwards forever. Both must read as "not configured".
		{"0", defaultChunkSize},
		{"-50000", defaultChunkSize},
		{"lots", defaultChunkSize},
		{"1e6", defaultChunkSize},
	}
	for _, tt := range tests {
		t.Run("BACKFILL_SLUG_CHUNK="+tt.env, func(t *testing.T) {
			t.Setenv("BACKFILL_SLUG_CHUNK", tt.env)
			if got := chunkSize(); got != tt.want {
				t.Errorf("chunkSize() = %d, want %d", got, tt.want)
			}
		})
	}
}
