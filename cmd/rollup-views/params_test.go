package main

import (
	"testing"

	"github.com/strelov1/freehire/internal/application/viewlog"
)

// The batch must come out in the same order every time. It is built by walking two Go
// maps and Go randomises that walk, so before the sort each run locked hundreds of
// thousands of `jobs` rows in a fresh random order inside one long transaction —
// which is what deadlocked against the ingest runs sharing the slot.
//
// This runs the same input many times: an unsorted implementation passes a single
// comparison roughly one time in n!, and fails this.
func TestBuildParamsIsDeterministicallyOrderedByJobID(t *testing.T) {
	counts := map[string]map[string]viewlog.Counts{
		"2026-07-21": {
			"delta": {Total: 4, Page: 4},
			"alpha": {Total: 1, Page: 1},
			"echo":  {Total: 5, Page: 0},
			"bravo": {Total: 2, Page: 2},
		},
		"2026-07-22": {
			"charlie": {Total: 3, Page: 3},
			"alpha":   {Total: 9, Page: 9},
		},
	}
	ids := map[string]int64{
		"alpha": 10, "bravo": 20, "charlie": 30, "delta": 40, "echo": 50,
	}

	want, wantTotal, err := buildParams(counts, ids)
	if err != nil {
		t.Fatal(err)
	}
	if wantTotal != 24 {
		t.Errorf("total = %d, want 24", wantTotal)
	}
	if len(want) != 6 {
		t.Fatalf("got %d params, want 6", len(want))
	}

	for i := 1; i < len(want); i++ {
		if want[i].JobID < want[i-1].JobID {
			t.Fatalf("params are not ascending by job id: %d before %d", want[i-1].JobID, want[i].JobID)
		}
		if want[i].JobID == want[i-1].JobID && want[i].Day.Time.Before(want[i-1].Day.Time) {
			t.Fatalf("params for one job are not ascending by day")
		}
	}

	for run := 0; run < 50; run++ {
		got, _, err := buildParams(counts, ids)
		if err != nil {
			t.Fatal(err)
		}
		for i := range got {
			if got[i] != want[i] {
				t.Fatalf("run %d differs at index %d: %+v vs %+v", run, i, got[i], want[i])
			}
		}
	}
}

// A slug the catalogue no longer knows is skipped, not carried into the batch as a
// zero job id — which would update whatever row happens to have id 0, or none.
func TestBuildParamsDropsUnresolvedSlugs(t *testing.T) {
	got, total, err := buildParams(
		map[string]map[string]viewlog.Counts{
			"2026-07-21": {"known": {Total: 3, Page: 2}, "vanished": {Total: 7, Page: 7}},
		},
		map[string]int64{"known": 11},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].JobID != 11 {
		t.Fatalf("got %+v, want only the known slug", got)
	}
	if got[0].TotalDelta != 3 || got[0].PageDelta != 2 {
		t.Errorf("deltas = %d/%d, want 3/2", got[0].TotalDelta, got[0].PageDelta)
	}
	// The reported figure counts only what was applied.
	if total != 3 {
		t.Errorf("total = %d, want 3 — an unresolved slug applies nothing", total)
	}
}

func TestBuildParamsRejectsAnUnparseableDay(t *testing.T) {
	if _, _, err := buildParams(
		map[string]map[string]viewlog.Counts{"not-a-day": {"s": {Total: 1}}},
		map[string]int64{"s": 1},
	); err == nil {
		t.Error("want an error")
	}
}
