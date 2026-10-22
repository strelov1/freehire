package config

import (
	"testing"
	"time"
)

func TestLoadEmbed_defaultsAndOverrides(t *testing.T) {
	for _, k := range []string{"EMBED_BATCH_SIZE", "EMBED_LEASE_SECONDS", "EMBED_MAX_ATTEMPTS", "EMBED_CALL_TIMEOUT_SECONDS"} {
		t.Setenv(k, "")
	}
	got, _ := LoadEmbed()
	// LeaseSeconds' own default (300) is floored up to CallTimeout's default (600): a lease
	// shorter than one batch's processing would re-claim it mid-flight.
	if got.BatchSize != 500 || got.LeaseSeconds != 600 || got.MaxAttempts != 3 || got.CallTimeout.Seconds() != 600 {
		t.Errorf("defaults wrong: %+v", got)
	}

	t.Setenv("EMBED_BATCH_SIZE", "1000")
	if got, _ := LoadEmbed(); got.BatchSize != 1000 {
		t.Errorf("batch-size override = %d, want 1000", got.BatchSize)
	}

	// A non-positive batch size is floored to 1 so the claim always makes progress.
	for _, bad := range []string{"0", "-3"} {
		t.Setenv("EMBED_BATCH_SIZE", bad)
		if got, _ := LoadEmbed(); got.BatchSize != 1 {
			t.Errorf("EMBED_BATCH_SIZE=%s clamped to %d, want 1", bad, got.BatchSize)
		}
	}
	t.Setenv("EMBED_BATCH_SIZE", "")

	// The lease is floored to the per-call timeout: a lease shorter than one batch's
	// processing (0 in the extreme) would re-claim it mid-flight and burn the retry budget.
	t.Setenv("EMBED_LEASE_SECONDS", "0")
	t.Setenv("EMBED_CALL_TIMEOUT_SECONDS", "90")
	if got, _ := LoadEmbed(); got.LeaseSeconds != 90 {
		t.Errorf("EMBED_LEASE_SECONDS=0 floored to %d, want 90 (the call timeout)", got.LeaseSeconds)
	}
}

// EMBED_MAX_RUN_SECONDS bounds a run so it exits before the unit's timeout kills it
// mid-wave. Unset is unbounded (the catch-up supervisor relies on that). A value that is
// set but unreadable fails the run instead of quietly meaning "unbounded" — that reading
// is indistinguishable from the knob working until the unit is killed again — and so does
// one that leaves no room for a single wave, which would drain nothing and exit green.
func TestLoadEmbed_maxRun(t *testing.T) {
	t.Setenv("EMBED_CALL_TIMEOUT_SECONDS", "600")
	tests := []struct {
		env     string
		want    time.Duration
		wantErr bool
	}{
		{env: "", want: 0},
		{env: "2700", want: 45 * time.Minute},
		{env: "0", wantErr: true},
		{env: "-1", wantErr: true},
		{env: "45m", wantErr: true},
		{env: "600", wantErr: true}, // not more than one CallTimeout: no wave could ever start
	}
	for _, tt := range tests {
		t.Run("EMBED_MAX_RUN_SECONDS="+tt.env, func(t *testing.T) {
			t.Setenv("EMBED_MAX_RUN_SECONDS", tt.env)
			got, err := LoadEmbed()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("LoadEmbed() = %+v, nil; want an error", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadEmbed(): %v", err)
			}
			if got.MaxRun != tt.want {
				t.Errorf("MaxRun = %s, want %s", got.MaxRun, tt.want)
			}
		})
	}
}
