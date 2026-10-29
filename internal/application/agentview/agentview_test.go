package agentview

import (
	"context"
	"testing"
	"time"
)

type sliceSink struct{ got []Event }

func (s *sliceSink) Write(e Event) { s.got = append(s.got, e) }

func TestRecordCarriesTheScopeAndTheIDs(t *testing.T) {
	sink := &sliceSink{}
	ctx := With(context.Background(), sink, SurfaceMCP, ViewerKey(0, "203.0.113.7", "Claude-User"))
	Record(ctx, KindDetail, []int64{42})
	if len(sink.got) != 1 {
		t.Fatalf("events = %d, want 1", len(sink.got))
	}
	e := sink.got[0]
	if e.Kind != KindDetail || e.Surface != SurfaceMCP || e.Viewer != "ip:203.0.113.7|ua:Claude-User" || len(e.JobIDs) != 1 || e.JobIDs[0] != 42 {
		t.Errorf("event = %+v", e)
	}
	if e.TS.IsZero() || e.TS.Location() != time.UTC {
		t.Errorf("ts = %v, want a UTC timestamp", e.TS)
	}
}

func TestRecordOfNoIDsWritesNothing(t *testing.T) {
	sink := &sliceSink{}
	Record(With(context.Background(), sink, SurfaceMCP, "v"), KindSearch, nil)
	if len(sink.got) != 0 {
		t.Errorf("events = %d, want 0 for an empty result", len(sink.got))
	}
}

func TestASignedInViewerIsKeyedByAccount(t *testing.T) {
	if got := ViewerKey(7, "203.0.113.7", "ua"); got != "user:7" {
		t.Errorf("ViewerKey = %q, want user:7", got)
	}
}

func TestAttachStoresAScopeRecordCanRead(t *testing.T) {
	// Fiber hands us SetUserValue, not a context; a map stands in for fasthttp's store.
	sink := &sliceSink{}
	store := map[any]any{}
	Attach(func(k, v any) { store[k] = v }, sink, SurfaceOJCP, "v")
	ctx := mapContext{context.Background(), store}
	Record(ctx, KindSearch, []int64{1, 2})
	if len(sink.got) != 1 || sink.got[0].Surface != SurfaceOJCP {
		t.Fatalf("events = %+v", sink.got)
	}
}

type mapContext struct {
	context.Context
	m map[any]any
}

func (c mapContext) Value(k any) any { return c.m[k] }

func TestFileNameIsTheEventsUTCDay(t *testing.T) {
	ts := time.Date(2026, 10, 28, 23, 30, 0, 0, time.FixedZone("x", -5*3600)) // 04:30 UTC on the 29th
	if got := FileName(ts); got != "agent-views-2026-10-29.jsonl" {
		t.Errorf("FileName = %q", got)
	}
}
