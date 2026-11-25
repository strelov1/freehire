package search

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRebuildPushSendsBoundedNDJSON(t *testing.T) {
	oldLimit := maxPushBytes
	maxPushBytes = 1 // One document per request.
	t.Cleanup(func() { maxPushBytes = oldLimit })

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodPost || req.URL.Path != "/indexes/jobs_rebuild/documents" ||
			!strings.HasPrefix(req.Header.Get("Content-Type"), "application/x-ndjson") ||
			req.URL.Query().Get("primaryKey") != "id" {
			t.Errorf("unexpected document request: %s %s (%s)", req.Method, req.URL, req.Header.Get("Content-Type"))
			http.Error(w, "expected an NDJSON document replacement", http.StatusBadRequest)
			return
		}
		var doc JobDocument
		decoder := json.NewDecoder(req.Body)
		if err := decoder.Decode(&doc); err != nil {
			t.Errorf("decode document: %v", err)
			http.Error(w, "invalid document", http.StatusBadRequest)
			return
		}
		if err := decoder.Decode(new(JobDocument)); err != io.EOF {
			t.Errorf("expected one document per request, got %v after the first document", err)
			http.Error(w, "expected one document", http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		_ = json.NewEncoder(w).Encode(map[string]int64{"taskUid": doc.ID})
	}))
	defer srv.Close()

	client := NewClient(srv.URL, "test-key")
	r := client.NewFacetRebuild()
	r.rebuild = client.manager.Index(r.rebuildUID)
	err := r.Push(t.Context(), []JobDocument{{ID: 11}, {ID: 22}})
	if err != nil {
		t.Fatal(err)
	}
	if len(r.tasks) != 2 || r.tasks[0] != 11 || r.tasks[1] != 22 {
		t.Fatalf("tracked tasks %v, want [11 22]", r.tasks)
	}
}
