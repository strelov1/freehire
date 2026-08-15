package searchdrain

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

	completeErr map[int64]error // Complete error for a job id (single-item path)

	completeBatches [][]int64 // job ids per Complete call (len>1 = a real batch)
	completed       []int64   // all job ids Complete'd
	failCalls       []failCall
	attempts        map[int64]int // outbox id -> attempts so far

	reapCalls []int // max_rows per Reap call
	reaped    int64 // rows Reap reports removed
	reapErr   error
}

type failCall struct {
	outboxID    int64
	maxAttempts int
	msg         string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		jobs: map[int64]db.Job{}, jobErr: map[int64]error{}, completeErr: map[int64]error{},
		attempts: map[int64]int{},
	}
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

func (s *fakeStore) Complete(_ context.Context, entries []Claimed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	var ids []int64
	for _, e := range entries {
		if err := s.completeErr[e.JobID]; err != nil {
			return err
		}
		ids = append(ids, e.JobID)
	}
	s.completeBatches = append(s.completeBatches, ids)
	s.completed = append(s.completed, ids...)
	return nil
}

func (s *fakeStore) Fail(_ context.Context, outboxID int64, msg string, maxAttempts int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCalls = append(s.failCalls, failCall{outboxID, maxAttempts, msg})
	s.attempts[outboxID]++
	return s.attempts[outboxID] >= maxAttempts, nil
}

func (s *fakeStore) Reap(_ context.Context, maxRows int) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.reapCalls = append(s.reapCalls, maxRows)
	return s.reaped, s.reapErr
}

func TestRunnerReapsIneligibleEntriesBeforeDraining(t *testing.T) {
	store := newFakeStore()
	store.reaped = 7
	ix := newFakeIndexer()
	seed(store, 1, 2)

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	if len(store.reapCalls) != 1 {
		t.Fatalf("Reap called %d times, want exactly once per run", len(store.reapCalls))
	}
	if stats.Reaped != 7 {
		t.Errorf("stats.Reaped = %d, want 7", stats.Reaped)
	}
	// Reaping first means the claim does not have to scan past rows it would only
	// skip, and the depth gauge stops counting work nobody will ever do.
	if stats.Indexed != 2 {
		t.Errorf("stats.Indexed = %d, want 2 — reaping must not disturb the drain", stats.Indexed)
	}
}

func TestRunnerDrainsEvenWhenReapFails(t *testing.T) {
	store := newFakeStore()
	store.reapErr = errors.New("deadlock detected")
	ix := newFakeIndexer()
	seed(store, 1, 2, 3)

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())

	// Reaping is housekeeping. Letting it fail the run would trade a queue that is
	// merely untidy for one that is not being drained at all.
	if err != nil {
		t.Fatalf("Run returned %v, want the reap failure to be logged and the drain to continue", err)
	}
	if stats.Indexed != 3 {
		t.Errorf("stats.Indexed = %d, want 3", stats.Indexed)
	}
	if stats.Reaped != 0 {
		t.Errorf("stats.Reaped = %d, want 0 when the reap failed", stats.Reaped)
	}
}

// fakeIndexer records IndexBatch calls. batchFails makes any multi-job IndexBatch
// fail (to exercise the per-item fallback); indexErr fails a specific job.
type fakeIndexer struct {
	mu         sync.Mutex
	indexCalls [][]int64 // job ids per IndexBatch call
	indexed    []int64
	batchFails bool
	indexErr   map[int64]error
	// blockUntilCtxDone makes IndexBatch hang until the call context expires and
	// return its error — simulating a push that is genuinely still working server-side
	// (Meilisearch never reported a failure) but outran the caller's CallTimeout.
	blockUntilCtxDone bool
}

func newFakeIndexer() *fakeIndexer { return &fakeIndexer{indexErr: map[int64]error{}} }

func (ix *fakeIndexer) IndexBatch(ctx context.Context, jobs []db.Job) error {
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
		return ctx.Err()
	}

	ix.mu.Lock()
	defer ix.mu.Unlock()
	if ix.batchFails && len(jobs) > 1 {
		return errors.New("batch index failed")
	}
	for _, j := range jobs {
		if err := ix.indexErr[j.ID]; err != nil {
			return err
		}
	}
	ix.indexed = append(ix.indexed, ids...)
	return nil
}

func opt() RunOptions {
	return RunOptions{BatchSize: 500, LeaseSeconds: 300, MaxAttempts: 3}
}

// seed queues one claimable entry per job id, with the job row to match.
func seed(s *fakeStore, ids ...int64) {
	for _, id := range ids {
		s.jobs[id] = db.Job{ID: id}
		s.pending = append(s.pending, Claimed{OutboxID: id * 10, JobID: id})
	}
}

func has(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestRunnerBatchesOneWaveIntoOneIndexCall(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1},
		{OutboxID: 20, JobID: 2},
		{OutboxID: 30, JobID: 3},
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 3 || stats.Failed != 0 {
		t.Fatalf("stats = %+v, want indexed=3 failed=0", stats)
	}
	// The whole wave must be indexed in ONE IndexBatch call and completed in ONE
	// Complete call — that is the fix's point: collapse many per-board pushes into
	// one Meili task per wave.
	if len(ix.indexCalls) != 1 || len(ix.indexCalls[0]) != 3 {
		t.Errorf("IndexBatch calls = %v, want a single 3-job batch", ix.indexCalls)
	}
	if len(store.completeBatches) != 1 || len(store.completeBatches[0]) != 3 {
		t.Errorf("Complete batches = %v, want a single 3-entry batch", store.completeBatches)
	}
}

func TestRunnerDrainsMultipleWavesUntilEmpty(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1},
		{OutboxID: 20, JobID: 2},
		{OutboxID: 30, JobID: 3},
	}

	opts := opt()
	opts.BatchSize = 1 // force three separate waves
	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opts)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 3 {
		t.Fatalf("stats = %+v, want indexed=3", stats)
	}
	if len(ix.indexCalls) != 3 {
		t.Errorf("IndexBatch calls = %v, want 3 (one per wave)", ix.indexCalls)
	}
}

func TestRunnerFallsBackToPerItemOnBatchFailure(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	ix.batchFails = true                           // the multi-job batch push fails
	ix.indexErr[2] = errors.New("job 2 is poison") // and job 2 fails individually too
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1},
		{OutboxID: 20, JobID: 2},
		{OutboxID: 30, JobID: 3},
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

// TestRunnerSkipsWaveOnBatchTimeoutInsteadOfCascading reproduces the 2026-08-05 prod
// incident: on this Meilisearch instance a single-document push costs about as much
// wall-clock time as a whole batch push (both dominated by a fixed whole-index
// re-merge, not by document count). A batch that merely outran CallTimeout — with
// Meilisearch still working on it server-side, not actually failed — must NOT cascade
// into per-item fallback, or one slow-but-fine batch turns into up to BatchSize
// equally slow individual pushes, each competing for the same disk IO that starved
// freehire-web's accept() queue and produced nginx 504s.
func TestRunnerSkipsWaveOnBatchTimeoutInsteadOfCascading(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	ix.blockUntilCtxDone = true // simulates a push that never returns before the deadline
	for _, id := range []int64{1, 2, 3} {
		store.jobs[id] = db.Job{ID: id}
	}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1},
		{OutboxID: 20, JobID: 2},
		{OutboxID: 30, JobID: 3},
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
		t.Fatalf("IndexBatch calls = %d, want 1 (the batch attempt only) — per-item fallback "+
			"on a mere timeout would multiply the number of equally-expensive Meili pushes by "+
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
	// dead-letters it immediately (maxAttempts=1), while the rest index.
	store.jobErr[2] = &pgconn.PgError{Code: "XX001"}
	store.jobs[1] = db.Job{ID: 1}
	store.jobs[3] = db.Job{ID: 3}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1},
		{OutboxID: 20, JobID: 2},
		{OutboxID: 30, JobID: 3},
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
	// Job 2 was hard-deleted (or closed/became a repost, so the claim no longer sees
	// it) after enqueue: the batch load comes back short, the per-item fallback finds
	// nothing, and the entry dead-letters immediately (maxAttempts=1) instead of
	// eating an attempt per cron run.
	store.jobs[1] = db.Job{ID: 1}
	store.jobs[3] = db.Job{ID: 3}
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1},
		{OutboxID: 20, JobID: 2},
		{OutboxID: 30, JobID: 3},
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
