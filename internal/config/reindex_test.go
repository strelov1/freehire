package config

import "testing"

func TestLoadReindex_Defaults(t *testing.T) {
	t.Setenv("MEILI_DATA_DIR", "")
	t.Setenv("REINDEX_MIN_FREE_GB", "")

	r := LoadReindex()

	if r.MeiliDataDir != "/var/lib/freehire/meili" {
		t.Errorf("MeiliDataDir default = %q, want /var/lib/freehire/meili", r.MeiliDataDir)
	}
	if r.MinFreeGB != 70 {
		t.Errorf("MinFreeGB default = %d, want 70", r.MinFreeGB)
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

// A negative floor disables the guard rather than silently inverting the comparison.
func TestLoadReindex_NegativeFloorClampedToZero(t *testing.T) {
	t.Setenv("REINDEX_MIN_FREE_GB", "-5")

	if got := LoadReindex().MinFreeGB; got != 0 {
		t.Errorf("MinFreeGB = %d, want 0 (clamped)", got)
	}
}
