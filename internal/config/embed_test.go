package config

import "testing"

func TestLoadEmbed_defaultsAndOverrides(t *testing.T) {
	for _, k := range []string{"EMBED_CONCURRENCY", "EMBED_LEASE_SECONDS", "EMBED_MAX_ATTEMPTS", "EMBED_CALL_TIMEOUT_SECONDS"} {
		t.Setenv(k, "")
	}
	got := LoadEmbed()
	if got.Concurrency != 8 || got.LeaseSeconds != 300 || got.MaxAttempts != 3 || got.CallTimeout.Seconds() != 120 {
		t.Errorf("defaults wrong: %+v", got)
	}

	t.Setenv("EMBED_CONCURRENCY", "24")
	if got := LoadEmbed(); got.Concurrency != 24 {
		t.Errorf("concurrency override = %d, want 24", got.Concurrency)
	}

	// A non-positive concurrency is floored to 1 so the claim always makes progress.
	for _, bad := range []string{"0", "-3"} {
		t.Setenv("EMBED_CONCURRENCY", bad)
		if got := LoadEmbed(); got.Concurrency != 1 {
			t.Errorf("EMBED_CONCURRENCY=%s clamped to %d, want 1", bad, got.Concurrency)
		}
	}
	t.Setenv("EMBED_CONCURRENCY", "")

	// The lease is floored to the per-call timeout: a lease shorter than one entry's
	// processing (0 in the extreme) would re-claim it mid-flight and burn the retry budget.
	t.Setenv("EMBED_LEASE_SECONDS", "0")
	t.Setenv("EMBED_CALL_TIMEOUT_SECONDS", "90")
	if got := LoadEmbed(); got.LeaseSeconds != 90 {
		t.Errorf("EMBED_LEASE_SECONDS=0 floored to %d, want 90 (the call timeout)", got.LeaseSeconds)
	}
}
