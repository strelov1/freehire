package sources

import (
	"strings"

	"golang.org/x/net/html"
)

// JazzHR omits JobPosting JSON-LD on some live pages while retaining the full
// server-rendered posting. Bind the fallback to the requested native identity;
// a listing, incomplete shell or another posting cannot prove this one was read.
func jazzhrDOMPosting(root *html.Node, id string) (title, location, description string, ok bool) {
	body := firstByTag(root, "body")
	if body == nil || !HasClass(body, "job-"+id) {
		return "", "", "", false
	}
	header := firstByClass(body, "job-header")
	desc := firstByID(body, "job-description")
	if header == nil || desc == nil {
		return "", "", "", false
	}
	heading := firstByTag(header, "h2")
	if heading == nil {
		return "", "", "", false
	}
	title = textContent(heading)
	description = sanitizeHTML(innerHTML(desc))
	visible, err := html.Parse(strings.NewReader(description))
	if title == "" || err != nil || textContent(visible) == "" {
		return "", "", "", false
	}
	walk(header, func(n *html.Node) bool {
		if n.Type == html.ElementNode && Attr(n, "title") == "Location" {
			location = textContent(n)
			return false
		}
		return true
	})
	return title, location, description, true
}
