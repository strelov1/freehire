package search

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestNDJSONPartsStayUnderLimitAndKeepEveryDocument(t *testing.T) {
	docs := make([]JobDocument, 50)
	for i := range docs {
		docs[i] = JobDocument{ID: int64(i + 1), Title: strings.Repeat("x", 100)}
	}
	one, _ := json.Marshal(docs[49]) // the largest: two-digit id
	limit := 5*(len(one)+1) + 3      // room for five documents, not six

	parts, err := ndjsonParts(docs, limit)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 10 {
		t.Fatalf("got %d parts, want 10", len(parts))
	}
	var next int64 = 1
	for _, p := range parts {
		if len(p) > limit {
			t.Fatalf("part of %d bytes exceeds limit %d", len(p), limit)
		}
		for _, line := range bytes.Split(bytes.TrimSuffix(p, []byte("\n")), []byte("\n")) {
			var d JobDocument
			if err := json.Unmarshal(line, &d); err != nil {
				t.Fatal(err)
			}
			if d.ID != next {
				t.Fatalf("got document %d, want %d", d.ID, next)
			}
			next++
		}
	}
	if next != 51 {
		t.Fatalf("parts carry %d documents, want 50", next-1)
	}
}

func TestNDJSONPartsGiveAnOversizedDocumentItsOwnPart(t *testing.T) {
	docs := []JobDocument{{ID: 1}, {ID: 2, Title: strings.Repeat("x", 500)}, {ID: 3}}
	parts, err := ndjsonParts(docs, 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(parts) != 3 {
		t.Fatalf("got %d parts, want 3", len(parts))
	}
}

func TestNDJSONPartsOfNothingIsNoRequest(t *testing.T) {
	parts, err := ndjsonParts(nil, 100)
	if err != nil || len(parts) != 0 {
		t.Fatalf("got %d parts, err %v; want none", len(parts), err)
	}
}

// A part that fails to submit must not leave earlier, accepted parts pending: the drain
// retries the wave one entry at a time next, and a part landing after that retry would
// re-add a job the retry had just removed.
func TestIndexJobsAwaitsAcceptedPartsWhenALaterPartFails(t *testing.T) {
	old := maxPushBytes
	maxPushBytes = 1 // one document per part
	t.Cleanup(func() { maxPushBytes = old })

	var mu sync.Mutex
	posts, polls := 0, map[int]int{}
	settled := map[int]bool{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodPost {
			posts++
			if posts == 3 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = io.WriteString(w, `{"message":"boom","code":"internal","type":"internal"}`)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = fmt.Fprintf(w, `{"taskUid":%d,"status":"enqueued","type":"documentAdditionOrUpdate"}`, posts)
			return
		}
		var uid int
		_, _ = fmt.Sscanf(r.URL.Path, "/tasks/%d", &uid)
		polls[uid]++
		status := "processing"
		if polls[uid] > 2 {
			status, settled[uid] = "succeeded", true
		}
		_, _ = fmt.Fprintf(w, `{"uid":%d,"status":%q,"type":"documentAdditionOrUpdate"}`, uid, status)
	}))
	defer srv.Close()

	err := NewClient(srv.URL, "test-key").IndexJobs(t.Context(), []JobDocument{{ID: 1}, {ID: 2}, {ID: 3}})
	if err == nil {
		t.Fatal("a failed submission surfaced no error")
	}
	mu.Lock()
	defer mu.Unlock()
	if !settled[1] || !settled[2] {
		t.Fatalf("returned before the accepted parts settled: settled=%v", settled)
	}
}
