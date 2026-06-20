package sources

import (
	"bytes"
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"time"

	fhttp "github.com/bogdanfinn/fhttp"
	tls_client "github.com/bogdanfinn/tls-client"
	"github.com/bogdanfinn/tls-client/profiles"
	"golang.org/x/net/html"

	"github.com/strelov1/freehire/internal/safehttp"
)

// metaHTTP is the Chrome-fingerprint transport used ONLY by the Meta adapter. Meta's edge
// rejects Go's default TLS+HTTP/2 fingerprint with HTTP 400 (a live spike confirmed plain
// JA3 spoofing via utls is not enough — it fingerprints the HTTP/2 layer too); tls-client
// with a Chrome profile spoofs both the JA3 and the Chrome HTTP/2 fingerprint and is served
// normally. This is the only file in the package that imports tls-client/fhttp; the fork
// never leaks into the rest of internal/sources. Connections still dial through the safehttp
// SSRF guard via WithDialer (TestMetaHTTPBlocksInternalTarget locks that contract).
//
// Upgrade note: tls-client bundles forks of net/http (fhttp) and utls. When bumping it, re-run
// the live adapter smoke (the edge can tighten its fingerprint check) and re-confirm the SSRF
// dial path still routes through WithDialer's Control hook.
type metaHTTP struct {
	client tls_client.HttpClient
}

const (
	metaClientTimeout = 30 * time.Second
	metaDialTimeout   = 15 * time.Second
	// metaUserAgent must agree with the spoofed Chrome profile below; a mismatched UA is a
	// cheap bot tell.
	metaUserAgent = "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/133.0.0.0 Safari/537.36"
)

// newMetaHTTP builds the Meta transport: a tls-client with the Chrome_133 profile dialing
// through the SSRF-guarded dialer.
func newMetaHTTP() (*metaHTTP, error) {
	client, err := tls_client.NewHttpClient(tls_client.NewNoopLogger(),
		tls_client.WithTimeoutSeconds(int(metaClientTimeout/time.Second)),
		tls_client.WithClientProfile(profiles.Chrome_133),
		tls_client.WithDialer(*safehttp.GuardedDialer(metaDialTimeout)),
	)
	if err != nil {
		return nil, fmt.Errorf("sources: build meta tls-client: %w", err)
	}
	return &metaHTTP{client: client}, nil
}

// get issues a Chrome-shaped GET and returns the bounded response body, erroring on any
// non-2xx status. Unlike the shared Client, it does not retry: this is a single low-volume
// daily crawl of one host, a dropped detail page reappears on the next run, and a transient
// list-fetch failure simply fails the board for that run (no jobs are closed on a single miss).
func (m *metaHTTP) get(ctx context.Context, url string) ([]byte, error) {
	req, err := fhttp.NewRequestWithContext(ctx, fhttp.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("sources: build request %s: %w", url, err)
	}
	req.Header = metaHeaders()

	resp, err := m.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("sources: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("sources: GET %s: status %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBody))
	if err != nil {
		return nil, fmt.Errorf("sources: read %s: %w", url, err)
	}
	return body, nil
}

// GetXML fetches url over the Chrome-fingerprint client and decodes its XML body into v.
func (m *metaHTTP) GetXML(ctx context.Context, url string, v any) error {
	body, err := m.get(ctx, url)
	if err != nil {
		return err
	}
	if err := xml.Unmarshal(body, v); err != nil {
		return fmt.Errorf("sources: decode %s: %w", url, err)
	}
	return nil
}

// GetHTML fetches url over the Chrome-fingerprint client and returns its parsed HTML tree.
func (m *metaHTTP) GetHTML(ctx context.Context, url string) (*html.Node, error) {
	body, err := m.get(ctx, url)
	if err != nil {
		return nil, err
	}
	node, err := html.Parse(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("sources: parse %s: %w", url, err)
	}
	return node, nil
}

// metaHeaders is a Chrome-shaped header set with an explicit order (fhttp's HeaderOrderKey).
// The header presence and order are part of the fingerprint Meta's edge checks, so they are
// kept consistent with the spoofed Chrome_133 profile.
func metaHeaders() fhttp.Header {
	return fhttp.Header{
		"sec-ch-ua":          {`"Chromium";v="133", "Not(A:Brand";v="99"`},
		"sec-ch-ua-mobile":   {"?0"},
		"sec-ch-ua-platform": {`"macOS"`},
		"user-agent":         {metaUserAgent},
		"accept":             {"text/html,application/xhtml+xml,application/xml;q=0.9,image/avif,image/webp,*/*;q=0.8"},
		"sec-fetch-site":     {"none"},
		"sec-fetch-mode":     {"navigate"},
		"sec-fetch-user":     {"?1"},
		"sec-fetch-dest":     {"document"},
		"accept-language":    {"en-US,en;q=0.9"},
		fhttp.HeaderOrderKey: {
			"sec-ch-ua", "sec-ch-ua-mobile", "sec-ch-ua-platform", "user-agent",
			"accept", "sec-fetch-site", "sec-fetch-mode", "sec-fetch-user",
			"sec-fetch-dest", "accept-language",
		},
	}
}
