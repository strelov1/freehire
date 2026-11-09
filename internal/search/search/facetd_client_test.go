package search

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestFacetdClientFacets(t *testing.T) {
	var got map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/facets" || r.Method != http.MethodPost {
			t.Errorf("%s %s", r.Method, r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		_, _ = w.Write([]byte(`{"total":5,"distribution":{"skills":{"go":3}},"stats":{"s":{"min":1,"max":2}}}`))
	}))
	defer srv.Close()
	c := NewFacetdClient(srv.URL+"/", srv.Client())
	a, err := c.Facets(context.Background(), [][]string{{"a = 1", "b = 2"}, {"c = 3"}}, []string{"skills"})
	if err != nil {
		t.Fatal(err)
	}
	if a.Total != 5 || a.Distribution["skills"]["go"] != 3 || a.Stats["s"] != (FacetStat{Min: 1, Max: 2}) {
		t.Fatalf("answer %+v", a)
	}
	b, _ := json.Marshal(got)
	if string(b) != `{"facets":["skills"],"filter":[["a = 1","b = 2"],["c = 3"]]}` {
		t.Fatalf("body %s", b)
	}
}

func TestFacetdClientDistinct(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var in struct {
			Filter any    `json:"filter"`
			Attr   string `json:"attr"`
		}
		_ = json.NewDecoder(r.Body).Decode(&in)
		if r.URL.Path != "/distinct" || in.Attr != "company_slug" || in.Filter != nil {
			t.Errorf("%s %+v", r.URL.Path, in)
		}
		_, _ = w.Write([]byte(`{"count":42}`))
	}))
	defer srv.Close()
	n, err := NewFacetdClient(srv.URL, srv.Client()).DistinctCount(context.Background(), nil, "company_slug")
	if err != nil || n != 42 {
		t.Fatalf("n=%d err=%v", n, err)
	}
}

func TestFacetdClientErrors(t *testing.T) {
	for _, tc := range []struct {
		status int
		want   error
	}{{422, ErrEngineUnsupported}, {503, ErrEngineNotReady}} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(tc.status)
			_, _ = w.Write([]byte(`{"error":"nope"}`))
		}))
		_, err := NewFacetdClient(srv.URL, srv.Client()).Facets(context.Background(), "a = 1", nil)
		if !errors.Is(err, tc.want) {
			t.Errorf("status %d: %v", tc.status, err)
		}
		srv.Close()
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(500) }))
	defer srv.Close()
	_, err := NewFacetdClient(srv.URL, srv.Client()).DistinctCount(context.Background(), nil, "x")
	if err == nil || errors.Is(err, ErrEngineUnsupported) || errors.Is(err, ErrEngineNotReady) {
		t.Fatalf("500: %v", err)
	}
}

func TestFacetdClientDeadline(t *testing.T) {
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { <-release }))
	defer srv.Close()
	defer close(release)
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	_, err := NewFacetdClient(srv.URL, srv.Client()).Facets(ctx, nil, nil)
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("err %v", err)
	}
}

func TestFacetdFilterShapes(t *testing.T) {
	for _, tc := range []struct {
		in   any
		want string
	}{
		{nil, `null`},
		{"a = 1", `"a = 1"`},
		{[]string{"a", "b"}, `["a","b"]`},
		{[][]string{{"a"}, {"b", "c"}}, `[["a"],["b","c"]]`},
		{[]any{"a", []string{"b"}, []any{"c"}}, `["a",["b"],["c"]]`},
	} {
		b, err := facetdFilterJSON(tc.in)
		if err != nil || string(b) != tc.want {
			t.Errorf("%#v: %s %v", tc.in, b, err)
		}
	}
}

func TestFacetdUnsupportedFilterMakesNoRequest(t *testing.T) {
	calls := 0
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) { calls++ }))
	defer srv.Close()
	c := NewFacetdClient(srv.URL, srv.Client())
	for _, f := range []any{42, map[string]any{"a": 1}, []any{1}, []any{[]any{2}}, make(chan int)} {
		if _, err := c.Facets(context.Background(), f, nil); !errors.Is(err, ErrEngineUnsupported) {
			t.Errorf("%#v: %v", f, err)
		}
		if _, err := c.DistinctCount(context.Background(), f, "x"); !errors.Is(err, ErrEngineUnsupported) {
			t.Errorf("%#v: %v", f, err)
		}
	}
	if calls != 0 {
		t.Fatalf("%d requests", calls)
	}
}

func dist(n int, count func(i int) int64) map[string]int64 {
	m := make(map[string]int64, n)
	for i := 0; i < n; i++ {
		m["v"+strconv.Itoa(i)] = count(i)
	}
	return m
}

func TestDiffRawCaseFold(t *testing.T) {
	m := rawFacets{Total: 3, Distribution: map[string]map[string]int64{"skills": {"Go": 2, "GO": 1}}}
	e := rawFacets{Total: 3, Distribution: map[string]map[string]int64{"skills": {"go": 3}}}
	if d := diffRaw(m, e); d != "" {
		t.Fatal(d)
	}
	e.Distribution["skills"]["go"] = 2
	if diffRaw(m, e) == "" {
		t.Fatal("real difference missed")
	}
}

func TestDiffRawCapTie(t *testing.T) {
	// 1100 distinct high counts, then 100 values tied at 1.
	counts := func(i int) int64 {
		if i < maxValuesPerFacet-100 {
			return int64(1000 + i)
		}
		return 1
	}
	m := dist(maxValuesPerFacet, counts)
	e := dist(maxValuesPerFacet, counts)
	// the engine picked different tied values
	for i := maxValuesPerFacet - 100; i < maxValuesPerFacet; i++ {
		delete(e, "v"+strconv.Itoa(i))
		e["w"+strconv.Itoa(i)] = 1
	}
	raw := func(d map[string]int64) rawFacets {
		return rawFacets{Distribution: map[string]map[string]int64{"skills": d}}
	}
	if d := diffRaw(raw(m), raw(e)); d != "" {
		t.Fatalf("tie flagged: %s", d)
	}
	// a difference above the tie is still reported
	e["v5"]++
	if diffRaw(raw(m), raw(e)) == "" {
		t.Fatal("difference above the tie missed")
	}
	e["v5"]--
	// fewer than 1200 values on the engine side is reported
	delete(e, "w1199")
	if diffRaw(raw(m), raw(e)) == "" {
		t.Fatal("short engine list missed")
	}
}

func TestFacetdClientWriteAndReady(t *testing.T) {
	status := http.StatusOK
	var got map[string]json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/write" {
			_ = json.NewDecoder(r.Body).Decode(&got)
		}
		w.WriteHeader(status)
		_, _ = w.Write([]byte(`{"generation":1}`))
	}))
	defer srv.Close()
	c := NewFacetdClient(srv.URL, srv.Client())

	doc := JobDocument{ID: 7}
	doc.Vectors = DocumentVectors{Skills: []float32{0.5}, Desc: [][]float32{{1}}}
	if err := c.Write(context.Background(), FacetdWrite{Upserts: []JobDocument{doc}, Close: []int64{1}, Delete: []int64{2, 3}}); err != nil {
		t.Fatal(err)
	}
	if string(got["close"]) != "[1]" || string(got["delete"]) != "[2,3]" || string(got["duplicate"]) != "null" {
		t.Fatalf("body %v", got)
	}
	if strings.Contains(string(got["upserts"]), "0.5") {
		t.Fatalf("vectors sent: %s", got["upserts"])
	}
	if err := c.Ready(context.Background()); err != nil {
		t.Fatal(err)
	}

	status = http.StatusServiceUnavailable
	if err := c.Write(context.Background(), FacetdWrite{Delete: []int64{1}}); !errors.Is(err, ErrEngineNotReady) {
		t.Fatalf("write 503: %v", err)
	}
	if err := c.Ready(context.Background()); !errors.Is(err, ErrEngineNotReady) {
		t.Fatalf("ready 503: %v", err)
	}
}
