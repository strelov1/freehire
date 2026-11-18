package searchping

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// IndexNowEngine announces URLs through IndexNow, the protocol Bing, Yandex, Seznam and
// Naver share. One POST tells all of them.
//
// No quota and no credential: ownership is proven by serving the key from the site at
// keyLocation, so the key is PUBLIC by design, not a secret.
//
// Google does not participate, so this does not substitute for GoogleEngine.
type IndexNowEngine struct {
	client      *http.Client
	host        string
	key         string
	keyLocation string
	endpoint    string
}

const (
	indexNowEngineName = "indexnow"
	indexNowEndpoint   = "https://api.indexnow.org/indexnow"

	// The protocol's own cap on one request.
	indexNowMaxURLs = 10000
)

// NewIndexNowEngine builds the engine, or returns nil when no key is configured — the
// fleet's "this channel is off". origin is the public site origin; the key file must be
// served from it at /<key>.txt, which is what VerifyKey checks.
func NewIndexNowEngine(origin, key string, timeout time.Duration) (*IndexNowEngine, error) {
	key = strings.TrimSpace(key)
	if key == "" {
		return nil, nil
	}
	origin = strings.TrimRight(strings.TrimSpace(origin), "/")
	parsed, err := url.Parse(origin)
	if err != nil || parsed.Host == "" {
		return nil, fmt.Errorf("indexnow: origin %q is not a URL with a host", origin)
	}
	return &IndexNowEngine{
		client:      &http.Client{Timeout: timeout},
		host:        parsed.Host,
		key:         key,
		keyLocation: origin + "/" + key + ".txt",
		endpoint:    indexNowEndpoint,
	}, nil
}

func (i *IndexNowEngine) Name() string { return indexNowEngineName }

// DailyBudget is 0: IndexNow publishes no quota; the worker's batch size bounds a run.
func (i *IndexNowEngine) DailyBudget() int { return 0 }

// Accepts takes everything. It is the only route for company pages, which Google's
// Indexing API may not be told about.
func (i *IndexNowEngine) Accepts(Kind) bool { return true }

// VerifyKey fetches the key file the site serves and checks it matches the key this
// engine will send. The key lives in two places (web/static and the worker's
// configuration) and a mismatch yields a 403 on every submission. Called once at
// startup; a failure is worth refusing to run for.
func (i *IndexNowEngine) VerifyKey(ctx context.Context) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, i.keyLocation, nil)
	if err != nil {
		return err
	}
	resp, err := i.client.Do(req)
	if err != nil {
		return fmt.Errorf("fetch %s: %w", i.keyLocation, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("key file %s: status %d (it must be served for IndexNow to accept anything)", i.keyLocation, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<10))
	if err != nil {
		return fmt.Errorf("read %s: %w", i.keyLocation, err)
	}
	if served := strings.TrimSpace(string(body)); served != i.key {
		return fmt.Errorf("key file %s serves a different key than this worker sends", i.keyLocation)
	}
	return nil
}

// Announce submits the batch in one call; the answer covers the whole list, so there is
// no partial success.
func (i *IndexNowEngine) Announce(ctx context.Context, urls []string) ([]string, error) {
	if len(urls) == 0 {
		return nil, nil
	}
	if len(urls) > indexNowMaxURLs {
		return nil, fmt.Errorf("indexnow: %d urls exceeds the protocol's limit of %d", len(urls), indexNowMaxURLs)
	}

	payload, err := json.Marshal(map[string]any{
		"host":        i.host,
		"key":         i.key,
		"keyLocation": i.keyLocation,
		"urlList":     urls,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, i.endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("content-type", "application/json; charset=utf-8")

	resp, err := i.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<10))

	// 200 accepted; 202 accepted with the key still being validated (normal after a key change).
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		return urls, nil
	}
	return nil, fmt.Errorf("indexnow status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
