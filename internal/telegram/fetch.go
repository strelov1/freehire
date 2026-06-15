package telegram

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/strelov1/freehire/internal/safehttp"
)

// maxPreviewBody caps how much of a t.me preview page is read into memory. The
// preview of a busy channel is tens of KiB; this bounds a hostile or oversized
// response (including a gzip bomb the transport transparently inflates).
const maxPreviewBody = 8 << 20 // 8 MiB

// Fetcher reads a channel's latest posts from the public t.me web preview. It is
// the single transport boundary of the crawl: a future MTProto-based reader (for
// preview-disabled channels) replaces this implementation and nothing else.
type Fetcher struct {
	httpClient *http.Client
	baseURL    string
	userAgent  string
}

// NewFetcher builds the default preview fetcher.
func NewFetcher() *Fetcher {
	return &Fetcher{
		// Channel names come from config, but the preview host is fixed; the guarded
		// transport is defence-in-depth against a redirect to an internal target.
		httpClient: safehttp.NewClient(15 * time.Second),
		baseURL:    "https://t.me",
		userAgent:  "freehire/0.1 (+https://freehire.dev)",
	}
}

// Fetch GETs the channel's preview page and parses its posts. Any non-2xx status
// is an error — the caller counts the channel failed; nothing here retries, the
// next scheduled run is the retry.
func (f *Fetcher) Fetch(ctx context.Context, channel string) ([]Post, error) {
	url := f.baseURL + "/s/" + channel
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("telegram: build request %s: %w", url, err)
	}
	req.Header.Set("User-Agent", f.userAgent)

	resp, err := f.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("telegram: GET %s: %w", url, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("telegram: GET %s: status %d", url, resp.StatusCode)
	}

	page, err := io.ReadAll(io.LimitReader(resp.Body, maxPreviewBody))
	if err != nil {
		return nil, fmt.Errorf("telegram: read %s: %w", url, err)
	}
	return ParsePreview(channel, page)
}
