package main

import (
	"testing"
	"time"
)

func TestAgentParamsAreOrderedByJobThenDay(t *testing.T) {
	// The same lock-order rule buildParams documents: a random map order inside one long
	// transaction is a deadlock waiting for an ingest writer.
	d1 := time.Date(2026, 10, 26, 0, 0, 0, 0, time.UTC)
	d2 := d1.AddDate(0, 0, 1)
	params, total := buildAgentParams(map[time.Time]map[int64]int{
		d2: {9: 1, 3: 2},
		d1: {9: 4},
	})
	if total != 7 {
		t.Errorf("total = %d, want 7", total)
	}
	want := []struct {
		id  int64
		day time.Time
	}{{3, d2}, {9, d1}, {9, d2}}
	if len(params) != len(want) {
		t.Fatalf("params = %d, want %d", len(params), len(want))
	}
	for i, w := range want {
		if params[i].JobID != w.id || !params[i].Day.Time.Equal(w.day) {
			t.Errorf("params[%d] = %d/%v, want %d/%v", i, params[i].JobID, params[i].Day.Time, w.id, w.day)
		}
	}
}
