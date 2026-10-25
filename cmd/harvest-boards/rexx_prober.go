package main

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/strelov1/freehire/internal/ingest/sources"
	"golang.org/x/net/html"
)

// rexxProber validates a rexx portal (a host, optionally with a path) by walking its listing,
// <base>/stellenangebote.html?start=<offset>, 100 postings a page, and counting the distinct
// posting permalinks (/<slug>-<lang>-j<id>.html). It mirrors internal/ingest/sources/rexx.go,
// which is unexported.
//
// It then reads ONE posting page and requires its schema.org JobPosting: the adapter reads a
// posting from nothing else, and a portal that renders none (voss, measured 2026-10-25) would
// yield an unreadable marker per posting every run. That page also names the employer —
// hiringOrganization, minus the " Jobportal" suffix a third of portals append.
type rexxProber struct{}

var (
	rexxProbeBoard     = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]*[a-z0-9])?(\.[a-z0-9]([a-z0-9-]*[a-z0-9])?)+(/[A-Za-z0-9._-]+)*$`)
	rexxProbePermalink = regexp.MustCompile(`-[a-z]{2,3}-j(\d+)\.html$`)
)

// rexxProbeMaxPages bounds the walk the way the adapter does.
const rexxProbeMaxPages = 100

func (rexxProber) probe(ctx context.Context, c httpClient, board string) (string, int, error) {
	board = strings.TrimSuffix(strings.TrimSpace(board), "/")
	if host, rest, _ := strings.Cut(board, "/"); host != "" {
		board = strings.ToLower(host) + strings.TrimSuffix("/"+rest, "/")
	}
	if !rexxProbeBoard.MatchString(board) {
		return "", 0, nil
	}
	base, err := url.Parse("https://" + board + "/")
	if err != nil {
		return "", 0, nil
	}
	listing := base.JoinPath("stellenangebote.html").String()
	ids := map[string]string{} // id → first permalink
	var first string
	for page := 0; page < rexxProbeMaxPages; page++ {
		root, err := c.GetHTML(ctx, listing+"?start="+strconv.Itoa(page*100))
		if err != nil {
			// No portal at that base answers 404; anything else is surfaced.
			var se *sources.StatusError
			if page == 0 && errors.As(err, &se) && se.Code == http.StatusNotFound {
				return "", 0, nil
			}
			return "", 0, err
		}
		added := 0
		for _, u := range rexxProbeLinks(base, root) {
			id := rexxProbeID(u)
			if _, ok := ids[id]; ok {
				continue
			}
			ids[id] = u
			if first == "" {
				first = u
			}
			added++
		}
		if added == 0 {
			break
		}
	}
	if len(ids) == 0 {
		return "", 0, nil
	}
	root, err := c.GetHTML(ctx, first)
	if err != nil {
		return "", 0, err
	}
	var posting struct {
		HiringOrganization struct {
			Name string `json:"name"`
		} `json:"hiringOrganization"`
	}
	if !sources.LDJobPosting(root, &posting) {
		return "", 0, nil // a portal the adapter cannot read a posting from
	}
	name := strings.TrimSpace(posting.HiringOrganization.Name)
	name = strings.TrimSpace(strings.TrimSuffix(name, "Jobportal"))
	return name, len(ids), nil
}

// rexxProbeLinks returns a listing page's posting permalinks, absolute, without the session id.
func rexxProbeLinks(base *url.URL, root *html.Node) []string {
	var out []string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			if ref, err := url.Parse(sources.Attr(n, "href")); err == nil {
				u := base.ResolveReference(ref)
				u.RawQuery, u.Fragment = "", ""
				if rexxProbeID(u.String()) != "" {
					out = append(out, u.String())
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return out
}

func rexxProbeID(u string) string {
	p, err := url.Parse(u)
	if err != nil {
		return ""
	}
	if m := rexxProbePermalink.FindStringSubmatch(p.Path); m != nil {
		return m[1]
	}
	return ""
}

// discover enumerates candidate portals from Common Crawl's index of both vendor domains.
func (rexxProber) discover(ctx context.Context, c httpClient) ([]string, error) {
	var out []string
	for _, pattern := range []string{"*.rexx-systems.com", "*.rexx-recruitment.com"} {
		boards, err := commonCrawlCandidates(ctx, c, pattern, recognizedBoard("rexx"))
		if err != nil {
			return nil, err
		}
		out = append(out, boards...)
	}
	return out, nil
}
