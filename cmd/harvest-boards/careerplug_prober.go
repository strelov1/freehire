package main

import (
	"context"
	"fmt"
	"regexp"

	"golang.org/x/net/html"
)

// careerplugProber validates a CareerPlug board "<slug>" by counting the postings linked
// from its /jobs listing page ("<slug>.careerplug.com/jobs"). CareerPlug's listing exposes
// no single account name (the adapter reads a per-posting employer from each job's JSON-LD),
// so the prober returns an empty name.
type careerplugProber struct{}

// careerplugJobLink captures a posting's numeric id from a CareerPlug job link (/jobs/<id>),
// so duplicate links to the same posting (title + row) count once and pagination/listing
// links are ignored.
var careerplugJobLink = regexp.MustCompile(`/jobs/(\d+)`)

func (careerplugProber) probe(ctx context.Context, c httpClient, slug string) (string, int, error) {
	root, err := c.GetHTML(ctx, fmt.Sprintf("https://%s.careerplug.com/jobs", slug))
	if err != nil {
		return "", 0, nil
	}
	ids := map[string]bool{}
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, a := range n.Attr {
				if a.Key == "href" {
					if m := careerplugJobLink.FindStringSubmatch(a.Val); m != nil {
						ids[m[1]] = true
					}
				}
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	if len(ids) == 0 {
		return "", 0, nil
	}
	return "", len(ids), nil
}
