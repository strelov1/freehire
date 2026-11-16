package sources

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"golang.org/x/net/html"
)

// PostJSONWithHeaders behaves like PostJSON but attaches extra request headers.
func (c *Client) PostJSONWithHeaders(ctx context.Context, url string, headers map[string]string, body, v any) error {
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("sources: marshal request %s: %w", url, err)
	}
	return c.do(ctx, request{
		method:  http.MethodPost,
		url:     url,
		body:    payload,
		accept:  "application/json",
		headers: headers,
		decode: func(resp *http.Response) error {
			return json.NewDecoder(resp.Body).Decode(v)
		},
	})
}

// CookieValue returns the value of the named cookie as currently held in the client's
// cookie jar for url's host, "" if unset or the client has no jar (the shared client,
// built without one). It exists for the double-submit CSRF pattern (e.g. aijobs.net's
// Django flow), where a value a prior GET's Set-Cookie response established must also be
// echoed back as a request header/body field — something the jar alone, which only
// re-attaches cookies to outgoing requests automatically, cannot do.
func (c *Client) CookieValue(rawURL, name string) string {
	if c.httpClient.Jar == nil {
		return ""
	}
	u, err := url.Parse(rawURL)
	if err != nil {
		return ""
	}
	for _, ck := range c.httpClient.Jar.Cookies(u) {
		if ck.Name == name {
			return ck.Value
		}
	}
	return ""
}

// PostFormWithHeaders POSTs values as an application/x-www-form-urlencoded body with
// extra request headers, and returns the response parsed as HTML — for a listing
// endpoint that is POST-only and CSRF-protected (aijobs.net's Django cookie-token
// flow) rather than a JSON API.
func (c *Client) PostFormWithHeaders(ctx context.Context, url string, headers map[string]string, values url.Values) (*html.Node, error) {
	var node *html.Node
	err := c.do(ctx, request{
		method:      http.MethodPost,
		url:         url,
		body:        []byte(values.Encode()),
		contentType: "application/x-www-form-urlencoded",
		accept:      "text/html",
		headers:     headers,
		decode: func(resp *http.Response) error {
			n, err := html.Parse(resp.Body)
			if err != nil {
				return err
			}
			node = n
			return nil
		},
	})
	return node, err
}

// PostFormJSON POSTs values form-urlencoded and decodes the JSON response into v.
func (c *Client) PostFormJSON(ctx context.Context, url string, values url.Values, v any) error {
	return c.do(ctx, request{
		method:      http.MethodPost,
		url:         url,
		body:        []byte(values.Encode()),
		contentType: "application/x-www-form-urlencoded",
		accept:      "application/json",
		decode: func(resp *http.Response) error {
			return json.NewDecoder(resp.Body).Decode(v)
		},
	})
}

// PostFormText POSTs values form-urlencoded and returns the raw response body, under the same
// maxTextBody cap as GetText.
func (c *Client) PostFormText(ctx context.Context, url string, values url.Values) (string, error) {
	var body string
	err := c.do(ctx, request{
		method:      http.MethodPost,
		url:         url,
		body:        []byte(values.Encode()),
		contentType: "application/x-www-form-urlencoded",
		accept:      "text/html",
		decode: func(resp *http.Response) error {
			b, err := io.ReadAll(newCappedReader(resp.Body, url, maxTextBody))
			body = string(b)
			return err
		},
	})
	return body, err
}
