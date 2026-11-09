package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/platform/db"
	"github.com/strelov1/freehire/internal/search/search"
	"github.com/strelov1/freehire/internal/search/searchdrain"
)

type fakeQueue struct {
	mu  sync.Mutex
	ids []int64
	err error
}

func (q *fakeQueue) Enqueue(_ context.Context, ids []int64) error {
	q.mu.Lock()
	defer q.mu.Unlock()
	q.ids = append(q.ids, ids...)
	return q.err
}

// fakeFacetd records /write bodies and answers with a settable status.
func fakeFacetd(t *testing.T, status *int, delay time.Duration) (*search.FacetdClient, *[]map[string]json.RawMessage, func()) {
	t.Helper()
	var mu sync.Mutex
	var bodies []map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/write" {
			var b map[string]json.RawMessage
			_ = json.NewDecoder(r.Body).Decode(&b)
			mu.Lock()
			bodies = append(bodies, b)
			mu.Unlock()
			time.Sleep(delay)
		}
		w.WriteHeader(*status)
		_, _ = w.Write([]byte(`{"generation":1}`))
	}))
	return search.NewFacetdClient(srv.URL, srv.Client()), &bodies, srv.Close
}

func TestMirrorSendsWaveAndQueuesNothing(t *testing.T) {
	status := 200
	c, bodies, stop := fakeFacetd(t, &status, 0)
	defer stop()
	q := &fakeQueue{}
	m := &facetdMirror{client: c, queue: q, timeout: time.Second}

	m.Apply(context.Background(), search.FacetdWrite{Upserts: []search.JobDocument{{ID: 1}}, Close: []int64{2}, Duplicate: []int64{3}, Delete: []int64{4}})

	if len(*bodies) != 1 || string((*bodies)[0]["close"]) != "[2]" || string((*bodies)[0]["duplicate"]) != "[3]" || string((*bodies)[0]["delete"]) != "[4]" {
		t.Fatalf("bodies %v", *bodies)
	}
	if len(q.ids) != 0 || m.failures.Load() != 0 {
		t.Fatalf("queued %v failures %d", q.ids, m.failures.Load())
	}
}

func TestMirrorQueuesIdsWhenFacetdFailsOrTimesOut(t *testing.T) {
	for name, tc := range map[string]struct {
		status int
		delay  time.Duration
	}{"not ready": {503, 0}, "server error": {500, 0}, "timeout": {200, 300 * time.Millisecond}} {
		t.Run(name, func(t *testing.T) {
			status := tc.status
			c, _, stop := fakeFacetd(t, &status, tc.delay)
			defer stop()
			q := &fakeQueue{}
			m := &facetdMirror{client: c, queue: q, timeout: 50 * time.Millisecond}

			start := time.Now()
			m.Apply(context.Background(), search.FacetdWrite{Upserts: []search.JobDocument{{ID: 1}}, Close: []int64{2}, Delete: []int64{4}})

			if time.Since(start) > 250*time.Millisecond {
				t.Fatalf("Apply took %s, must be bounded by the timeout", time.Since(start))
			}
			slices.Sort(q.ids)
			if !slices.Equal(q.ids, []int64{1, 2, 4}) || m.failures.Load() != 1 {
				t.Fatalf("queued %v failures %d", q.ids, m.failures.Load())
			}
		})
	}
}

func TestMirrorUnsetMakesNoCalls(t *testing.T) {
	var m *facetdMirror // FACETD_URL unset
	m.Apply(context.Background(), search.FacetdWrite{Delete: []int64{1}})

	status := 200
	c, bodies, stop := fakeFacetd(t, &status, 0)
	defer stop()
	(&facetdMirror{client: c, queue: &fakeQueue{}, timeout: time.Second}).Apply(context.Background(), search.FacetdWrite{})
	if len(*bodies) != 0 {
		t.Fatalf("an empty wave made a call: %v", *bodies)
	}
}

func TestMirrorEnqueueFailureDoesNotPanicOrFail(t *testing.T) {
	status := 500
	c, _, stop := fakeFacetd(t, &status, 0)
	defer stop()
	m := &facetdMirror{client: c, queue: &fakeQueue{err: errors.New("db down")}, timeout: time.Second}
	m.Apply(context.Background(), search.FacetdWrite{Delete: []int64{1}})
}

func TestClassifyForFacetd(t *testing.T) {
	ts := pgtype.Timestamptz{Valid: true, Time: time.Now()}
	dup := pgtype.Int8{Valid: true, Int64: 99}
	jobs := []db.Job{
		{ID: 1, IsPrivate: true},
		{ID: 2, ClosedAt: ts},
		{ID: 3, ClosedAt: ts, DuplicateOf: dup},
		{ID: 4, DuplicateOf: dup},
		{ID: 5},
	}
	open, w := classifyForFacetd([]int64{1, 2, 3, 4, 5, 6}, jobs)

	if len(open) != 1 || open[0].ID != 5 {
		t.Fatalf("open %v", open)
	}
	check := func(name string, got, want []int64) {
		if !slices.Equal(got, want) {
			t.Errorf("%s = %v, want %v", name, got, want)
		}
	}
	check("delete", w.Delete, []int64{1, 6}) // private and missing
	check("close", w.Close, []int64{2, 3})
	check("duplicate", w.Duplicate, []int64{3, 4})
}

type fakeRetryStore struct {
	queued    []searchdrain.Claimed
	jobs      []db.Job
	completed []int64
	failed    []int64
	claims    int
}

func (s *fakeRetryStore) Claim(_ context.Context, batch, _ int) ([]searchdrain.Claimed, error) {
	s.claims++
	n := min(batch, len(s.queued))
	out := s.queued[:n]
	s.queued = s.queued[n:]
	return out, nil
}
func (s *fakeRetryStore) Jobs(context.Context, []int64) ([]db.Job, error) { return s.jobs, nil }
func (s *fakeRetryStore) Complete(_ context.Context, ids []int64) error {
	s.completed = append(s.completed, ids...)
	return nil
}
func (s *fakeRetryStore) Fail(_ context.Context, ids []int64, _ string, _ int) error {
	s.failed = append(s.failed, ids...)
	return nil
}
func (s *fakeRetryStore) Stats(context.Context) (int64, int64, error) { return 0, 0, nil }

type fakeTarget struct {
	ready  error
	write  error
	writes []search.FacetdWrite
}

func (f *fakeTarget) Ready(context.Context) error { return f.ready }
func (f *fakeTarget) Write(_ context.Context, w search.FacetdWrite) error {
	if f.write != nil {
		return f.write
	}
	f.writes = append(f.writes, w)
	return nil
}

func fakeBuild(_ context.Context, jobs []db.Job) ([]search.JobDocument, []int64, error) {
	var docs []search.JobDocument
	var withheld []int64
	for _, j := range jobs {
		if j.ID == 50 { // pretend job 50 is withheld
			withheld = append(withheld, j.ID)
			continue
		}
		docs = append(docs, search.JobDocument{ID: j.ID})
	}
	return docs, withheld, nil
}

func retryFixture() (*fakeRetryStore, *fakeTarget, facetdRetry) {
	st := &fakeRetryStore{
		queued: []searchdrain.Claimed{{OutboxID: 10, JobID: 5}, {OutboxID: 11, JobID: 6}, {OutboxID: 12, JobID: 50}},
		jobs:   []db.Job{{ID: 5}, {ID: 50}, {ID: 6, ClosedAt: pgtype.Timestamptz{Valid: true, Time: time.Now()}}},
	}
	tg := &fakeTarget{}
	return st, tg, facetdRetry{store: st, facetd: tg, build: fakeBuild, mirror: &facetdMirror{}}
}

func TestRetryPassSendsCurrentStateInOneWrite(t *testing.T) {
	st, tg, r := retryFixture()
	res := r.Run(context.Background(), searchdrain.RunOptions{BatchSize: 10, LeaseSeconds: 1, MaxAttempts: 3})

	if len(tg.writes) != 1 || res.Sent != 3 {
		t.Fatalf("writes %d sent %d", len(tg.writes), res.Sent)
	}
	w := tg.writes[0]
	if len(w.Upserts) != 1 || w.Upserts[0].ID != 5 || !slices.Equal(w.Close, []int64{6}) || !slices.Equal(w.Delete, []int64{50}) {
		t.Fatalf("write %+v", w)
	}
	if !slices.Equal(st.completed, []int64{10, 11, 12}) || len(st.failed) != 0 {
		t.Fatalf("completed %v failed %v", st.completed, st.failed)
	}
}

func TestRetryPassSkipsWhileFacetdNotReady(t *testing.T) {
	st, tg, r := retryFixture()
	tg.ready = search.ErrEngineNotReady
	r.Run(context.Background(), searchdrain.RunOptions{BatchSize: 10})
	if st.claims != 0 || len(tg.writes) != 0 {
		t.Fatalf("claimed %d, wrote %d while not ready", st.claims, len(tg.writes))
	}
}

func TestRetryPassFailsBatchAndStops(t *testing.T) {
	st, tg, r := retryFixture()
	tg.write = errors.New("boom")
	res := r.Run(context.Background(), searchdrain.RunOptions{BatchSize: 2, MaxAttempts: 3})
	if res.Failed != 2 || !slices.Equal(st.failed, []int64{10, 11}) || len(st.completed) != 0 || st.claims != 1 {
		t.Fatalf("res %+v failed %v completed %v claims %d", res, st.failed, st.completed, st.claims)
	}
	if r.mirror.failures.Load() != 1 {
		t.Fatalf("failures %d", r.mirror.failures.Load())
	}
}

func TestRetryPassHonoursStopAndMaxPerRun(t *testing.T) {
	st, tg, r := retryFixture()
	r.Run(context.Background(), searchdrain.RunOptions{BatchSize: 10, Stop: func() bool { return true }})
	if st.claims != 0 {
		t.Fatalf("claimed while stopped")
	}
	r.Run(context.Background(), searchdrain.RunOptions{BatchSize: 1, MaxPerRun: 2})
	if len(tg.writes) != 2 || len(st.queued) != 1 {
		t.Fatalf("writes %d left %d", len(tg.writes), len(st.queued))
	}
}

func TestRenderFacetdMetrics(t *testing.T) {
	got := renderFacetdMetrics(7, 1, 2)
	for _, want := range []string{"freehire_facetd_outbox_depth 7\n", "freehire_facetd_outbox_dead_letters 1\n", "freehire_facetd_write_failures 2\n"} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %s", want, got)
		}
	}
}
