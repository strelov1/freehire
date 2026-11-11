// Package htmltext converts stored job-description HTML into alternative
// representations for consumers: plain text and Markdown. Each converter is
// lenient — a conversion error degrades to returning the input unchanged rather
// than failing the caller.
package htmltext

import (
	"container/list"
	"crypto/sha256"
	"encoding/hex"
	stdhtml "html"
	"strings"
	"sync"

	htmltomarkdown "github.com/JohannesKaufmann/html-to-markdown/v2"
	"github.com/jaytaylor/html2text"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/llm"
)

// maxInputRunes bounds request-facing conversions. Callers can supply a full stored
// description, and neither third-party converter bounds its own work, so truncating
// before conversion caps the cost of repeatedly rendering it on a request path.
const maxInputRunes = 50_000

// cacheCapacity bounds the memory each per-format result cache can hold. A description
// rarely changes, so the same content is very likely to be converted again — the same
// job appears across multiple search-result pages, and AgentSearchJobs is public and
// unauthenticated. Caching by content hash rather than by job id means a changed
// description is simply a cache miss, with no invalidation to get wrong.
const cacheCapacity = 128

var (
	textCache = newCache(cacheCapacity)
	mdCache   = newCache(cacheCapacity)
)

// ToText renders HTML as plain text with tags removed. On a conversion error it
// returns the (possibly truncated) input unchanged.
func ToText(html string) string {
	return convert(html, textCache, func(s string) (string, error) {
		return html2text.FromString(s, html2text.Options{OmitLinks: true})
	})
}

// ToTextFull extracts the complete visible text for derivation, without wrapping,
// emphasis markers, truncation or caching. Text-node content is preserved and block
// elements add line boundaries. Plain text keeps its original line boundaries;
// on a parse error it returns the full input unchanged.
func ToTextFull(input string) string {
	if !hasHTMLMarkup(input) {
		return stdhtml.UnescapeString(input)
	}
	doc, err := html.Parse(strings.NewReader(input))
	if err != nil {
		return input
	}
	var out strings.Builder
	var visit func(*html.Node)
	visit = func(node *html.Node) {
		if node.Type == html.TextNode {
			out.WriteString(node.Data)
			return
		}
		block := false
		if node.Type == html.ElementNode {
			switch node.Data {
			case "head", "script", "style", "template":
				return
			case "address", "article", "aside", "blockquote", "br", "dd", "details", "div", "dl", "dt",
				"fieldset", "figcaption", "figure", "footer", "form", "h1", "h2", "h3", "h4", "h5", "h6",
				"header", "hr", "li", "main", "nav", "ol", "p", "pre", "section", "summary", "table",
				"tbody", "td", "tfoot", "th", "thead", "tr", "ul":
				block = true
			}
		}
		if block {
			out.WriteByte('\n')
		}
		for child := node.FirstChild; child != nil; child = child.NextSibling {
			visit(child)
		}
		if block {
			out.WriteByte('\n')
		}
	}
	visit(doc)
	return strings.TrimSpace(out.String())
}

// hasHTMLMarkup distinguishes tags from plain-text comparisons such as "< 5".
func hasHTMLMarkup(input string) bool {
	tokens := html.NewTokenizer(strings.NewReader(input))
	for {
		switch tokens.Next() {
		case html.StartTagToken, html.EndTagToken, html.SelfClosingTagToken, html.CommentToken, html.DoctypeToken:
			return true
		case html.ErrorToken:
			return false
		}
	}
}

// ToMarkdown converts HTML to Markdown, preserving block structure such as lists
// and headings. On a conversion error it returns the (possibly truncated) input
// unchanged.
func ToMarkdown(html string) string {
	return convert(html, mdCache, func(s string) (string, error) {
		return htmltomarkdown.ConvertString(s)
	})
}

// convert bounds html to maxInputRunes, serves a cached result for identical input when
// one exists, and otherwise runs fn and caches a successful result.
func convert(html string, cache *resultCache, fn func(string) (string, error)) string {
	html = llm.TruncateRunes(html, maxInputRunes)
	key := hashOf(html)
	if out, ok := cache.get(key); ok {
		return out
	}
	out, err := fn(html)
	if err != nil {
		return html
	}
	cache.put(key, out)
	return out
}

func hashOf(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// resultCache is a fixed-capacity, concurrency-safe LRU keyed by content hash.
type resultCache struct {
	mu       sync.Mutex
	capacity int
	order    *list.List
	items    map[string]*list.Element
}

type cacheEntry struct {
	key   string
	value string
}

func newCache(capacity int) *resultCache {
	return &resultCache{
		capacity: capacity,
		order:    list.New(),
		items:    make(map[string]*list.Element, capacity),
	}
}

func (c *resultCache) get(key string) (string, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return "", false
	}
	c.order.MoveToFront(el)
	return el.Value.(*cacheEntry).value, true
}

func (c *resultCache) put(key, value string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if el, ok := c.items[key]; ok {
		el.Value.(*cacheEntry).value = value
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&cacheEntry{key: key, value: value})
	if c.order.Len() > c.capacity {
		oldest := c.order.Back()
		c.order.Remove(oldest)
		delete(c.items, oldest.Value.(*cacheEntry).key)
	}
}
