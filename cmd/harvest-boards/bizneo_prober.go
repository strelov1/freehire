package main

import (
	"context"
	"errors"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/ingest/sources"
)

// bizneoProber validates a Bizneo career site (the board is the whole host,
// somos-grupo-mas.careers.ats.bizneo.cloud) by reading the first listing page the adapter reads
// and counting its job cards; the site's heading states the total ("Tenemos 19 ofertas …"), which
// is preferred when present. The employer is the og:title of the page. It mirrors
// internal/ingest/sources/bizneo.go, which is unexported.
//
// bizneo.cloud has no wildcard DNS, so an unknown host fails to resolve and counts as a probe
// failure; a switched-off site answers 404. A customer's own domain (emprego.imancorp.pt) serves
// the same pages; such a host counts only when its listing loads Bizneo's career-site bundle,
// since nothing else ties an arbitrary host to the platform.
type bizneoProber struct{}

var (
	bizneoHostPattern  = regexp.MustCompile(`^(?:[a-z0-9](?:[a-z0-9-]*[a-z0-9])?\.)+[a-z]{2,}$`)
	bizneoCloudHost    = regexp.MustCompile(`\.bizneo\.cloud$`)
	bizneoCareerBundle = regexp.MustCompile(`^https://assets\.bizneo\.com/vite/assets/custom-companies`)
	bizneoTotal        = regexp.MustCompile(`(\d+)`)
)

func (bizneoProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	if !bizneoHostPattern.MatchString(board) || board == "bizneo.com" || strings.HasSuffix(board, ".bizneo.com") {
		return "", 0, nil
	}
	root, err := c.GetHTML(ctx, "https://"+board+"/jobs?page=1")
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil
		}
		return "", 0, err
	}
	cards := 0
	total := -1
	bundle := bizneoCloudHost.MatchString(board)
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode {
			if n.Data == "a" && sources.HasClass(n, "job-card") {
				cards++
			}
			if n.Data == "script" && bizneoCareerBundle.MatchString(sources.Attr(n, "src")) {
				bundle = true
			}
			if total < 0 && n.Data == "p" && sources.HasClass(n, "subtitle") {
				if m := bizneoTotal.FindStringSubmatch(bizneoNodeText(n)); m != nil {
					total, _ = strconv.Atoi(m[1])
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
	}
	visit(root)
	if !bundle {
		return "", 0, nil
	}
	if total < 0 {
		total = cards
	}
	return strings.TrimSpace(metaProperty(root, "og:title")), total, nil
}

// discover enumerates candidate sites from Common Crawl's index of *.bizneo.cloud.
func (bizneoProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	return commonCrawlCandidates(ctx, c, "*.bizneo.cloud", recognizedBoard("bizneo"))
}

// bizneoNodeText is the concatenated text under n.
func bizneoNodeText(n *html.Node) string {
	var b strings.Builder
	var visit func(*html.Node)
	visit = func(m *html.Node) {
		if m.Type == html.TextNode {
			b.WriteString(m.Data)
		}
		for ch := m.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
	}
	visit(n)
	return b.String()
}
