package embedprogress

import (
	"encoding/json"
	"testing"
	"time"
)

func TestBuild_EverythingMeasured(t *testing.T) {
	snap := &Snapshot{Eligible: 1000, Covered: 600, QueueDepth: ptr(int64(500)), MeasuredAt: t0}
	latest := &Sample{Depth: 300, At: t0.Add(time.Hour)}
	rate := 50.0

	p := Build(snap, latest, &rate)

	if *p.Eligible != 1000 || *p.Covered != 800 || *p.SnapshotCovered != 600 || *p.Queued != 300 {
		t.Errorf("figures = %+v, want eligible 1000, covered 800 (600 + the 200 drained since), snapshot_covered 600, queued 300", p)
	}
	if *p.RatePerMinute != 50 || *p.ETASeconds != 360 {
		t.Errorf("rate/eta = %v/%v, want 50 and 360 (300 / 50 per minute)", *p.RatePerMinute, *p.ETASeconds)
	}
	if !p.SnapshotAt.Equal(t0) || !p.MeasuredAt.Equal(t0.Add(time.Hour)) {
		t.Errorf("times = %v/%v", p.SnapshotAt, p.MeasuredAt)
	}
}

// The queue also carries rows that are not eligible — closed postings waiting to have
// their state cleared, duplicates, private ones — and on production on 2026-10-30 it
// held more rows than there were eligible postings. eligible − queued then read 0% while
// 45% was embedded; moving the measured figure by the queue's change cancels them out.
func TestBuild_QueueLongerThanEligibleStillCountsCoverage(t *testing.T) {
	snap := &Snapshot{Eligible: 934_619, Covered: 423_195, QueueDepth: ptr(int64(1_009_175)), MeasuredAt: t0}
	p := Build(snap, &Sample{Depth: 1_000_000, At: t0.Add(time.Hour)}, nil)
	if *p.Covered != 432_370 {
		t.Errorf("covered = %d, want 432370 (423195 + the 9175 drained since)", *p.Covered)
	}
}

func TestBuild_CoveredIsClamped(t *testing.T) {
	snap := &Snapshot{Eligible: 100, Covered: 10, QueueDepth: ptr(int64(50)), MeasuredAt: t0}
	if p := Build(snap, &Sample{Depth: 200, At: t0}, nil); *p.Covered != 0 {
		t.Errorf("covered = %d after a burst of enqueues, want 0, never negative", *p.Covered)
	}
	if p := Build(snap, &Sample{Depth: 0, At: t0}, nil); *p.Covered != 60 {
		t.Errorf("covered = %d after a full drain, want 60", *p.Covered)
	}
	full := &Snapshot{Eligible: 100, Covered: 95, QueueDepth: ptr(int64(50)), MeasuredAt: t0}
	if p := Build(full, &Sample{Depth: 0, At: t0}, nil); *p.Covered != 100 {
		t.Errorf("covered = %d, want 100, never above eligible", *p.Covered)
	}
}

// A snapshot published before the queue depth was recorded with it has nothing to move
// the figure from, so the measured count stands until the next one.
func TestBuild_SnapshotWithoutQueueDepthUsesTheMeasuredCount(t *testing.T) {
	snap := &Snapshot{Eligible: 1000, Covered: 600, MeasuredAt: t0}
	if p := Build(snap, &Sample{Depth: 300, At: t0}, nil); *p.Covered != 600 {
		t.Errorf("covered = %d, want the snapshot's 600", *p.Covered)
	}
}

func TestBuild_NoETAWithoutAPositiveRate(t *testing.T) {
	zero := 0.0
	p := Build(nil, &Sample{Depth: 10, At: t0}, &zero)
	if p.ETASeconds != nil {
		t.Errorf("eta = %v, want none at rate 0", *p.ETASeconds)
	}
}

// Every unmeasured figure is null on the wire, never 0 — a zero reaching a page reads
// as a measurement.
func TestBuild_UnmeasuredIsNull(t *testing.T) {
	raw, err := json.Marshal(Build(nil, nil, nil))
	if err != nil {
		t.Fatal(err)
	}
	var m map[string]any
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"eligible", "covered", "snapshot_covered", "queued", "rate_per_minute", "eta_seconds", "snapshot_at", "measured_at"} {
		v, present := m[k]
		if !present || v != nil {
			t.Errorf("%s = %v (present %v), want null", k, v, present)
		}
	}
}

func TestBuild_QueueWithoutSnapshot(t *testing.T) {
	p := Build(nil, &Sample{Depth: 42, At: t0}, nil)
	if p.Queued == nil || *p.Queued != 42 {
		t.Errorf("queued = %v, want 42 even with no snapshot", p.Queued)
	}
	if p.Eligible != nil || p.Covered != nil {
		t.Error("eligible/covered set without a snapshot")
	}
}

func ptr[T any](v T) *T { return &v }
