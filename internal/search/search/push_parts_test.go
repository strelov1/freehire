package search

import (
	"bytes"
	"encoding/json"
	"strings"
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
