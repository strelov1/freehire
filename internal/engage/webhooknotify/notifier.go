// Package webhooknotify is the webhook implementation of notify.Notifier: it
// POSTs a subscription digest to the account's configured webhook
// destination. Unlike the other channels, the destination is a URL the user
// supplies, so every send goes through an SSRF-guarded client
// (internal/platform/safehttp) and the response is watched for the one
// definitive "stop sending" signal a receiver can give us (HTTP 410).
package webhooknotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/strelov1/freehire/internal/engage/notify"
	"github.com/strelov1/freehire/internal/platform/safehttp"
)

// requestTimeout bounds one delivery attempt so a hanging third-party endpoint cannot stall the
// worker pass.
const requestTimeout = 10 * time.Second

// Compile-time guarantee that Notifier satisfies the channel abstraction.
var _ notify.Notifier = (*Notifier)(nil)

// Notifier delivers a digest as a plain HTTP POST to dest (the account's
// webhook URL, see recipient() in internal/engage/notify).
type Notifier struct {
	http *http.Client
}

// NewNotifier builds a Notifier with an SSRF-guarded client bounded by
// requestTimeout.
func NewNotifier() *Notifier {
	return newNotifier(safehttp.NewClient(requestTimeout))
}

// newNotifier builds a Notifier against an arbitrary HTTP client, for tests: safehttp refuses
// private addresses, which makes a loopback test server unreachable.
func newNotifier(httpc *http.Client) *Notifier {
	return &Notifier{http: httpc}
}

// payload is the JSON body POSTed to the destination.
type payload struct {
	SavedSearchName string             `json:"saved_search_name"`
	Total           int                `json:"total"`
	Jobs            []notify.DigestJob `json:"jobs"`
}

// Send POSTs the digest to dest. A 410 Gone becomes notify.ErrRecipientGone, so the engine
// disables the destination and soft-skips instead of retrying.
//
// One POST per digest: a receiver reads one saved search's payload, and this loop keeps that
// wire shape even if the engine bundled digests (see notify.Message).
func (n *Notifier) Send(ctx context.Context, _ string, dest string, m notify.Message) error {
	if err := validateScheme(dest); err != nil {
		return fmt.Errorf("webhooknotify: %w", err)
	}
	for _, d := range m.Digests {
		if err := n.post(ctx, dest, d); err != nil {
			return err
		}
	}
	return nil
}

func (n *Notifier) post(ctx context.Context, dest string, d notify.Digest) error {
	body, err := json.Marshal(payload{SavedSearchName: d.SavedSearchName, Total: d.Total, Jobs: d.Jobs})
	if err != nil {
		return fmt.Errorf("webhooknotify: encode payload: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, dest, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("webhooknotify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := n.http.Do(req)
	if err != nil {
		return fmt.Errorf("webhooknotify: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusGone {
		return fmt.Errorf("%w: webhook destination responded 410 Gone", notify.ErrRecipientGone)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("webhooknotify: destination responded %d", resp.StatusCode)
	}
	return nil
}

// TestSearchName is the saved_search_name a probe carries, so a receiver can
// tell it apart from a real digest.
const TestSearchName = "freehire webhook test"

// Test POSTs a sample digest with no jobs to dest and reports the answer with Send's error
// vocabulary, proving a URL accepts our payload before it is relied on. The empty job list is
// non-nil so it encodes as [], not null.
func (n *Notifier) Test(ctx context.Context, dest string) error {
	return n.Send(ctx, notify.ChannelWebhook, dest, notify.Message{Digests: []notify.Digest{{
		SavedSearchName: TestSearchName,
		Jobs:            []notify.DigestJob{},
	}}})
}

// validateScheme rejects anything but http/https before a request is built. Defense in depth
// beside the API layer's creation-time check; recipient() does not otherwise validate dest.
func validateScheme(raw string) error {
	u, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("invalid destination url: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("unsupported destination url scheme %q", u.Scheme)
	}
	return nil
}
