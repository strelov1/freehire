package telegram

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestFetcherFetchesAndParses(t *testing.T) {
	page, err := os.ReadFile("testdata/preview.html")
	if err != nil {
		t.Fatal(err)
	}
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write(page)
	}))
	defer srv.Close()

	f := NewFetcher()
	f.baseURL = srv.URL
	f.httpClient = srv.Client() // reach the loopback test server past the SSRF guard

	posts, err := f.Fetch(context.Background(), "hrlunapark")
	if err != nil {
		t.Fatalf("fetch: %v", err)
	}
	if gotPath != "/s/hrlunapark" {
		t.Errorf("path = %q, want /s/hrlunapark", gotPath)
	}
	if len(posts) != 3 {
		t.Errorf("posts = %d, want 3", len(posts))
	}
}

func TestFetcherErrorsOnHTTPFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusTooManyRequests)
	}))
	defer srv.Close()

	f := NewFetcher()
	f.baseURL = srv.URL
	f.httpClient = srv.Client() // reach the loopback test server past the SSRF guard

	if _, err := f.Fetch(context.Background(), "foo"); err == nil {
		t.Fatal("want error on 429, got nil")
	}
}
