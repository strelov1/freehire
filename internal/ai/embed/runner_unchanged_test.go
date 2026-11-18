package embed

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/strelov1/freehire/internal/platform/db"
)

// content_hash moves on fields the embedder never reads — skills, geography, posted_at —
// and on 2026-10-21 three dictionary/geo deploys moved it on ~92k already-embedded jobs,
// every one queued for a TEI re-embed of an unchanged text. A job whose stored input hash
// still matches its text, under the target model, is re-stamped without an embed; any
// other job is embedded exactly as before.
func TestRunnerRestampsAnUnchangedTextWithoutEmbedding(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	text := db.Job{Title: "Go Engineer", Description: "Build things."}
	stamped := func(id int64, model, inputHash string) db.Job {
		j := text
		j.ID = id
		j.SemanticEmbeddedModel = pgtype.Text{String: model, Valid: model != ""}
		j.SemanticEmbeddedInputHash = pgtype.Text{String: inputHash, Valid: inputHash != ""}
		return j
	}
	same := fakeInputHash(text)
	store.jobs[1] = stamped(1, "e5-test", same)         // facets moved, text did not
	store.jobs[2] = stamped(2, "e5-test", "h:old|text") // text changed
	store.jobs[3] = stamped(3, "e5-test", "")           // embedded before the hash existed
	store.jobs[4] = stamped(4, "e5-previous", same)     // model migration
	store.jobs[5] = stamped(5, "", "")                  // never embedded
	for _, id := range []int64{1, 2, 3, 4, 5} {
		store.pending = append(store.pending, Claimed{OutboxID: id * 10, JobID: id})
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Unchanged != 1 || stats.Indexed != 4 {
		t.Fatalf("stats = %+v, want unchanged=1 indexed=4", stats)
	}
	if len(ix.indexCalls) != 1 || has(ix.indexCalls[0], 1) || len(ix.indexCalls[0]) != 4 {
		t.Errorf("IndexOpen calls = %v, want one call with jobs 2-5 and never job 1", ix.indexCalls)
	}
	if len(store.unchangedDone) != 1 || store.unchangedDone[0] != 1 {
		t.Errorf("CompleteUnchanged = %v, want job 1 alone", store.unchangedDone)
	}
	// Every job leaves stamped with the hash of the text it now carries — the embedded ones
	// because that is what the vectors were computed from.
	for id := int64(1); id <= 5; id++ {
		if got := store.stampedInputHashes[id]; got != same {
			t.Errorf("job %d stamped input hash %q, want %q", id, got, same)
		}
	}
}

// The per-item fallback must make the same decision, or a wave that fell back would
// re-embed the unchanged jobs the batch path would have skipped.
func TestRunnerFallbackAlsoSkipsAnUnchangedText(t *testing.T) {
	store := newFakeStore()
	ix := newFakeIndexer()
	for _, id := range []int64{1, 2} {
		j := db.Job{ID: id, Title: "Go Engineer", Description: "Build things.",
			SemanticEmbeddedModel: pgtype.Text{String: "e5-test", Valid: true}}
		if id == 1 {
			j.SemanticEmbeddedInputHash = pgtype.Text{String: fakeInputHash(j), Valid: true}
		}
		store.jobs[id] = j
	}
	// Job 3 is a corrupted row: it aborts the batch load and sends the wave per item.
	store.jobErr[3] = &pgconn.PgError{Code: "XX001"}
	for _, id := range []int64{1, 2, 3} {
		store.pending = append(store.pending, Claimed{OutboxID: id * 10, JobID: id})
	}

	stats, err := Runner{Store: store, Indexer: ix}.Run(context.Background(), opt())
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if stats.Unchanged != 1 || stats.Indexed != 1 || stats.DeadLettered != 1 {
		t.Fatalf("stats = %+v, want unchanged=1 indexed=1 dead=1", stats)
	}
	for _, call := range ix.indexCalls {
		if has(call, 1) {
			t.Errorf("IndexOpen calls = %v, job 1's text is unchanged and must never be embedded", ix.indexCalls)
		}
	}
}
