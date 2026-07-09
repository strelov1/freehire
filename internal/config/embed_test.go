package config

import "testing"

func TestLoadEmbed_defaultsAndOverrides(t *testing.T) {
	for _, k := range []string{"EMBED_BATCH_SIZE", "EMBED_LEASE_SECONDS", "EMBED_MAX_ATTEMPTS", "EMBED_CALL_TIMEOUT_SECONDS"} {
		t.Setenv(k, "")
	}
	got := LoadEmbed()
	if got.BatchSize != 500 || got.LeaseSeconds != 300 || got.MaxAttempts != 3 || got.CallTimeout.Seconds() != 300 {
		t.Errorf("defaults wrong: %+v", got)
	}

	t.Setenv("EMBED_BATCH_SIZE", "1000")
	if got := LoadEmbed(); got.BatchSize != 1000 {
		t.Errorf("batch-size override = %d, want 1000", got.BatchSize)
	}

	// A non-positive batch size is floored to 1 so the claim always makes progress.
	for _, bad := range []string{"0", "-3"} {
		t.Setenv("EMBED_BATCH_SIZE", bad)
		if got := LoadEmbed(); got.BatchSize != 1 {
			t.Errorf("EMBED_BATCH_SIZE=%s clamped to %d, want 1", bad, got.BatchSize)
		}
	}
	t.Setenv("EMBED_BATCH_SIZE", "")

	// The lease is floored to the per-call timeout: a lease shorter than one batch's
	// processing (0 in the extreme) would re-claim it mid-flight and burn the retry budget.
	t.Setenv("EMBED_LEASE_SECONDS", "0")
	t.Setenv("EMBED_CALL_TIMEOUT_SECONDS", "90")
	if got := LoadEmbed(); got.LeaseSeconds != 90 {
		t.Errorf("EMBED_LEASE_SECONDS=0 floored to %d, want 90 (the call timeout)", got.LeaseSeconds)
	}
}
