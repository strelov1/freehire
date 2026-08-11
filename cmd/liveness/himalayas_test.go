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
