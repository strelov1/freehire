package main

import (
	"errors"
	"strings"
	"testing"
)

const giB = uint64(1) << 30

func TestGuardDisk_AbortsWhenBelowFloor(t *testing.T) {
	probe := func(string) (uint64, error) { return 10 * giB, nil }

	err := guardDisk("/data/meili", 70, probe)

	if err == nil {
		t.Fatal("expected abort when free (10G) < floor (70G), got nil")
	}
	// The message must name both numbers so an operator can act on it.
	if !strings.Contains(err.Error(), "10") || !strings.Contains(err.Error(), "70") {
		t.Errorf("error should mention free and required GiB, got: %v", err)
	}
}

func TestGuardDisk_ProceedsWhenAtOrAboveFloor(t *testing.T) {
	probe := func(string) (uint64, error) { return 100 * giB, nil }

	if err := guardDisk("/data/meili", 70, probe); err != nil {
		t.Fatalf("expected proceed when free (100G) >= floor (70G), got: %v", err)
	}
}

func TestGuardDisk_DisabledWhenFloorZero(t *testing.T) {
	probe := func(string) (uint64, error) { return 0, nil }

	if err := guardDisk("/data/meili", 0, probe); err != nil {
		t.Fatalf("floor 0 disables the guard, expected nil even at 0 free, got: %v", err)
	}
}

// The guard is a best-effort safety net, not a correctness gate: if it cannot measure
// the dir (e.g. MEILI_DATA_DIR does not exist off the prod host, as in CI/dev), it must
// fail OPEN — skip the guard and let reindex proceed — rather than block reindex
// everywhere it cannot statfs the path.
func TestGuardDisk_SkipsWhenProbeFails(t *testing.T) {
	probe := func(string) (uint64, error) { return 0, errors.New("statfs boom") }

	if err := guardDisk("/data/meili", 70, probe); err != nil {
		t.Fatalf("probe failure must fail open (skip guard), got: %v", err)
	}
}
