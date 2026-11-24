package main

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

// gzipXML gzip-compresses an XML body, mirroring the .xml.gz shape himalayas serves.
func gzipXML(t *testing.T, xml string) []byte {
	t.Helper()
	var buf bytes.Buffer
	w := gzip.NewWriter(&buf)
	if _, err := w.Write([]byte(xml)); err != nil {
		t.Fatalf("gzip write: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("gzip close: %v", err)
	}
	return buf.Bytes()
}

func TestFetchSitemapLiveURLsUnionsAllShards(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, fmt.Sprintf(
			`<sitemapindex><sitemap><loc>%s/shard1.xml.gz</loc></sitemap><sitemap><loc>%s/shard2.xml.gz</loc></sitemap></sitemapindex>`,
			srv.URL, srv.URL)))
	})
	mux.HandleFunc("/shard1.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, `<urlset><url><loc>https://himalayas.app/companies/acme/jobs/a</loc></url></urlset>`))
	})
	mux.HandleFunc("/shard2.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, `<urlset><url><loc>https://himalayas.app/companies/acme/jobs/b</loc></url></urlset>`))
	})

	got, err := fetchSitemapLiveURLs(context.Background(), srv.Client(), srv.URL+"/index.xml.gz")
	if err != nil {
		t.Fatalf("fetchSitemapLiveURLs() err = %v", err)
	}
	want := []string{"https://himalayas.app/companies/acme/jobs/a", "https://himalayas.app/companies/acme/jobs/b"}
	if len(got) != len(want) {
		t.Fatalf("got %d urls, want %d: %v", len(got), len(want), got)
	}
	for _, u := range want {
		if _, ok := got[u]; !ok {
			t.Errorf("missing %s in live set", u)
		}
	}
}

func TestFetchSitemapLiveURLsFailsOnBadShard(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, fmt.Sprintf(
			`<sitemapindex><sitemap><loc>%s/shard1.xml.gz</loc></sitemap><sitemap><loc>%s/missing.xml.gz</loc></sitemap></sitemapindex>`,
			srv.URL, srv.URL)))
	})
	mux.HandleFunc("/shard1.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, `<urlset><url><loc>https://himalayas.app/companies/acme/jobs/a</loc></url></urlset>`))
	})
	// /missing.xml.gz intentionally unregistered: the mux 404s it.

	_, err := fetchSitemapLiveURLs(context.Background(), srv.Client(), srv.URL+"/index.xml.gz")
	if err == nil {
		t.Fatal("fetchSitemapLiveURLs() must error when a shard fetch fails, not return a partial set")
	}
}

func TestFetchSitemapLiveURLsFailsOnEmptyIndex(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, `<sitemapindex></sitemapindex>`))
	})

	_, err := fetchSitemapLiveURLs(context.Background(), srv.Client(), srv.URL+"/index.xml.gz")
	if err == nil {
		t.Fatal("fetchSitemapLiveURLs() must error on an index with no shards")
	}
}

// himalayas serves the index and its shards as gzip FILES. The shared decoder accepts a plain
// body too, and one shard answering plain XML must still be read rather than strike its jobs.
func TestFetchSitemapLiveURLsReadsPlainShard(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, fmt.Sprintf(
			`<sitemapindex><sitemap><loc>%s/shard1.xml</loc></sitemap></sitemapindex>`, srv.URL)))
	})
	mux.HandleFunc("/shard1.xml", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`<urlset><url><loc>https://himalayas.app/companies/acme/jobs/a</loc></url></urlset>`))
	})

	got, err := fetchSitemapLiveURLs(context.Background(), srv.Client(), srv.URL+"/index.xml.gz")
	if err != nil {
		t.Fatalf("fetchSitemapLiveURLs() err = %v", err)
	}
	if _, ok := got["https://himalayas.app/companies/acme/jobs/a"]; !ok {
		t.Errorf("plain shard not read: %v", got)
	}
}

// A 200 carrying something other than a sitemap is not evidence of what is live: it must fail
// the fetch (under-closing) rather than resolve to an empty set.
func TestFetchSitemapLiveURLsFailsOnNonSitemapShard(t *testing.T) {
	mux := http.NewServeMux()
	srv := httptest.NewServer(mux)
	defer srv.Close()

	mux.HandleFunc("/index.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, fmt.Sprintf(
			`<sitemapindex><sitemap><loc>%s/shard1.xml.gz</loc></sitemap></sitemapindex>`, srv.URL)))
	})
	mux.HandleFunc("/shard1.xml.gz", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(gzipXML(t, `<html><body>blocked</body></html>`))
	})

	if _, err := fetchSitemapLiveURLs(context.Background(), srv.Client(), srv.URL+"/index.xml.gz"); err == nil {
		t.Fatal("fetchSitemapLiveURLs() must error on a shard that is not a sitemap")
	}
}
