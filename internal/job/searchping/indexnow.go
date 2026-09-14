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
// No quota and no credential: ownership is proven by serving the key back from the site
// itself, at keyLocation. That makes the key PUBLIC by design — it is not a secret, and
// treating it as one would only stop it from being checked into the repository beside
// the file that serves it.
//
// Google does not participate and has said it will not, so this engine is not a
// substitute for GoogleEngine; it is the half of the problem that needs no approval.
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

// DailyBudget is 0: IndexNow publishes no quota. The protocol asks for restraint rather
// than counting, and what bounds a run here is the batch size the worker was given.
func (i *IndexNowEngine) DailyBudget() int { return 0 }

// VerifyKey fetches the key file the site serves and checks it matches the key this
// engine will send. It exists because the key lives in two places — the static file in
// web/static and the worker's configuration — and IndexNow's answer to a mismatch is a
// 403 on every submission, which reads like a broken integration rather than two values
// that drifted apart. Called once at startup; a failure is worth refusing to run for,
// since every send after it would be rejected anyway.
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

// Announce submits the batch in one call, which is what the protocol is for. It answers
// for the whole list at once, so either all of the URLs were accepted or none were —
// there is no partial success to report.
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

	// 200 accepted, 202 accepted with the key still being validated. Both mean the URLs
	// were taken; 202 is the normal answer for the first submission after a key change.
	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusAccepted {
		return urls, nil
	}
	return nil, fmt.Errorf("indexnow status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
}
