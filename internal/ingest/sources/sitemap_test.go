package sources

import (
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func gzipBytes(t *testing.T, raw string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write([]byte(raw)); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeSitemapBody(t *testing.T) {
	const urlset = `<urlset><url><loc>https://x.test/a</loc><lastmod>2026-01-02</lastmod></url></urlset>`
	const index = `<sitemapindex><sitemap><loc>https://x.test/s.xml.gz</loc></sitemap></sitemapindex>`
	tests := []struct {
		name    string
		body    []byte
		max     int64
		want    SitemapDoc
		wantErr string
	}{
		{name: "plain urlset", body: []byte(urlset), max: 1 << 20,
			want: SitemapDoc{URLs: []SitemapLoc{{Loc: "https://x.test/a", LastMod: "2026-01-02"}}}},
		{name: "gzip urlset", body: gzipBytes(t, urlset), max: 1 << 20,
			want: SitemapDoc{URLs: []SitemapLoc{{Loc: "https://x.test/a", LastMod: "2026-01-02"}}}},
		{name: "sitemap index", body: []byte(index), max: 1 << 20,
			want: SitemapDoc{Sitemaps: []SitemapLoc{{Loc: "https://x.test/s.xml.gz"}}}},
		{name: "byte-order mark and surrounding space", body: append([]byte("\n\xef\xbb\xbf"), []byte(urlset+" ")...), max: 1 << 20,
			want: SitemapDoc{URLs: []SitemapLoc{{Loc: "https://x.test/a", LastMod: "2026-01-02"}}}},
		{name: "gzip over its cap", body: gzipBytes(t, urlset), max: 8, wantErr: "gzip sitemap exceeds 8 bytes"},
		{name: "other root", body: []byte(`<rss><loc>x</loc></rss>`), max: 1 << 20, wantErr: `sitemap root "rss"`},
		{name: "truncated gzip", body: gzipBytes(t, urlset)[:6], max: 1 << 20, wantErr: "gzip: unexpected EOF"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := decodeSitemapBody(tc.body, tc.max)
			if tc.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("err = %v", err)
			}
			if len(got.URLs) != len(tc.want.URLs) || len(got.Sitemaps) != len(tc.want.Sitemaps) {
				t.Fatalf("got %+v, want %+v", got, tc.want)
			}
			for i := range got.URLs {
				if got.URLs[i] != tc.want.URLs[i] {
					t.Errorf("URLs[%d] = %+v, want %+v", i, got.URLs[i], tc.want.URLs[i])
				}
			}
			for i := range got.Sitemaps {
				if got.Sitemaps[i] != tc.want.Sitemaps[i] {
					t.Errorf("Sitemaps[%d] = %+v, want %+v", i, got.Sitemaps[i], tc.want.Sitemaps[i])
				}
			}
		})
	}
}

func TestFetchSitemap(t *testing.T) {
	const shard = `<urlset><url><loc>https://x.test/a</loc></url></urlset>`
	status := http.StatusOK
	body := gzipBytes(t, shard)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("User-Agent") != "ua-test" {
			t.Errorf("User-Agent = %q, want ua-test", r.Header.Get("User-Agent"))
		}
		w.WriteHeader(status)
		_, _ = w.Write(body)
	}))
	defer srv.Close()
	ctx := context.Background()

	doc, err := FetchSitemap(ctx, srv.Client(), srv.URL+"/s.xml.gz", "ua-test", 1<<20)
	if err != nil {
		t.Fatalf("FetchSitemap: %v", err)
	}
	if len(doc.URLs) != 1 || doc.URLs[0].Loc != "https://x.test/a" {
		t.Errorf("got %+v", doc)
	}

	status = http.StatusForbidden
	if _, err := FetchSitemap(ctx, srv.Client(), srv.URL+"/s.xml.gz", "ua-test", 1<<20); err == nil || !strings.Contains(err.Error(), "status 403") {
		t.Errorf("403: err = %v, want status 403", err)
	}

	status = http.StatusOK
	// A body past the wire cap is refused before it is decoded, not read as a short sitemap.
	if _, err := FetchSitemap(ctx, srv.Client(), srv.URL+"/s.xml.gz", "ua-test", 16); err == nil {
		t.Error("a body past the cap must error")
	}

	// A transport failure is returned as is, so the caller can inspect it.
	if _, err := FetchSitemap(ctx, srv.Client(), "http://127.0.0.1:1/x", "ua-test", 1<<20); err == nil {
		t.Error("an unreachable host must error")
	}
}
