package sources

import (
	"net/url"
	"strings"

	"golang.org/x/net/html"
)

// Generic DOM and schema.org-microdata helpers shared by the adapters whose detail page
// is server-rendered HTML (successfactors, vk, …) and by jsonld.go. They carry no
// platform-specific logic; an adapter selects the elements it needs by tag/attr/itemprop.

// walk visits nodes depth-first, descending into a node's children only while visit
// returns true for it.
func walk(n *html.Node, visit func(*html.Node) bool) {
	if !visit(n) {
		return
	}
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		walk(c, visit)
	}
}

// attr returns the value of the named attribute, or "".
func attr(n *html.Node, name string) string {
	for _, a := range n.Attr {
		if a.Key == name {
			return a.Val
		}
	}
	return ""
}

// textContent returns the concatenated text of n's descendants, trimmed.
func textContent(n *html.Node) string {
	var b strings.Builder
	walk(n, func(c *html.Node) bool {
		if c.Type == html.TextNode {
			b.WriteString(c.Data)
		}
		return true
	})
	return strings.TrimSpace(b.String())
}

// innerHTML renders n's children back to HTML markup.
func innerHTML(n *html.Node) string {
	var b strings.Builder
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		_ = html.Render(&b, c)
	}
	return b.String()
}

// findItemprops returns every element node whose itemprop attribute equals prop, in
// document order.
func findItemprops(root *html.Node, prop string) []*html.Node {
	var out []*html.Node
	walk(root, func(n *html.Node) bool {
		if n.Type == html.ElementNode && attr(n, "itemprop") == prop {
			out = append(out, n)
		}
		return true
	})
	return out
}

// itempropText returns the concatenated text of the first element carrying the given
// schema.org itemprop, or "" when none is present.
func itempropText(root *html.Node, prop string) string {
	ns := findItemprops(root, prop)
	if len(ns) == 0 {
		return ""
	}
	return textContent(ns[0])
}

// itempropHTML returns the rendered inner HTML of the richest element carrying the given
// itemprop, so it can be sanitized, or "" when none is present. A page may wrap several
// near-empty itemprop layout regions around the real body, so the element with the most
// text content is chosen rather than the first.
func itempropHTML(root *html.Node, prop string) string {
	ns := findItemprops(root, prop)
	if len(ns) == 0 {
		return ""
	}
	best, bestLen := ns[0], len(textContent(ns[0]))
	for _, n := range ns[1:] {
		if l := len(textContent(n)); l > bestLen {
			best, bestLen = n, l
		}
	}
	return innerHTML(best)
}

// ElementAttr is the exported form of elementAttr, for sibling packages (e.g.
// internal/linksource) that read a single attribute off a server-rendered element — e.g.
// the datetime of a <time class="..."> publish-date element.
func ElementAttr(root *html.Node, tag, class, name string) string {
	return elementAttr(root, tag, class, name)
}

// elementAttr returns the named attribute of the first <tag> element whose class list
// contains class, or "" when none matches. An empty class matches any element of that tag.
func elementAttr(root *html.Node, tag, class, name string) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == tag && hasClass(n, class) {
			found = attr(n, name)
			return false
		}
		return true
	})
	return found
}

// hasClass reports whether n's space-separated class attribute contains class; an empty
// class matches any element.
func hasClass(n *html.Node, class string) bool {
	if class == "" {
		return true
	}
	for _, c := range strings.Fields(attr(n, "class")) {
		if c == class {
			return true
		}
	}
	return false
}

// jobLinks walks the listing DOM and returns the absolute, deduplicated href of
// every anchor the isJob predicate accepts, resolved against base. First-seen
// order is preserved. It is the shared body of the per-adapter *JobLinks helpers,
// which differ only in their isJob test.
func jobLinks(base *url.URL, root *html.Node, isJob func(href string) bool) []string {
	var out []string
	seen := map[string]struct{}{}
	walk(root, func(n *html.Node) bool {
		if n.Type != html.ElementNode || n.Data != "a" {
			return true
		}
		href := attr(n, "href")
		if href == "" || !isJob(href) {
			return true
		}
		ref, err := url.Parse(href)
		if err != nil {
			return true // unparseable href → not a usable job link
		}
		abs := base.ResolveReference(ref).String()
		if _, ok := seen[abs]; !ok {
			seen[abs] = struct{}{}
			out = append(out, abs)
		}
		return true
	})
	return out
}

// metaProperty returns the content of <meta property="..."> (e.g. og:title), or "".
func metaProperty(root *html.Node, property string) string {
	var found string
	walk(root, func(n *html.Node) bool {
		if found != "" {
			return false
		}
		if n.Type == html.ElementNode && n.Data == "meta" &&
			attr(n, "property") == property {
			found = attr(n, "content")
			return false
		}
		return true
	})
	return found
}
