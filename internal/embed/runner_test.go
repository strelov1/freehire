package embed

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

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

	openBatches   [][]int64                  // job ids per CompleteOpen call (len>1 = a real batch)
	openChunks    map[int64][]ChunkEmbedding // chunks handed to CompleteOpen, keyed by job id
	closedBatches [][]int64                  // job ids per CompleteClosed call
	openDone      []int64                    // all job ids CompleteOpen'd
	closedDone    []int64                    // all job ids CompleteClosed'd
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

func (s *fakeStore) CompleteOpen(_ context.Context, entries []Claimed, _ string, chunks map[int64][]ChunkEmbedding) error {
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
	if s.openChunks == nil {
		s.openChunks = map[int64][]ChunkEmbedding{}
	}
	// "Replace" semantics: a job absent from this call's chunks map (no chunk-worthy
	// description) must end up with NO entry, even if a prior call left one — every
	// entry in this batch clears any stale chunk set before a fresh one (if any) lands.
	for _, e := range entries {
		delete(s.openChunks, e.JobID)
	}
	for id, c := range chunks {
		s.openChunks[id] = c
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

// fakeIndexer records IndexOpen calls. batchFails makes any multi-job IndexOpen fail
// (to exercise the per-item fallback); indexErr fails a specific job.
type fakeIndexer struct {
	mu         sync.Mutex
	indexCalls [][]int64 // job ids per IndexOpen call
	indexed    []int64
	batchFails bool
	indexErr   map[int64]error
	// blockUntilCtxDone makes IndexOpen hang until the call context expires and return
	// its error — simulating a push that is genuinely still working server-side (the
	// index never reported a failure) but outran the caller's CallTimeout.
	blockUntilCtxDone bool
}

func newFakeIndexer() *fakeIndexer { return &fakeIndexer{indexErr: map[int64]error{}} }

func (ix *fakeIndexer) IndexOpen(ctx context.Context, jobs []db.Job) (map[int64][]ChunkEmbedding, error) {
	ix.mu.Lock()
	ids := make([]int64, len(jobs))
	for i, j := range jobs {
		ids[i] = j.ID
	}
	ix.indexCalls = append(ix.indexCalls, ids)
	block := ix.blockUntilCtxDone
	ix.mu.Unlock()

	if block {
		<-ctx.Done()
		return nil, ctx.Err()
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()
	chunks := make(map[int64][]ChunkEmbedding, len(jobs))
	for _, j := range jobs {
		// A deterministic 2-chunk set per job, so tests can assert chunks reach
		// CompleteOpen and are keyed correctly, without a real embedder.
		chunks[j.ID] = []ChunkEmbedding{
			{ChunkIndex: 0, Vector: []float32{float32(j.ID), 0}},
			{ChunkIndex: 1, Vector: []float32{float32(j.ID), 1}},
		}
	}
	if ix.batchFails && len(jobs) > 1 {
		return nil, errors.New("batch embed failed")
	}
	for _, j := range jobs {
		if err := ix.indexErr[j.ID]; err != nil {
			return nil, err
		}
	}
	ix.indexed = append(ix.indexed, ids...)
	return chunks, nil
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
	// Closed jobs have nothing left to compute or embed — CompleteClosed alone (Store)
	// is the whole closed-job side effect, in ONE batch call.
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

// TestRunnerSkipsWaveOnBatchTimeoutInsteadOfCascading mirrors
// internal/searchdrain's identically-named test for the 2026-08-05 prod incident: this
// worker pushes into a Meili index whose cost is a fixed whole-index re-merge, the same
// mechanism that turned one slow-but-fine batch into a cascade of equally slow per-item
// pushes and produced a real outage in the sibling worker. A batch that merely outran
// CallTimeout — with the index still working on it server-side, not actually failed —
// must NOT cascade into per-item fallback here either.
func TestRunnerSkipsWaveOnBatchTimeoutInsteadOfCascading(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	ix.blockUntilCtxDone = true // simulates a push that never returns before the deadline
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
		{OutboxID: 30, JobID: 3, Closed: false},
	}

	opts := opt()
	opts.CallTimeout = 20 * time.Millisecond

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 0 || stats.Failed != 0 || stats.DeadLettered != 0 {
		t.Fatalf("stats = %+v, want all zero — a timed-out batch is skipped this run, not "+
			"fallen back to per-item and not counted as a failure", stats)
	}
	// The one batch attempt, and NOTHING else: no per-item retries.
	if len(ix.indexCalls) != 1 {
		t.Fatalf("IndexOpen calls = %d, want 1 (the batch attempt only) — per-item fallback "+
			"on a mere timeout would multiply the number of equally-expensive index pushes by "+
			"the batch size", len(ix.indexCalls))
	}
	// No attempts burned: the wave stays claimed and becomes retryable once its lease
	// expires, so a later run retries the WHOLE batch fresh rather than spending the
	// entry's limited attempt budget on a timeout that says nothing about the document.
	if len(store.failCalls) != 0 {
		t.Errorf("failCalls = %+v, want none — a timeout must not burn the retry budget", store.failCalls)
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

func TestRunnerMissingJobDeadLettersInFallback(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	// Job 2 was hard-deleted after enqueue: the batch load comes back short, the
	// per-item fallback finds nothing, and the entry dead-letters immediately
	// (maxAttempts=1) instead of eating an attempt per cron run.
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
	if len(store.failCalls) != 1 || store.failCalls[0].outboxID != 20 || store.failCalls[0].maxAttempts != 1 {
		t.Errorf("failCalls = %+v, want one for outbox 20 with maxAttempts=1 (immediate dead-letter)", store.failCalls)
	}
}

// The Indexer's chunk embeddings (job_semantic_chunks' source data) must reach
// CompleteOpen so they commit to Postgres alongside the provenance stamp, sharing
// its transaction and its batch-then-per-item-fallback behavior.
func TestRunnerPersistsChunksToStore(t *testing.T) {
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
	if len(store.openChunks) != 2 {
		t.Fatalf("openChunks = %v; want 2 entries", store.openChunks)
	}
	for _, id := range []int64{1, 2} {
		got := store.openChunks[id]
		if len(got) != 2 {
			t.Fatalf("chunks for job %d = %v; want 2 (fakeIndexer's fixed set)", id, got)
		}
		if got[0].ChunkIndex != 0 || got[1].ChunkIndex != 1 {
			t.Errorf("chunk indices for job %d = %d,%d; want 0,1", id, got[0].ChunkIndex, got[1].ChunkIndex)
		}
	}
}

// A job re-embedded with NO chunks this time (an empty/very short description now)
// must end up with none in the store — CompleteOpen replaces, it never appends, so a
// job can't accumulate a mix of a stale chunk set and nothing new.
func TestRunnerReplacesChunksWithEmptySetWhenIndexerReturnsNone(t *testing.T) {
	store := newFakeStore()
	store.jobs[1] = db.Job{ID: 1}
	store.openChunks = map[int64][]ChunkEmbedding{
		1: {{ChunkIndex: 0, Vector: []float32{9}}}, // simulates a stale chunk from a prior embed
	}
	// Drive processOpenOne directly through the runner's normal per-item path with an
	// indexer stub that returns no chunks for job 1 — bypasses fakeIndexer's fixed
	// 2-chunk fixture entirely.
	store.pending = []Claimed{{OutboxID: 10, JobID: 1, Closed: false}}

	rn := &run{store: store, indexer: emptyChunksIndexer{}, opt: opt()}
	rn.processOpenOne(context.Background(), Claimed{OutboxID: 10, JobID: 1, Closed: false})

	if got, ok := store.openChunks[1]; ok {
		t.Fatalf("openChunks[1] = %v, want no entry (replaced with nothing)", got)
	}
}

// emptyChunksIndexer is a minimal Indexer that never returns any chunks — used to
// exercise CompleteOpen's replace-with-nothing path.
type emptyChunksIndexer struct{}

func (emptyChunksIndexer) IndexOpen(_ context.Context, jobs []db.Job) (map[int64][]ChunkEmbedding, error) {
	return nil, nil
}
