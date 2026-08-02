package main

import (
	"strings"

	"golang.org/x/net/html"
)

// pageTitle returns the document's <title> text, or "". Three probers read it: opencats for
// the portal's own name, and jazzhr and trakstar because their platforms publish the employer
// nowhere else the prober looks.
func pageTitle(root *html.Node) string {
	var title string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if title != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "title" && n.FirstChild != nil {
			title = strings.TrimSpace(n.FirstChild.Data)
			return
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return title
}
