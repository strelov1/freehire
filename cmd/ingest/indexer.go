package main

import (
	"context"
	"log"
	"sync"

	"github.com/strelov1/freehire/internal/search"
)

// indexChunkSize is how many documents the indexer buffers before flushing a
// batch to the search engine. Fat batches amortize the per-batch indexing cost on
// a large index (Meilisearch re-indexes per batch), and Meilisearch auto-batches
// consecutive pushes on top. A const for now; promote to config if it needs tuning.
const indexChunkSize = 1000

// pushFunc sends a batch of documents to the live search index.
type pushFunc func(ctx context.Context, docs []search.JobDocument) error

// indexStats tallies a run's incremental indexing for the done-log line.
type indexStats struct {
	Indexed int // docs successfully pushed
	Failed  int // docs in a batch whose push failed
}

// batchIndexer buffers documents from concurrent ingest saves and flushes them to
// the live search index in fixed-size chunks (and once more on Flush at run end).
// It is best-effort: a push error is tallied and logged, never returned, so a
// search-engine outage cannot fail an ingest run — the batch reindex reconciles any
// miss. The push submits documents without awaiting Meilisearch's indexing task
// (see Client.SubmitJobs), so a full chunk does not stall the crawl on indexing.
// Safe for concurrent Add; the lock is dropped across the network push so a flush
// never blocks other savers from buffering.
type batchIndexer struct {
	push      pushFunc
	chunkSize int

	mu    sync.Mutex
	buf   []search.JobDocument
	stats indexStats
}

func newBatchIndexer(push pushFunc, chunkSize int) *batchIndexer {
	return &batchIndexer{push: push, chunkSize: chunkSize}
}

// Add buffers one document, auto-flushing a chunk once the buffer is full.
func (b *batchIndexer) Add(ctx context.Context, doc search.JobDocument) {
	b.mu.Lock()
	b.buf = append(b.buf, doc)
	if len(b.buf) < b.chunkSize {
		b.mu.Unlock()
		return
	}
	batch := b.take()
	b.mu.Unlock()
	b.send(ctx, batch)
}

// Flush pushes whatever remains buffered. Call once after the crawl completes.
func (b *batchIndexer) Flush(ctx context.Context) {
	b.mu.Lock()
	batch := b.take()
	b.mu.Unlock()
	if len(batch) > 0 {
		b.send(ctx, batch)
	}
}

// Stats returns the running totals (safe to read after Flush).
func (b *batchIndexer) Stats() indexStats {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.stats
}

// take detaches the buffered batch. Caller must hold the lock.
func (b *batchIndexer) take() []search.JobDocument {
	batch := b.buf
	b.buf = nil
	return batch
}

// send pushes a batch outside the lock and records the outcome. Errors are
// swallowed (best-effort) after logging.
func (b *batchIndexer) send(ctx context.Context, batch []search.JobDocument) {
	err := b.push(ctx, batch)

	b.mu.Lock()
	if err != nil {
		b.stats.Failed += len(batch)
	} else {
		b.stats.Indexed += len(batch)
	}
	b.mu.Unlock()

	if err != nil {
		log.Printf("ingest: index push of %d docs failed: %v", len(batch), err)
	}
}
