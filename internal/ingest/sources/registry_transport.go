package sources

// cookieSessionSource builds a registry entry for an adapter whose API is session- or
// CSRF-cookie-authenticated (taleo, aijobs) and so cannot share the shared jar-less
// client. On a real crawl (c != nil) it hands build a fresh cookie-jar client; on the
// transport-free taxonomy path (c == nil, e.g. FilterableProviders) it hands build a true
// nil transport, since T is an interface type parameter and its zero value is nil —
// Provider()/marker assertions never touch it.
func cookieSessionSource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newCookieClient()).(T))
}

// singleUseConnSource builds a registry entry for an adapter whose platform mishandles a
// reused HTTP connection (profession) and so cannot share the pooled client. It mirrors
// cookieSessionSource exactly, including its nil-transport case: on the taxonomy path
// (c == nil) it hands build a true nil, since T is an interface type parameter whose zero
// value is nil and Provider()/marker assertions never touch it.
//
// The failure it answers is worth stating, because it does not present as a transport
// problem: over a pooled connection the platform answers the first request or two and then
// returns a bare EOF, and one of the answers it does give can be a truncated body — a
// sitemap that parses cleanly as a much shorter one. Pacing does not help and a retry
// cannot, since the retry reuses the same pool.
func singleUseConnSource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newSingleUseConnClient()).(T))
}

// browserUASource builds a registry entry for an adapter whose platform refuses any client
// that identifies itself, on a path that platform's own robots.txt allows (4dayweek). It
// mirrors cookieSessionSource exactly, including its nil-transport case for the taxonomy path.
// See BrowserUserAgent for the measurement, and for the line this does NOT cross.
func browserUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newBrowserUAClient()).(T))
}

// macBrowserUASource is browserUASource for an edge that admits only a macOS browser string
// (Feishu Hire). See macBrowserUserAgent.
func macBrowserUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newMacBrowserUAClient()).(T))
}

// compatibleUASource is browserUASource for an edge that admits the project's own identity once
// it is written in the browser-compatible form (OCC Mundial). See compatibleUserAgent.
func compatibleUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(NewCompatibleUAClient()).(T))
}

// crawlerUASource is compatibleUASource for an edge that refuses every self-identifying
// string but serves a recognised search-crawler one (UKG Pro Recruiting: its nginx 403s
// "freehire/0.1 (+https://freehire.me)" on the listing API and the detail pages alike, while
// the Googlebot string and a plain browser string are both served — measured live 2026-11-05).
// See crawlerUserAgent for the line this does not cross.
func crawlerUASource[T any](c HTTPClient, build func(T) Source) Source {
	if c == nil {
		var zero T
		return build(zero)
	}
	return build(any(newCrawlerUAClient()).(T))
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
