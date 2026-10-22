package main

import "testing"

// Unset means the default; a value that is set and unreadable fails the run rather than
// quietly running at the default width, and a zero or negative width would never advance.
func TestChunkSize(t *testing.T) {
	tests := []struct {
		env     string
		want    int64
		wantErr bool
	}{
		{env: "", want: defaultChunkSize},
		{env: "2000000", want: 2_000_000},
		{env: "0", wantErr: true},
		{env: "-1", wantErr: true},
		{env: "1e6", wantErr: true},
	}
	for _, tt := range tests {
		t.Run("BACKFILL_SEMANTIC_INPUT_HASH_CHUNK="+tt.env, func(t *testing.T) {
			t.Setenv("BACKFILL_SEMANTIC_INPUT_HASH_CHUNK", tt.env)
			got, err := chunkSize()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("chunkSize() with %q = %d, nil; want an error", tt.env, got)
				}
				return
			}
			if err != nil || got != tt.want {
				t.Fatalf("chunkSize() with %q = %d, %v; want %d", tt.env, got, err, tt.want)
			}
		})
	}
}
