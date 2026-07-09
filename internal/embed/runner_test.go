package embed

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/db"
)

// fakeStore is an in-memory Store: a queue of claimable entries drained one wave at a
// time, plus call recorders. Guarded by mu because a wave processes entries concurrently.
type fakeStore struct {
	mu sync.Mutex

	pending  []Claimed        // claimed in FIFO order, one wave per Claim call up to batch
	jobs     map[int64]db.Job // rows returned by Job
	jobErr   map[int64]error  // load error for a job id (e.g. corrupted row)
	failWith map[int64]error  // CompleteOpen/CompleteClosed error for a job id

	openDone   []int64             // job ids CompleteOpen'd
	openStamps map[int64]stampArgs // model+hash stamped per job
	closedDone []int64             // job ids CompleteClosed'd
	failCalls  []failCall          // recorded Fail calls
	attempts   map[int64]int       // outbox id -> attempts so far (dead-letters at maxAttempts)
}

type stampArgs struct {
	model string
	hash  pgtype.Text
}

type failCall struct {
	outboxID    int64
	maxAttempts int
	msg         string
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		jobs: map[int64]db.Job{}, jobErr: map[int64]error{}, failWith: map[int64]error{},
		openStamps: map[int64]stampArgs{}, attempts: map[int64]int{},
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

func (s *fakeStore) Job(_ context.Context, id int64) (db.Job, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.jobErr[id]; err != nil {
		return db.Job{}, err
	}
	return s.jobs[id], nil
}

func (s *fakeStore) CompleteOpen(_ context.Context, entry Claimed, model string, hash pgtype.Text) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.failWith[entry.JobID]; err != nil {
		return err
	}
	s.openDone = append(s.openDone, entry.JobID)
	s.openStamps[entry.JobID] = stampArgs{model, hash}
	return nil
}

func (s *fakeStore) CompleteClosed(_ context.Context, entry Claimed) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.failWith[entry.JobID]; err != nil {
		return err
	}
	s.closedDone = append(s.closedDone, entry.JobID)
	return nil
}

func (s *fakeStore) Fail(_ context.Context, outboxID int64, msg string, maxAttempts int) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.failCalls = append(s.failCalls, failCall{outboxID, maxAttempts, msg})
	s.attempts[outboxID]++
	return s.attempts[outboxID] >= maxAttempts, nil
}

// fakeIndexer records which jobs were embedded vs removed, and can fail specific ids.
type fakeIndexer struct {
	mu        sync.Mutex
	indexed   []int64
	removed   []int64
	indexErr  map[int64]error
	removeErr map[int64]error
}

func newFakeIndexer() *fakeIndexer {
	return &fakeIndexer{indexErr: map[int64]error{}, removeErr: map[int64]error{}}
}

func (ix *fakeIndexer) IndexOpen(_ context.Context, job db.Job) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.indexErr[job.ID]; err != nil {
		return err
	}
	ix.indexed = append(ix.indexed, job.ID)
	return nil
}

func (ix *fakeIndexer) RemoveClosed(_ context.Context, jobID int64) error {
	ix.mu.Lock()
	defer ix.mu.Unlock()
	if err := ix.removeErr[jobID]; err != nil {
		return err
	}
	ix.removed = append(ix.removed, jobID)
	return nil
}

func opt() RunOptions {
	return RunOptions{TargetModel: "e5-test", Concurrency: 2, LeaseSeconds: 300, MaxAttempts: 3}
}

func has(ids []int64, id int64) bool {
	for _, x := range ids {
		if x == id {
			return true
		}
	}
	return false
}

func TestRunnerBranchesOnClosed(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	// Two open jobs (embed + stamp) and one closed job (remove + clear).
	store.jobs[1] = db.Job{ID: 1, ContentHash: pgtype.Text{String: "h1", Valid: true}}
	store.jobs[2] = db.Job{ID: 2} // NULL content_hash → stamp NULL
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
		{OutboxID: 30, JobID: 3, Closed: true},
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 2 || stats.Removed != 1 || stats.Failed != 0 || stats.DeadLettered != 0 {
		t.Fatalf("stats = %+v, want indexed=2 removed=1 failed=0 dead=0", stats)
	}
	if !has(ix.indexed, 1) || !has(ix.indexed, 2) {
		t.Errorf("indexed = %v, want jobs 1 and 2 embedded", ix.indexed)
	}
	if !has(ix.removed, 3) {
		t.Errorf("removed = %v, want job 3 removed", ix.removed)
	}
	if !has(store.openDone, 1) || !has(store.openDone, 2) {
		t.Errorf("openDone = %v, want 1 and 2", store.openDone)
	}
	if !has(store.closedDone, 3) {
		t.Errorf("closedDone = %v, want 3", store.closedDone)
	}
	// The open path must stamp the model and the exact embedded content_hash (NULL stays NULL).
	if s := store.openStamps[1]; s.model != "e5-test" || !s.hash.Valid || s.hash.String != "h1" {
		t.Errorf("job 1 stamp = %+v, want model e5-test / hash h1", s)
	}
	if s := store.openStamps[2]; s.model != "e5-test" || s.hash.Valid {
		t.Errorf("job 2 stamp = %+v, want model e5-test / NULL hash", s)
	}
}

func TestRunnerFailureDoesNotAbort(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	store.jobs[1] = db.Job{ID: 1}
	store.jobs[2] = db.Job{ID: 2}
	ix.indexErr[1] = errors.New("embed backend down") // job 1's embed fails
	store.pending = []Claimed{
		{OutboxID: 10, JobID: 1, Closed: false},
		{OutboxID: 20, JobID: 2, Closed: false},
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Indexed != 1 || stats.Failed != 1 {
		t.Fatalf("stats = %+v, want indexed=1 failed=1 (job 2 still done despite job 1 failing)", stats)
	}
	if !has(ix.indexed, 2) {
		t.Errorf("job 2 not indexed — a sibling failure aborted the wave")
	}
	if len(store.failCalls) != 1 || store.failCalls[0].outboxID != 10 || store.failCalls[0].maxAttempts != opt().MaxAttempts {
		t.Errorf("failCalls = %+v, want one for outbox 10 at maxAttempts=%d", store.failCalls, opt().MaxAttempts)
	}
}

func TestRunnerCorruptedRowDeadLettersImmediately(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	// A corrupted row can never load — it must dead-letter on the first attempt
	// (maxAttempts=1), not burn the whole attempt budget across cron runs.
	store.jobErr[1] = &pgconn.PgError{Code: "XX001"} // data_corrupted
	store.pending = []Claimed{{OutboxID: 10, JobID: 1, Closed: false}}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.DeadLettered != 1 || stats.Indexed != 0 {
		t.Fatalf("stats = %+v, want dead=1 indexed=0", stats)
	}
	if len(store.failCalls) != 1 || store.failCalls[0].maxAttempts != 1 {
		t.Errorf("failCalls = %+v, want one with maxAttempts=1 (immediate dead-letter)", store.failCalls)
	}
}
