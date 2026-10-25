package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

// onlyfyProber validates an onlyfy site (the <board> of <board>.onlyfy.jobs) by asking its
// listing fragment for one row: the fragment's pager states the board's posting count. The
// listing names no employer, so the first posting's schema.org JobPosting supplies it
// (hiringOrganization) — without it the catalogue row would be named after the site slug. It
// mirrors internal/ingest/sources/onlyfy.go, which is unexported.
type onlyfyProber struct{}

var (
	onlyfyProbeBoard = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	onlyfyProbeTotal = regexp.MustCompile(`#pageInfoTotalCount'\)\.text\((\d+)\)`)
	onlyfyProbeJob   = regexp.MustCompile(`^/job/([a-z0-9]+)$`)
)

func (onlyfyProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.ToLower(strings.TrimSpace(board))
	if !onlyfyProbeBoard.MatchString(board) {
		return "", 0, nil
	}
	site := "https://" + board + ".onlyfy.jobs"
	root, final, err := onlyfyGetResolved(ctx, c, site+"/candidate/job/ajax_list?display_length=1&page=1&sort=date&sort_dir=DESC")
	if err == nil && !strings.HasPrefix(final, site+"/") {
		// A renamed site (and every <board>.jobbase.io) redirects to another site name. That
		// name is the board; this one would store the same site a second time, so it is a
		// skip, and the destination stands as a candidate of its own.
		return "", 0, nil
	}
	if err != nil {
		var se *sources.StatusError
		if errors.As(err, &se) && se.Code == http.StatusNotFound {
			return "", 0, nil // no such site
		}
		return "", 0, err
	}
	total, first := -1, ""
	var visit func(*html.Node)
	visit = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "script" && n.FirstChild != nil {
			if m := onlyfyProbeTotal.FindStringSubmatch(n.FirstChild.Data); m != nil {
				total, _ = strconv.Atoi(m[1])
			}
		}
		if n.Type == html.ElementNode && n.Data == "a" && first == "" {
			if m := onlyfyProbeJob.FindStringSubmatch(sources.Attr(n, "href")); m != nil {
				first = m[1]
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			visit(ch)
		}
	}
	visit(root)
	if total < 0 {
		return "", 0, fmt.Errorf("onlyfy probe: %s: listing states no posting count", board)
	}
	if total == 0 || first == "" {
		return "", 0, nil
	}
	detail, err := c.GetHTML(ctx, site+"/job/"+first)
	if err != nil {
		return "", 0, err
	}
	var posting struct {
		HiringOrganization struct {
			Name string `json:"name"`
		} `json:"hiringOrganization"`
	}
	sources.LDJobPosting(detail, &posting)
	return strings.TrimSpace(posting.HiringOrganization.Name), total, nil
}

// onlyfyGetResolved fetches a page and reports the URL it landed on after redirects. A client
// that cannot say (no test fake needs to) reports the requested URL.
func onlyfyGetResolved(ctx context.Context, c httpClient, url string) (*html.Node, string, error) {
	if r, ok := c.(sources.HTMLResolvedGetter); ok {
		return r.GetHTMLResolved(ctx, url)
	}
	root, err := c.GetHTML(ctx, url)
	return root, url, err
}

// discover enumerates candidate sites from Common Crawl's index of both domains.
func (onlyfyProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	var out []string
	for _, pattern := range []string{"*.onlyfy.jobs", "*.jobbase.io"} {
		boards, err := commonCrawlCandidates(ctx, c, pattern, recognizedBoard("onlyfy"))
		if err != nil {
			return nil, err
		}
		out = append(out, boards...)
	}
	return out, nil
}
