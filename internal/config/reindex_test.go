package config

import "testing"

func TestLoadReindex_Defaults(t *testing.T) {
	t.Setenv("MEILI_DATA_DIR", "")
	t.Setenv("REINDEX_MIN_FREE_GB", "")
	// Cleared explicitly: without this the assertion below depends on the environment
	// the test process inherited, and a developer (or CI job) with REINDEX_DEDUP set
	// would see a failure that says nothing about the code.
	t.Setenv("REINDEX_DEDUP", "")

	r := LoadReindex()

	if r.MeiliDataDir != "/var/lib/freehire/meili" {
		t.Errorf("MeiliDataDir default = %q, want /var/lib/freehire/meili", r.MeiliDataDir)
	}
	if r.MinFreeGB != 70 {
		t.Errorf("MinFreeGB default = %d, want 70", r.MinFreeGB)
	}
	// The default that matters most here: an unconfigured reindex rebuilds the index
	// and nothing else. It used to run the duplicate-marker passes unconditionally,
	// and they grew until the rebuild never happened at all.
	if r.Dedup {
		t.Error("Dedup default = true, want false — the marker passes are opt-in")
	}
}

func TestLoadReindex_FromEnv(t *testing.T) {
	t.Setenv("MEILI_DATA_DIR", "/data/meili")
	t.Setenv("REINDEX_MIN_FREE_GB", "120")

	r := LoadReindex()

	if r.MeiliDataDir != "/data/meili" {
		t.Errorf("MeiliDataDir = %q, want /data/meili", r.MeiliDataDir)
	}
	if r.MinFreeGB != 120 {
		t.Errorf("MinFreeGB = %d, want 120", r.MinFreeGB)
	}
}

// The dedup passes are opt-in, so the env var is the only way to get them.
func TestLoadReindex_DedupOptIn(t *testing.T) {
	for _, tt := range []struct {
		value string
		want  bool
	}{
		{"1", true},
		{"true", true},
		{"0", false},
		{"", false},
		// Anything unparseable falls back to the default rather than enabling a pass
		// that can run for hours.
		{"yes please", false},
	} {
		t.Run("REINDEX_DEDUP="+tt.value, func(t *testing.T) {
			t.Setenv("REINDEX_DEDUP", tt.value)
			if got := LoadReindex().Dedup; got != tt.want {
				t.Errorf("Dedup = %v, want %v", got, tt.want)
			}
		})
	}
}

// A negative floor disables the guard rather than silently inverting the comparison.
func TestLoadReindex_NegativeFloorClampedToZero(t *testing.T) {
	t.Setenv("REINDEX_MIN_FREE_GB", "-5")

	if got := LoadReindex().MinFreeGB; got != 0 {
		t.Errorf("MinFreeGB = %d, want 0 (clamped)", got)
	}
}
