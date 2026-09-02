package sources

import (
	"context"
	"fmt"
	"net/url"
	"path"
	"strings"

	"golang.org/x/net/html"
)

// paycor adapts Paycor Recruiting career sites. The board is the 32-hex clientId that
// addresses one employer's portal, so the listing is
// recruitingbypaycor.com/career/CareerHome.action?clientId=<board> and every posting is
// .../JobIntroduction.action?clientId=<board>&id=<posting>. Both are server-rendered HTML
// with no JSON API behind them, and the listing carries the board's whole open catalogue in
// one page — there is no pagination.
//
// Only the enumeration reads the listing. Tenants pick among several listing templates and
// the markup around a posting's title and location differs between them (one theme wraps the
// whole listing in the employer's own website chrome), but every posting page renders from
// the same labelled cells. So the adapter takes just the links from the listing — the one
// thing every template shares — and reads every field from a per-posting detail fetch
// (bounded-concurrency), like the other HTML detail adapters (jazzhr, betterteam).
//
// The element ids those cells carry are prefixed "gnewton", after the ATS Paycor built this
// on. They are the stable hooks: the CSS classes beside them are what a tenant restyles.
// Paycor states no publication date anywhere on either page, so PostedAt is left nil.
type paycor struct {
	http HTMLGetter
}

// NewPaycor builds the Paycor Recruiting adapter over the given HTTP client.
func NewPaycor(c HTMLGetter) Source { return paycor{http: c} }

func (paycor) Provider() string { return "paycor" }

const (
	paycorCareerBase = "https://recruitingbypaycor.com/career"
	// paycorJobAction is the last path segment of a posting page. Enumeration matches on it
	// rather than on the markup around the link, which varies by listing template.
	paycorJobAction = "JobIntroduction.action"
)

func (s paycor) Fetch(ctx context.Context, e CompanyEntry) ([]Job, error) {
	root, err := s.http.GetHTML(ctx, paycorListingURL(e.Board))
	if err != nil {
		return nil, fmt.Errorf("paycor: listing %s: %w", e.Board, err)
	}

	// Each posting's fields come from its own page fetch, fanned out under a bounded pool.
	return fetchDetails(paycorPostingIDs(root, e.Board), defaultDetailWorkers, func(id string) (Job, bool) {
		return s.detail(ctx, e, id)
	}), nil
}

// detail fetches one posting page and maps it to a Job, returning ok=false when the fetch
// fails or the page is not a live posting, so the caller skips just that posting. A posting
// that has been filled or withdrawn — and an id that never existed — still answers 200, with
// a "no longer active" notice in place of the fields, so the title cell is what says a
// posting is live.
func (s paycor) detail(ctx context.Context, e CompanyEntry, id string) (Job, bool) {
	jobURL := paycorJobURL(e.Board, id)
	root, err := s.http.GetHTML(ctx, jobURL)
	if err != nil {
		return Job{}, false
	}
	title := paycorFieldValue(root, "gnewtonJobPosition")
	if title == "" {
		return Job{}, false
	}
	location := paycorFieldValue(root, "gnewtonJobLocationInfo")

	var description string
	if body := firstByID(root, "gnewtonJobDescriptionText"); body != nil {
		description = sanitizeHTML(innerHTML(body))
	}

	return Job{
		ExternalID:  id,
		URL:         jobURL,
		Title:       title,
		Company:     e.Company,
		Location:    location,
		Description: description,
		// Paycor exposes no structured remote flag, so isRemote(location) is the only signal
		// (never the title, which false-positives on "Remote …" role names).
		Remote: isRemote(location),
	}, true
}

// paycorListingURL is the board's career listing, which renders every open posting.
func paycorListingURL(board string) string {
	return fmt.Sprintf("%s/CareerHome.action?clientId=%s", paycorCareerBase, board)
}

// paycorJobURL is a posting's canonical page. The listing decorates its own links with empty
// tracking parameters ("&source=&lang=en"), so the URL is rebuilt from the ids rather than
// taken from the href: the stored URL is then the same string on every crawl, whatever the
// listing appended to it.
func paycorJobURL(board, id string) string {
	return fmt.Sprintf("%s/%s?clientId=%s&id=%s", paycorCareerBase, paycorJobAction, board, id)
}

// paycorPostingIDs returns the ids of the postings a listing links, de-duplicated in
// first-seen order. It enumerates ids rather than hrefs because one posting can be linked
// more than once under different tracking parameters, and because the id is what a canonical
// URL is rebuilt from.
func paycorPostingIDs(root *html.Node, board string) []string {
	var ids []string
	seen := make(map[string]bool)
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		id := paycorPostingID(attr(n, "href"), board)
		if id == "" || seen[id] {
			return true
		}
		seen[id] = true
		ids = append(ids, id)
		return true
	})
	return ids
}

// paycorPostingID returns the posting id an href addresses on the given board, or "" when it
// addresses no posting on it. Three hrefs do not: a link to another action, the board's own
// "submit your resume" link (the same action with no id), and a posting under a DIFFERENT
// clientId — a themed listing renders inside the employer's own website, so the page carries
// whatever else that site links to.
func paycorPostingID(href, board string) string {
	u, err := url.Parse(href)
	if err != nil || path.Base(u.Path) != paycorJobAction {
		return ""
	}
	q := u.Query()
	// The client id is hex and the portal serves the same board in either case.
	if !strings.EqualFold(q.Get("clientId"), board) {
		return ""
	}
	return q.Get("id")
}

// paycorFieldValue returns the value of one of a posting page's labelled field cells. Such a
// cell renders its label as a leading <b> ending in a colon ("<b>Position:</b>&nbsp;Line
// Cook"), so the label is skipped as an element rather than split back off a concatenated
// string — and skipped by that exact shape, because a <b> anywhere else in the cell is
// emphasis the employer put INSIDE the value, and dropping it would silently swallow words.
// What is left is whitespace-normalized: the markup indents generously, uses non-breaking
// spaces after the label, and separates repeated values (a posting naming two locations) with
// <br/>. Returns "" when the page has no such cell, which is how a withdrawn posting reads.
func paycorFieldValue(root *html.Node, id string) string {
	cell := firstByID(root, id)
	if cell == nil {
		return ""
	}
	var b strings.Builder
	leading := true
	for c := cell.FirstChild; c != nil; c = c.NextSibling {
		text := textContent(c)
		if text == "" {
			continue // the markup's own indentation, and the &nbsp; after the label
		}
		if leading {
			leading = false
			if c.Type == html.ElementNode && c.Data == "b" && strings.HasSuffix(text, ":") {
				continue // the field's own label, not part of its value
			}
		}
		b.WriteString(text)
		b.WriteByte(' ')
	}
	return strings.Join(strings.Fields(b.String()), " ")
}
