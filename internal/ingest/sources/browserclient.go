package sources

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"net/http"

	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/platform/browser"
)

// browserClient fetches through a headless browser instead of an HTTP client, for a source
// whose pages are served only to a client that ran JavaScript.
//
// It implements exactly XMLGetter and HTMLGetter — which together are echojobsHTTP — so an
// adapter written against those reads browser-fetched bytes without knowing it. That is the
// whole point of the seam: what broke at echojobs was how the bytes arrive, not how they are
// read, and not one line of the reading changed.
//
// Its errors are *StatusError, the same type the plain client produces, and that is
// load-bearing rather than tidy. detailUnreadable and isRateLimited match the status
// structurally through that type; a browser-fetched 404 that arrived as a bare error would
// stop meaning "this posting is gone" and the lifecycle would quietly stop closing anything.
type browserClient struct {
	session sessionSource
}

// sessionSource hands out the browser session, built on first use. The indirection is what
// keeps Chrome from starting for a crawl that never reaches a browser-tier provider — see
// lazySession.
type sessionSource interface {
	get(ctx context.Context) (*browser.Session, error)
}

var (
	_ XMLGetter  = (*browserClient)(nil)
	_ HTMLGetter = (*browserClient)(nil)
)

func newBrowserClient(s sessionSource) *browserClient { return &browserClient{session: s} }

// browserStatusError renders a fetched status the way the plain client renders one, or nil
// when the response was a success. Only 2xx is a success: a 3xx reaching here means the
// browser did not follow the redirect, which is a fetch that did not deliver content.
func browserStatusError(url string, status int) error {
	if status >= 200 && status < 300 {
		return nil
	}
	return &StatusError{Method: http.MethodGet, Code: status, URL: url}
}

func (c *browserClient) get(ctx context.Context, url string) ([]byte, error) {
	session, err := c.session.get(ctx)
	if err != nil {
		return nil, fmt.Errorf("sources: browser GET %s: %w", url, err)
	}
	status, body, err := session.Fetch(ctx, url)
	if err != nil {
		return nil, fmt.Errorf("sources: browser GET %s: %w", url, err)
	}
	if err := browserStatusError(url, status); err != nil {
		return nil, err
	}
	return body, nil
}

// GetXML fetches url through the browser and decodes its body as XML.
func (c *browserClient) GetXML(ctx context.Context, url string, v any) error {
	body, err := c.get(ctx, url)
	if err != nil {
		return err
	}
	if err := xml.NewDecoder(bytes.NewReader(body)).Decode(v); err != nil {
		return fmt.Errorf("sources: browser GET %s: decode xml: %w", url, err)
	}
	return nil
}

// GetHTML fetches url through the browser and parses its body as HTML.
//
// The body is the response as the server sent it, not the DOM the browser would have built
// from it — the fetch happens inside a page rather than by navigating to the URL. For a
// server-rendered posting that is the same markup and a great deal cheaper; a source that
// only assembles its content client-side would need a navigation instead, and does not have
// one here.
func (c *browserClient) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	body, err := c.get(ctx, url)
	if err != nil {
		return nil, err
	}
	node, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sources: browser GET %s: parse html: %w", url, err)
	}
	return node, nil
}
