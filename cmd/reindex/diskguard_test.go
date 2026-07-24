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

func TestGuardDisk_PropagatesProbeError(t *testing.T) {
	sentinel := errors.New("statfs boom")
	probe := func(string) (uint64, error) { return 0, sentinel }

	if err := guardDisk("/data/meili", 70, probe); !errors.Is(err, sentinel) {
		t.Fatalf("expected probe error to propagate, got: %v", err)
	}
}
