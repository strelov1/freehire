package main

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	"golang.org/x/net/html"
)

// oracleProber validates an Oracle Recruiting Cloud board "<host>/<site>" by listing one
// requisition from its public candidate-experience API. Oracle exposes no employer name
// (the host is an opaque tenant code), so it returns an empty name and leans on the
// seed-supplied company; see internal/sources/oracle.go for the board-id shape.
type oracleProber struct{}

// dedupKey folds an Oracle board's site segment to lower case. Oracle answers a requisition
// listing for any spelling of the site — CX_2001, cx_2001 and Cx_2001 all return the same 51
// jobs on tenant eetz.fa.us6 (checked live) — so two spellings are one board, and a candidate
// derived from a job URL carries whatever case that URL happened to use. Without folding it is
// filed beside the board we already crawl, which is the same defect Paycom's hex keys had.
//
// The HOST half is folded too: DNS is case-insensitive, so it cannot distinguish boards either.
// This does not collapse a tenant's several real sites (CX, CX_1, CX_2001 are different names,
// not different spellings) — only the case of one name.
func (oracleProber) dedupKey(boardID string) string { return strings.ToLower(boardID) }

func (oracleProber) probe(ctx context.Context, c httpClient, boardID string) (string, int, error) {
	host, site, ok := strings.Cut(boardID, "/")
	if !ok || host == "" || site == "" {
		return "", 0, nil
	}
	url := fmt.Sprintf(
		"https://%s/hcmRestApi/resources/latest/recruitingCEJobRequisitions"+
			"?onlyData=true&finder=findReqs;siteNumber=%s,limit=1", host, site)
	var resp struct {
		Items []struct {
			TotalJobsCount  int `json:"TotalJobsCount"`
			RequisitionList []struct {
				ID string `json:"Id"`
			} `json:"requisitionList"`
		} `json:"items"`
	}
	if err := c.GetJSON(ctx, url, &resp); err != nil || len(resp.Items) == 0 {
		return "", 0, nil
	}
	page := resp.Items[0]
	n := page.TotalJobsCount
	if n == 0 {
		n = len(page.RequisitionList)
	}
	if n == 0 {
		return "", 0, nil
	}
	return "", n, nil
}

// jazzhrProber validates a JazzHR board "<slug>" by counting the postings linked from its
// single /apply listing page ("<slug>.applytojob.com/apply"). JazzHR's listing exposes no
// employer name, but the page titles itself "<Company> - Career Page", so the prober reads
// the employer there and the board can be gated against the name the seed expected rather
// than accepted on liveness alone.
type jazzhrProber struct{}

// jazzhrApplyHref captures a posting's token from a JazzHR job link (/apply/<token>/<slug>),
// so duplicate links to the same posting (title + card) count once.
var jazzhrApplyHref = regexp.MustCompile(`/apply/([A-Za-z0-9]+)/`)

func (jazzhrProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s.applytojob.com/apply", slug))
	if err != nil {
		return "", 0, nil
	}
	tokens := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					if m := jazzhrApplyHref.FindStringSubmatch(a.Val); m != nil {
						tokens[m[1]] = true
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if len(tokens) == 0 {
		return "", 0, nil
	}
	return jazzhrEmployer(pageTitle(root)), len(tokens), nil
}

// jazzhrEmployer pulls the employer out of a JazzHR career-page title, which renders as
// "<Company> - Career Page". A title without that suffix names nobody in particular, so it
// yields "" and the board falls back to the seed's label rather than being gated against a
// generic word.
func jazzhrEmployer(title string) string {
	name, ok := strings.CutSuffix(strings.TrimSpace(title), " - Career Page")
	if !ok {
		return ""
	}
	return strings.TrimSpace(name)
}
