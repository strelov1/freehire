package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/strelov1/freehire/internal/search/search"
)

func docsOf(ids ...int64) []search.JobDocument {
	out := make([]search.JobDocument, len(ids))
	for i, id := range ids {
		out[i] = search.JobDocument{ID: id}
	}
	return out
}

func TestTeeSendsSameBatchesToBoth(t *testing.T) {
	m, f := &fakeRebuilder{}, &fakeRebuilder{}
	tee := &teeRebuild{primary: m, secondary: f}
	ctx := context.Background()
	_ = tee.Prepare(ctx)
	_ = tee.Push(ctx, docsOf(1, 2))
	_ = tee.Push(ctx, docsOf(3))
	if err := tee.Promote(ctx); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(m.pushed, f.pushed) || !reflect.DeepEqual(m.calls, f.calls) {
		t.Errorf("meili %v %v, facetd %v %v", m.calls, m.pushed, f.calls, f.pushed)
	}
}

type failingRebuilder struct {
	fakeRebuilder
	failAt string
}

func (f *failingRebuilder) Push(ctx context.Context, d []search.JobDocument) error {
	_ = f.fakeRebuilder.Push(ctx, d)
	if f.failAt == "push" {
		return errors.New("facetd down")
	}
	return nil
}

func TestTeeFacetdFailureDoesNotFailMeili(t *testing.T) {
	m, f := &fakeRebuilder{}, &failingRebuilder{failAt: "push"}
	tee := &teeRebuild{primary: m, secondary: f}
	ctx := context.Background()
	_ = tee.Prepare(ctx)
	for _, b := range [][]int64{{1}, {2}, {3}} {
		if err := tee.Push(ctx, docsOf(b...)); err != nil {
			t.Fatalf("push failed Meili's run: %v", err)
		}
	}
	if err := tee.Promote(ctx); err != nil {
		t.Fatal(err)
	}
	if len(m.pushed) != 3 || m.calls[len(m.calls)-1] != "promote" {
		t.Errorf("meili = %v %v", m.calls, m.pushed)
	}
	// facetd got the first failing push, was aborted, and was skipped afterwards
	want := []string{"prepare", "push", "cleanup"}
	if !reflect.DeepEqual(f.calls, want) {
		t.Errorf("facetd calls = %v, want %v", f.calls, want)
	}
}

func TestTeePrimaryFailureStillAbortsBoth(t *testing.T) {
	m, f := &fakeRebuilder{pushErr: errors.New("meili boom")}, &fakeRebuilder{}
	tee := &teeRebuild{primary: m, secondary: f}
	ctx := context.Background()
	_ = tee.Prepare(ctx)
	if err := tee.Push(ctx, docsOf(1)); err == nil {
		t.Fatal("meili failure must surface")
	}
	_ = tee.Cleanup(ctx)
	if f.calls[len(f.calls)-1] != "cleanup" {
		t.Errorf("facetd not aborted: %v", f.calls)
	}
}

func TestFacetdRebuildWire(t *testing.T) {
	var paths []string
	var pushed struct{ Docs []map[string]any }
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		paths = append(paths, r.URL.Path)
		if r.URL.Path == "/rebuild/push" {
			b, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(b, &pushed)
		}
		if r.URL.Path == "/rebuild/commit" {
			http.Error(w, "boom", http.StatusInternalServerError)
		}
	}))
	defer srv.Close()
	f := newFacetdRebuild(srv.URL + "/")
	ctx := context.Background()
	if err := f.Prepare(ctx); err != nil {
		t.Fatal(err)
	}
	docs := docsOf(7)
	docs[0].Vectors = search.DocumentVectors{Skills: []float32{0.5}}
	if err := f.Push(ctx, docs); err != nil {
		t.Fatal(err)
	}
	if docs[0].Vectors.Skills == nil {
		t.Error("Push must not clear the caller's vectors (Meilisearch gets the same batch)")
	}
	if err := f.Promote(ctx); err == nil {
		t.Error("500 on commit must be an error")
	}
	_ = f.Cleanup(ctx)
	if !reflect.DeepEqual(paths, []string{"/rebuild/begin", "/rebuild/push", "/rebuild/commit", "/rebuild/abort"}) {
		t.Errorf("paths = %v", paths)
	}
	if len(pushed.Docs) != 1 || pushed.Docs[0]["id"].(float64) != 7 {
		t.Errorf("pushed = %v", pushed)
	}
	if v, _ := pushed.Docs[0]["_vectors"].(map[string]any); v["skills"] != nil {
		t.Errorf("facetd got vectors it does not index: %v", v)
	}
}

func TestRetryFacetdRebuildsTheSecondaryAlone(t *testing.T) {
	f := &fakeRebuilder{}
	tee := &teeRebuild{primary: &fakeRebuilder{}, secondary: f, secondaryDown: true}
	var got rebuilder
	ok := retryFacetd(context.Background(), tee, func(_ context.Context, b rebuilder) error { got = b; return nil })
	if !ok || got != f {
		t.Errorf("retry = %v on %v, want true on the facetd rebuilder", ok, got)
	}
	if retryFacetd(context.Background(), tee, func(context.Context, rebuilder) error { return errors.New("facetd down") }) {
		t.Error("a failed retry reported facetd as caught up")
	}
}
