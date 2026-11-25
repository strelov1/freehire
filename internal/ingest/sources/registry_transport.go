package sources

// ownTransportSource builds a registry entry for an adapter that cannot share the pooled
// client and needs a transport of its own: a session- or CSRF-cookie jar (taleo, aijobs), a
// connection the platform mishandles when reused (profession), or a User-Agent its edge admits
// (4dayweek, Feishu Hire, OCC Mundial, UKG). On a real crawl (c != nil) it hands build a client
// from newClient; on the transport-free taxonomy path (c == nil, e.g. FilterableProviders) it
// hands build a true nil transport, since T is an interface type parameter and its zero value is
// nil — Provider()/marker assertions never touch it.
func ownTransportSource[T any](c HTTPClient, newClient func() *Client, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newClient()).(T))
}

// cookieSessionSource is ownTransportSource for an adapter whose API is session- or
// CSRF-cookie-authenticated, and so cannot share the shared jar-less client.
func cookieSessionSource[T any](c HTTPClient, build func(T) Source) Source {
	return ownTransportSource(c, newCookieClient, build)
}

// singleUseConnSource is ownTransportSource for an adapter whose platform mishandles a reused
// HTTP connection (profession).
//
// The failure it answers is worth stating, because it does not present as a transport
// problem: over a pooled connection the platform answers the first request or two and then
// returns a bare EOF, and one of the answers it does give can be a truncated body — a
// sitemap that parses cleanly as a much shorter one. Pacing does not help and a retry
// cannot, since the retry reuses the same pool.
func singleUseConnSource[T any](c HTTPClient, build func(T) Source) Source {
	return ownTransportSource(c, newSingleUseConnClient, build)
}

// browserUASource is ownTransportSource for an adapter whose platform refuses any client that
// identifies itself, on a path that platform's own robots.txt allows (4dayweek). See
// BrowserUserAgent for the rationale, and for the line this does NOT cross.
func browserUASource[T any](c HTTPClient, build func(T) Source) Source {
	return ownTransportSource(c, newBrowserUAClient, build)
}

// macBrowserUASource is browserUASource for an edge that admits only a macOS browser string
// (Feishu Hire). See macBrowserUserAgent.
func macBrowserUASource[T any](c HTTPClient, build func(T) Source) Source {
	return ownTransportSource(c, newMacBrowserUAClient, build)
}

// compatibleUASource is browserUASource for an edge that admits the project's own identity once
// it is written in the browser-compatible form (OCC Mundial). See compatibleUserAgent.
func compatibleUASource[T any](c HTTPClient, build func(T) Source) Source {
	return ownTransportSource(c, NewCompatibleUAClient, build)
}

// crawlerUASource is compatibleUASource for an edge that refuses every self-identifying
// string but serves a recognised search-crawler one (UKG Pro Recruiting: its nginx 403s
// "freehire/0.1 (+https://freehire.me)" on the listing API and the detail pages alike, while
// the Googlebot string and a plain browser string are both served).
// See crawlerUserAgent for the line this does not cross.
func crawlerUASource[T any](c HTTPClient, build func(T) Source) Source {
	return ownTransportSource(c, newCrawlerUAClient, build)
}

// reg indexes sources by provider key. A duplicate key means two adapters claim the
// same platform — a programming error — so it panics rather than silently dropping one.
func reg(sources ...Source) map[string]Source {
	m := make(map[string]Source, len(sources))
	for _, s := range sources {
		if _, dup := m[s.Provider()]; dup {
			panic("sources: duplicate provider " + s.Provider())
		}
		m[s.Provider()] = s
	}
	return m
}
