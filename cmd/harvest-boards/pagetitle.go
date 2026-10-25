package main

import (
	"strings"

	"golang.org/x/net/html"
)

// pageTitle returns the document's <title> text, or "". Five probers read it: opencats for
// the portal's own name, and jazzhr, trakstar, darwinbox and pyjamahr because their platforms
// publish the employer nowhere else the prober looks.
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

// metaProperty returns the content of <meta property="..."> (e.g. og:site_name), or "".
// Teamtailor and Zoho Recruit read it: the platform names the employer there and nowhere else
// on the listing.
func metaProperty(root *html.Node, property string) string {
	var content string
	var walk func(*html.Node)
	walk = func(n *html.Node) {
		if content != "" {
			return
		}
		if n.Type == html.ElementNode && n.Data == "meta" {
			var prop, value string
			for _, a := range n.Attr {
				switch a.Key {
				case "property":
					prop = a.Val
				case "content":
					value = a.Val
				}
			}
			if prop == property {
				content = strings.TrimSpace(value)
				return
			}
		}
		for ch := n.FirstChild; ch != nil; ch = ch.NextSibling {
			walk(ch)
		}
	}
	walk(root)
	return content
}
