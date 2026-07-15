package embed

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"

	"github.com/strelov1/freehire/internal/db"
)

// fakeStore is an in-memory Store. It records batch vs. per-item calls so tests can
// assert the happy path takes one batch call and a failure falls back to per-item.
type fakeStore struct {
	mu sync.Mutex

	pending []Claimed        // claimed FIFO, one wave per Claim up to batch
	jobs    map[int64]db.Job // rows returned by Jobs
	jobErr  map[int64]error  // load error for a job id (e.g. corrupted row)

	indexErr map[int64]error // CompleteOpen error for a job id (single-item path)

	openBatches   [][]int64           // job ids per CompleteOpen call (len>1 = a real batch)
	openVectors   map[int64][]float32 // vectors handed to CompleteOpen, keyed by job id
	closedBatches [][]int64           // job ids per CompleteClosed call
	openDone      []int64             // all job ids CompleteOpen'd
	closedDone    []int64             // all job ids CompleteClosed'd
	failCalls     []failCall
	attempts      map[int64]int // outbox id -> attempts so far
}

type failCall struct {
	outboxID    int64
	maxAttempts int
	msg         string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		jobs: map[int64]db.Job{}, jobErr: map[int64]error{}, indexErr: map[int64]error{},
		attempts: map[int64]int{},
	}
}

func (s *fakeStore) Enqueue(context.Context, string) (int64, error) {
	return int64(len(s.pending)), nil
}

func (s *fakeStore) Claim(_ context.Context, batch, _ int) ([]Claimed, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	n := batch
	if n > len(s.pending) {
		n = len(s.pending)
	}
	wave := s.pending[:n]
	s.pending = s.pending[n:]
	return wave, nil
}

func (s *fakeStore) Jobs(_ context.Context, ids []int64) ([]db.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	// A corrupted row aborts a whole multi-id load (a seq scan hits it); a single-id
	// load surfaces the row's own error so the runner can isolate it.
	for _, id := range ids {
		if err := s.jobErr[id]; err != nil {
			return nil, err
		}
	}
	out := make([]db.Job, 0, len(ids))
	for _, id := range ids {
		if j, ok := s.jobs[id]; ok {
			out = append(out, j)
		}
	}
	return out, nil
}

func (s *fakeStore) CompleteOpen(_ context.Context, entries []Claimed, _ string, vectors map[int64][]float32) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	for _, e := range entries {
		if err := s.indexErr[e.JobID]; err != nil {
			return err
		}
		ids = append(ids, e.JobID)
	}
	s.openBatches = append(s.openBatches, ids)
	s.openDone = append(s.openDone, ids...)
	if s.openVectors == nil {
		s.openVectors = map[int64][]float32{}
	}
	for id, v := range vectors {
		s.openVectors[id] = v
	}
	return nil
}

func (s *fakeStore) CompleteClosed(_ context.Context, entries []Claimed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	for _, e := range entries {
		ids = append(ids, e.JobID)
	}
	s.closedBatches = append(s.closedBatches, ids)
	s.closedDone = append(s.closedDone, ids...)
	return nil
}

func (s *fakeStore) Fail(_ context.Context, outboxID int64, msg string, maxAttempts int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCalls = append(s.failCalls, failCall{outboxID, maxAttempts, msg})
	s.attempts[outboxID]++
	return s.attempts[outboxID] >= maxAttempts, nil
}

// fakeIndexer records IndexOpen/RemoveClosed calls. batchFails makes any multi-job
// IndexOpen fail (to exercise the per-item fallback); indexErr fails a specific job.
type fakeIndexer struct {
	mu         sync.Mutex
	indexCalls [][]int64 // job ids per IndexOpen call
	removeCall [][]int64 // ids per RemoveClosed call
	indexed    []int64
	removed    []int64
	batchFails bool
	indexErr   map[int64]error
}

func newFakeIndexer() *fakeIndexer { return &fakeIndexer{indexErr: map[int64]error{}} }

func (ix *fakeIndexer) IndexOpen(_ context.Context, jobs []db.Job) (map[int64][]float32, error) {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ids := make([]int64, len(jobs))
	vecs := make(map[int64][]float32, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
		vecs[j.ID] = []float32{float32(j.ID)} // deterministic per-job vector
	}
	ix.indexCalls = append(ix.indexCalls, ids)
	if ix.batchFails && len(jobs) > 1 {
		return nil, errors.New("batch embed failed")
	}
	for _, j := range jobs {
		if err := ix.indexErr[j.ID]; err != nil {
			return nil, err
		}
	}
	ix.indexed = append(ix.indexed, ids...)
	return vecs, nil
}

func (ix *fakeIndexer) RemoveClosed(_ context.Context, ids []int64) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	ix.removeCall = append(ix.removeCall, ids)
	ix.removed = append(ix.removed, ids...)
	return nil
}

func opt() RunOptions {
	return RunOptions{TargetModel: "e5-test", BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3}
}

func has(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestRunnerBatchesOpenAndClosed(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
		{OutboxID: 30, JobID: 3, Closed: false},
		{OutboxID: 40, JobID: 4, Closed: true},
		{OutboxID: 50, JobID: 5, Closed: true},
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 3 || stats.Removed != 2 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want indexed=3 removed=2 failed=0", stats)
	}
	// The whole wave of open jobs must be embedded in ONE IndexOpen call and completed
	// in ONE CompleteOpen call — that is the batching (one Meili task per wave).
	if len(ix.indexCalls) != 1 || len(ix.indexCalls[0]) != 3 {
		t.Errorf("IndexOpen calls = %v, want a single 3-job batch", ix.indexCalls)
	}
	if len(store.openBatches) != 1 || len(store.openBatches[0]) != 3 {
		t.Errorf("CompleteOpen batches = %v, want a single 3-entry batch", store.openBatches)
	}
	if len(ix.removeCall) != 1 || len(ix.removeCall[0]) != 2 {
		t.Errorf("RemoveClosed calls = %v, want a single 2-id batch", ix.removeCall)
	}
	if len(store.closedBatches) != 1 || len(store.closedBatches[0]) != 2 {
		t.Errorf("CompleteClosed batches = %v, want a single 2-entry batch", store.closedBatches)
	}
}

func TestRunnerFallsBackToPerItemOnBatchFailure(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	ix.batchFails = true                           // the multi-job batch embed fails
	ix.indexErr[2] = errors.New("job 2 is poison") // and job 2 fails individually too
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
		{OutboxID: 30, JobID: 3, Closed: false},
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Batch failed → per-item retry: jobs 1 and 3 succeed, job 2 fails in isolation.
	if stats.Indexed != 2 || stats.Failed != 1 {
		t.Fatalf("stats = %+v, want indexed=2 failed=1 (only the poison job fails)", stats)
	}
	if !has(ix.indexed, 1) || !has(ix.indexed, 3) {
		t.Errorf("indexed = %v, want jobs 1 and 3 to survive the poison job", ix.indexed)
	}
	if len(store.failCalls) != 1 || store.failCalls[0].outboxID != 20 {
		t.Errorf("failCalls = %+v, want one for outbox 20 (job 2)", store.failCalls)
	}
}

func TestRunnerCorruptedRowDeadLettersInFallback(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	// A corrupted row aborts the batch load; the per-item fallback isolates it and
	// dead-letters it immediately (maxAttempts=1), while the rest embed.
	store.jobErr[2] = &pgconn.PgError{Code: "XX001"}
	store.jobs[1] = db.Job{ID: 1}
	store.jobs[3] = db.Job{ID: 3}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
		{OutboxID: 30, JobID: 3, Closed: false},
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 2 || stats.DeadLettered != 1 {
		t.Fatalf("stats = %+v, want indexed=2 dead=1", stats)
	}
	if len(store.failCalls) != 1 || store.failCalls[0].maxAttempts != 1 {
		t.Errorf("failCalls = %+v, want one with maxAttempts=1 (immediate dead-letter)", store.failCalls)
	}
}

func TestRunnerPersistsVectorsToStore(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	for _, id := range []int64{1, 2} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
	}

	if _, err := (Runner{Store: store, Indexer: ix}).Run(context.Background(), opt()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	// The vectors IndexOpen produced must reach CompleteOpen so they commit to Postgres
	// alongside the provenance stamp (the fake indexer returns [float32(id)] per job).
	if len(store.openVectors) != 2 {
		t.Fatalf("openVectors = %v; want 2 entries", store.openVectors)
	}
	if got := store.openVectors[1]; len(got) != 1 || got[0] != 1 {
		t.Fatalf("vector for job 1 = %v; want [1]", got)
	}
	if got := store.openVectors[2]; len(got) != 1 || got[0] != 2 {
		t.Fatalf("vector for job 2 = %v; want [2]", got)
	}
}
