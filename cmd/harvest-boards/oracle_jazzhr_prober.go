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
// employer name (the adapter reads it from each posting's JSON-LD at ingest), so the prober
// returns an empty name.
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
	return "", len(tokens), nil
}
