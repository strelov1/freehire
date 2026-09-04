package ingestsched

import (
	"testing"
	"time"
)

// The case that matters most, and the one the whole change turns on: a provider nobody has
// configured is SCHEDULED, on documented defaults. If this returned "not scheduled", the
// configuration table would be the roster again, and "nobody added a row" would once more
// be indistinguishable from "we decided not to crawl this".
func TestEffectiveWithNoOverrideRunsOnDefaults(t *testing.T) {
	got := Effective("greenhouse", nil)

	if got.Provider != "greenhouse" {
		t.Errorf("Provider = %q, want %q", got.Provider, "greenhouse")
	}
	if got.Shards != DefaultShards {
		t.Errorf("Shards = %d, want %d", got.Shards, DefaultShards)
	}
	if got.Cadence != DefaultCadence {
		t.Errorf("Cadence = %v, want %v", got.Cadence, DefaultCadence)
	}
	if got.RunTimeout != DefaultRunTimeout {
		t.Errorf("RunTimeout = %v, want %v", got.RunTimeout, DefaultRunTimeout)
	}
	if !got.Enabled {
		t.Error("Enabled = false, want true — an unconfigured provider is scheduled")
	}
	if got.Overridden {
		t.Error("Overridden = true, want false")
	}
	if got.Managed {
		t.Error("Managed = true, want false — the rollout gate is off until flipped")
	}
}

func TestEffectiveTakesTheOverridesValues(t *testing.T) {
	got := Effective("paylocity", &Override{
		Provider:   "paylocity",
		Shards:     24,
		Cadence:    24 * time.Hour,
		RunTimeout: 4500 * time.Second,
		Enabled:    true,
		Notes:      "~10.42s/board measured; 395 boards per shard needs 4117s",
		Managed:    true,
	})

	if got.Shards != 24 {
		t.Errorf("Shards = %d, want 24", got.Shards)
	}
	if got.Cadence != 24*time.Hour {
		t.Errorf("Cadence = %v, want 24h", got.Cadence)
	}
	if got.RunTimeout != 4500*time.Second {
		t.Errorf("RunTimeout = %v, want 4500s", got.RunTimeout)
	}
	if !got.Overridden {
		t.Error("Overridden = false, want true")
	}
	if !got.Managed {
		t.Error("Managed = false, want true")
	}
	if got.Notes == "" {
		t.Error("Notes were dropped; the measurement is why the number is what it is")
	}
}

// A disabled provider must carry its reason all the way to the report. A disable whose
// reason is lost on the way out is barely better than no reason at all.
func TestEffectiveCarriesTheDisableReason(t *testing.T) {
	const reason = "fingerprint client has no proxy support; hard-403s the prod IP"
	got := Effective("bayt", &Override{
		Provider:       "bayt",
		Shards:         1,
		Cadence:        time.Hour,
		RunTimeout:     DefaultRunTimeout,
		Enabled:        false,
		DisabledReason: reason,
	})

	if got.Enabled {
		t.Error("Enabled = true, want false")
	}
	if got.DisabledReason != reason {
		t.Errorf("DisabledReason = %q, want %q", got.DisabledReason, reason)
	}
}

// Sharding is what makes an oversized provider crawlable, and every consumer of that fact
// asks the same question: which slices exist? Asking Settings, rather than each caller
// looping 1..Shards itself, keeps "unsharded is shard 1 of 1" a single statement.
func TestShardSelectorsCoverEveryShardExactlyOnce(t *testing.T) {
	unsharded := Effective("greenhouse", nil).ShardSelectors()
	if len(unsharded) != 1 {
		t.Fatalf("unsharded selectors = %v, want exactly one", unsharded)
	}
	if unsharded[0].Index != 1 || unsharded[0].Of != 1 {
		t.Errorf("unsharded selector = %d/%d, want 1/1", unsharded[0].Index, unsharded[0].Of)
	}

	sharded := Effective("workday", &Override{
		Provider: "workday", Shards: 6, Cadence: 6 * time.Hour, RunTimeout: DefaultRunTimeout,
		Enabled: true,
	}).ShardSelectors()
	if len(sharded) != 6 {
		t.Fatalf("sharded selectors = %d, want 6", len(sharded))
	}
	for i, s := range sharded {
		if s.Index != i+1 || s.Of != 6 {
			t.Errorf("selector[%d] = %d/%d, want %d/6", i, s.Index, s.Of, i+1)
		}
	}
}
