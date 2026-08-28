package inbox_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/strelov1/freehire/internal/application/inbox"
)

// fakeIngester records the batch it was handed, so a test can assert on the write that was
// NOT made — which is what "refuse the whole push" means.
type fakeIngester struct {
	batches [][]inbox.IncomingMessage
	result  inbox.IngestResult
	err     error
}

func (f *fakeIngester) IngestBatch(_ context.Context, _ int64, msgs []inbox.IncomingMessage) (inbox.IngestResult, error) {
	f.batches = append(f.batches, msgs)
	return f.result, f.err
}

func one(id string) inbox.IncomingMessage {
	return inbox.IncomingMessage{ExternalID: id, FromAddr: "ats@acme.test", Subject: "Interview",
		ReceivedAt: time.Unix(1_700_000_000, 0).UTC()}
}

// TestIngestRules pins the rules that used to live in a Fiber handler, and are therefore now
// reachable by the in-app assistant — which issues no HTTP request and could not ingest mail
// at all while they lived up there.
func TestIngestRules(t *testing.T) {
	ctx := context.Background()

	t.Run("a good batch reaches the writer once", func(t *testing.T) {
		w := &fakeIngester{result: inbox.IngestResult{Inserted: 2}}
		svc := inbox.New(nil, nil, inbox.WithIngester(w))

		got, err := svc.Ingest(ctx, 42, []inbox.IncomingMessage{one("a"), one("b")})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.Inserted != 2 {
			t.Errorf("Inserted = %d, want the writer's answer 2", got.Inserted)
		}
		if len(w.batches) != 1 || len(w.batches[0]) != 2 {
			t.Errorf("writer saw %+v, want one batch of 2", w.batches)
		}
	})

	t.Run("an empty batch is refused", func(t *testing.T) {
		w := &fakeIngester{}
		svc := inbox.New(nil, nil, inbox.WithIngester(w))

		var refused *inbox.BatchError
		if _, err := svc.Ingest(ctx, 42, nil); !errors.As(err, &refused) {
			t.Fatalf("err = %v, want *BatchError", err)
		}
		if len(w.batches) != 0 {
			t.Error("an empty batch must not reach the writer")
		}
	})

	t.Run("an oversized batch is refused whole, never truncated", func(t *testing.T) {
		// Truncating would make the reported counts a partial truth the caller has to guess at.
		msgs := make([]inbox.IncomingMessage, inbox.MaxIngestBatch+1)
		for i := range msgs {
			msgs[i] = one(strings.Repeat("x", i+1))
		}
		w := &fakeIngester{}
		svc := inbox.New(nil, nil, inbox.WithIngester(w))

		var refused *inbox.BatchError
		if _, err := svc.Ingest(ctx, 42, msgs); !errors.As(err, &refused) {
			t.Fatalf("err = %v, want *BatchError", err)
		}
		if len(w.batches) != 0 {
			t.Error("an oversized batch must not be written at all, let alone in part")
		}
	})

	t.Run("exactly the ceiling is accepted", func(t *testing.T) {
		msgs := make([]inbox.IncomingMessage, inbox.MaxIngestBatch)
		for i := range msgs {
			msgs[i] = one(strings.Repeat("x", i+1))
		}
		svc := inbox.New(nil, nil, inbox.WithIngester(&fakeIngester{}))
		if _, err := svc.Ingest(ctx, 42, msgs); err != nil {
			t.Fatalf("a batch AT the ceiling must be accepted: %v", err)
		}
	})

	t.Run("a message with no deduplication key refuses the whole batch", func(t *testing.T) {
		// The bad message is LAST on purpose: validation runs before any write, so the
		// earlier ones cannot be stored under a refusal.
		w := &fakeIngester{}
		svc := inbox.New(nil, nil, inbox.WithIngester(w))

		var refused *inbox.BatchError
		_, err := svc.Ingest(ctx, 42, []inbox.IncomingMessage{one("a"), {Subject: "no id"}})
		if !errors.As(err, &refused) {
			t.Fatalf("err = %v, want *BatchError", err)
		}
		if !strings.Contains(refused.Error(), "messages[1]") {
			t.Errorf("refusal = %q; it must name WHICH message so the caller can fix it", refused)
		}
		if len(w.batches) != 0 {
			t.Error("the good message must not be stored when a later one is invalid")
		}
	})

	t.Run("no writer wired reports itself unavailable", func(t *testing.T) {
		svc := inbox.New(nil, nil)
		if _, err := svc.Ingest(ctx, 42, []inbox.IncomingMessage{one("a")}); !errors.Is(err, inbox.ErrUnavailable) {
			t.Errorf("err = %v, want ErrUnavailable", err)
		}
	})
}
