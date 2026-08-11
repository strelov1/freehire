package liveness

import (
	"context"
	"io"
	"net/http"
)

// UserAgent identifies the probe to employer sites, matching the ingest client.
const UserAgent = "freehire/0.1 (+https://freehire.me)"

// maxBody caps how much of a response we read: enough to find an expired phrase or
// judge content length, without buffering a pathologically large page.
const maxBody = 512 << 10 // 512 KiB

// Fetch probes a posting URL with a plain GET, following redirects, and returns the
// final HTTP status, the resolved URL, and the (capped) body text for Classify. A
// transport failure (DNS, refused, timeout) returns status 0 with the error, which
// Classify treats as not-expired — a probe that could not reach the page is never a
// death signal.
func Fetch(ctx context.Context, client *http.Client, rawURL string) (status int, finalURL, body string, err error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return 0, rawURL, "", err
	}
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml")

	resp, err := client.Do(req)
	if err != nil {
		return 0, rawURL, "", err
	}
	defer resp.Body.Close()

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		return resp.StatusCode, resp.Request.URL.String(), "", err
	}
	return resp.StatusCode, resp.Request.URL.String(), string(b), nil
}
