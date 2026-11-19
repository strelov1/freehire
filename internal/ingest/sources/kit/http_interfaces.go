package kit

import (
	"context"
	"io"
	"net/url"

	"golang.org/x/net/html"
)

// JSONGetter fetches a URL and decodes its JSON body into v.
type JSONGetter interface {
	GetJSON(ctx context.Context, url string, v any) error
}

// JSONResolvedGetter fetches a URL following redirects and decodes its JSON body, also
// returning the FINAL URL — the JSON counterpart of HTMLResolvedGetter, for a platform that
// answers "this board is not served" with a redirect instead of a status code.
type JSONResolvedGetter interface {
	GetJSONResolved(ctx context.Context, url string, v any) (string, error)
}

// StreamGetter issues a GET and hands the raw response body to fn for streaming decode.
// Unlike GetJSON it uses a long timeout and neither buffers nor size-caps the body, so it
// suits large, slow feeds (e.g. the throttled JobStream) that a 15s, size-capped GetJSON
// cannot read.
type StreamGetter interface {
	GetStream(ctx context.Context, url, accept string, fn func(io.Reader) error) error
}

// XMLGetter fetches a URL and decodes its XML body into v (platforms publishing an XML
// feed, e.g. Personio).
type XMLGetter interface {
	GetXML(ctx context.Context, url string, v any) error
}

// XMLResolvedGetter fetches a URL following redirects and decodes its XML body, also
// returning the FINAL URL — the XML counterpart of JSONResolvedGetter, for a feed whose
// platform answers "this board is not served" with a redirect instead of a status code.
type XMLResolvedGetter interface {
	GetXMLResolved(ctx context.Context, url string, v any) (string, error)
}

// HTMLGetter fetches a URL and returns its parsed HTML tree, for adapters whose detail
// is server-rendered HTML rather than a structured body (e.g. SuccessFactors).
type HTMLGetter interface {
	GetHTML(ctx context.Context, url string) (*html.Node, error)
}

// HTMLResolvedGetter fetches a URL following redirects and returns its parsed HTML tree
// together with the FINAL URL after redirects — for adapters that must observe a redirect
// (e.g. djinni's listing, whose past-the-end page 302s to the bare listing, so the final
// URL is the only reliable end-of-feed signal).
type HTMLResolvedGetter interface {
	GetHTMLResolved(ctx context.Context, url string) (*html.Node, string, error)
}

// TextGetter fetches a URL and returns its raw response body, for adapters that regex a
// token or config blob out of a page rather than parsing its DOM (e.g. Cornerstone).
type TextGetter interface {
	GetText(ctx context.Context, url string) (string, error)
}

// HeaderTextGetter behaves like TextGetter but attaches extra request headers, for a raw-body
// endpoint gated behind a non-secret header (e.g. a Next.js App Router RSC data endpoint, which
// returns its flight payload only when the request carries the "RSC: 1" header). The custom
// headers never override the standard User-Agent/Accept.
type HeaderTextGetter interface {
	GetTextWithHeaders(ctx context.Context, url string, headers map[string]string) (string, error)
}

// LargeTextGetter behaves like TextGetter but without its tighter maxTextBody cap, for the rare
// adapter whose raw-body page is legitimately large because it inlines a whole feed rather than
// carrying a link to scan for (e.g. Yandex Crowd's /vacancies page, which server-renders every
// posting's full description into one <script> tag).
type LargeTextGetter interface {
	GetLargeText(ctx context.Context, url string) (string, error)
}

// JSONPoster sends a JSON request body and decodes the JSON response (platforms whose
// listing API is POST-only, e.g. Workday).
type JSONPoster interface {
	PostJSON(ctx context.Context, url string, body, v any) error
}

// HeaderJSONGetter and HeaderJSONPoster behave like JSONGetter/JSONPoster but attach
// extra request headers, for an API gated behind a non-secret header (e.g. MTS's public
// x-api-key). The custom headers never override the standard User-Agent/Accept.
type HeaderJSONGetter interface {
	GetJSONWithHeaders(ctx context.Context, url string, headers map[string]string, v any) error
}

type HeaderJSONPoster interface {
	PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error
}

// CookieReader reads back a cookie a prior request established in the client's jar, for
// the double-submit CSRF pattern (see Client.CookieValue).
type CookieReader interface {
	CookieValue(url, name string) string
}

// HeaderFormPoster POSTs a form-urlencoded body with extra request headers and returns
// the response parsed as HTML — for a CSRF-protected listing endpoint that is form-bodied
// rather than JSON (e.g. aijobs.net's Django cookie-token flow).
type HeaderFormPoster interface {
	PostFormWithHeaders(ctx context.Context, url string, headers map[string]string, values url.Values) (*html.Node, error)
}

// FormJSONPoster POSTs a form-urlencoded body and decodes the JSON response, for an endpoint
// that reads its parameters the way a jQuery $.ajax call sends them (HiringRoom's listing,
// which refuses the same parameters as JSON or as a query string).
type FormJSONPoster interface {
	PostFormJSON(ctx context.Context, url string, values url.Values, v any) error
}

// FormTextPoster POSTs a form-urlencoded body and returns the raw response body — the
// TextGetter of a form, for a site whose pages are not UTF-8 and so must be transcoded
// before they are parsed (Civil Service Jobs serves Windows-1252 and declares no charset).
type FormTextPoster interface {
	PostFormText(ctx context.Context, url string, values url.Values) (string, error)
}

// HTTPClient is the full transport surface, composing every capability role. The real
// Client implements it; sources.All holds one and passes it to each adapter, which then
// narrows it to the role(s) it actually uses.
type HTTPClient interface {
	JSONGetter
	JSONResolvedGetter
	StreamGetter
	XMLGetter
	XMLResolvedGetter
	HTMLGetter
	HTMLResolvedGetter
	TextGetter
	HeaderTextGetter
	LargeTextGetter
	JSONPoster
	HeaderJSONGetter
	HeaderJSONPoster
	HeaderFormPoster
	FormJSONPoster
	FormTextPoster
	CookieReader
}
