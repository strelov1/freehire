package sources

import (
	"bytes"
	"context"
	"fmt"
	"log"
	"net/url"
	"os"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

// pagedFake serves an endless listing: every page up to a caller-chosen ceiling carries one new
// job link, so a paging loop never reaches a natural end (an empty page) on its own — only a
// maxPages cap can stop it. It exists to exercise the cap-exhaustion path of pagedLinks, as
// opposed to routedHTTP's fixed per-URL routes. HrefPrefix lets a caller match its own
// adapter's link shape (empty defaults to "/job/"); a request whose URL omits the page query
// param entirely (an adapter whose own page-1 URL omits it, e.g. careerplug) is read as page 1.
type pagedFake struct{ HrefPrefix string }

func (f pagedFake) GetHTML(_ context.Context, rawURL string) (*html.Node, error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return nil, err
	}
	page := u.Query().Get("page")
	if page == "" {
		page = "1"
	}
	prefix := f.HrefPrefix
	if prefix == "" {
		prefix = "/job/"
	}
	body := fmt.Sprintf(`<html><body><a href="%s%s">Role</a></body></html>`, prefix, page)
	return html.Parse(strings.NewReader(body))
}

func pagedFakeLinks(base *url.URL, root *html.Node) []string {
	return jobLinks(base, root, func(string) bool { return true })
}

func pagedFakeURL(page int) string {
	return fmt.Sprintf("https://example.com/listing?page=%d", page)
}

// A truncated fullCatalog walk (crawlAllPagedLinks) must never look like a successfully
// exhausted, genuinely small catalogue: reaching maxPages while every page still yields a new
// link means the walk stopped short of the tail, and the caller (a source-scoped unseen sweep)
// relies on an error here to fall back to the safe company-scoped close instead of mass-closing
// everything past the cap.
func TestCrawlAllPagedLinksErrorsWhenCapTruncatesWalk(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	fake := pagedFake{}

	_, err := crawlAllPagedLinks(context.Background(), fake, 3, pagedFakeURL,
		func(root *html.Node) []string { return pagedFakeLinks(base, root) })
	if err == nil {
		t.Fatal("want an error when the cap — not an empty page — ends the walk")
	}
}

// crawlPagedLinks accepts a partial listing by design (a later page failing already ends
// enumeration with what was gathered), so a cap-truncated walk must not fail the crawl — but it
// must still be visible as a log line rather than silently indistinguishable from a genuinely
// small catalogue.
func TestCrawlPagedLinksLogsWhenCapTruncatesWalk(t *testing.T) {
	var buf bytes.Buffer
	log.SetOutput(&buf)
	defer log.SetOutput(os.Stderr)

	base, _ := url.Parse("https://example.com/")
	fake := pagedFake{}

	out, err := crawlPagedLinks(context.Background(), fake, 3, pagedFakeURL,
		func(root *html.Node) []string { return pagedFakeLinks(base, root) })
	if err != nil {
		t.Fatalf("crawlPagedLinks: %v", err)
	}
	if len(out) != 3 {
		t.Fatalf("got %d links, want 3 (the cap's worth, gathered so far)", len(out))
	}
	if !strings.Contains(buf.String(), "cap") {
		t.Errorf("log output = %q, want a warning naming the page cap", buf.String())
	}
}

// Reaching the cap on the SAME page an empty page would otherwise have followed is not a
// truncation: nothing indicates the listing goes deeper, so no error and no log line.
func TestCrawlAllPagedLinksNaturalEndAtCapIsNotTruncation(t *testing.T) {
	base, _ := url.Parse("https://example.com/")
	fake := (&routedHTTP{}).
		route("page=1", `<html><body><a href="/job/1">Role</a></body></html>`).
		route("page=2", `<html><body></body></html>`)

	out, err := crawlAllPagedLinks(context.Background(), fake, 2, pagedFakeURL,
		func(root *html.Node) []string { return pagedFakeLinks(base, root) })
	if err != nil {
		t.Fatalf("crawlAllPagedLinks: %v", err)
	}
	if len(out) != 1 {
		t.Fatalf("got %d links, want 1", len(out))
	}
}
