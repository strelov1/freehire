// Package pushnotify sends mobile push notifications through the Expo Push
// API. No APNs/FCM credentials are needed — Expo's relay handles both
// platforms from one token format, keyed to the app's own Expo project. This
// is intentionally a bare send path: retry/backoff belongs to whichever
// notification engine eventually adopts push as a channel, matching how
// email-notify and telegram-notify also carry no retry logic of their own.
package pushnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// defaultExpoPushURL is Expo's push-send endpoint.
const defaultExpoPushURL = "https://exp.host/--/api/v2/push/send"

// sendTimeout bounds one round trip to Expo.
const sendTimeout = 10 * time.Second

// deviceNotRegistered is the Expo error code meaning the OS itself revoked
// the token (app uninstalled, permission pulled) — Expo will never
// successfully deliver to it again.
const deviceNotRegistered = "DeviceNotRegistered"

// Notifier sends one push message to a device token.
type Notifier interface {
	Send(ctx context.Context, token, title, body string) error
}

// TokenPruner removes a token that's been reported permanently
// undeliverable. *db.Queries satisfies this via its generated
// PruneDeadPushToken method.
type TokenPruner interface {
	PruneDeadPushToken(ctx context.Context, token string) error
}

// ExpoNotifier implements Notifier over the Expo Push API.
type ExpoNotifier struct {
	client *http.Client
	pruner TokenPruner
	apiURL string
}

// NewExpoNotifier returns a Notifier that sends through Expo's push API and
// prunes tokens Expo reports as permanently dead via pruner.
func NewExpoNotifier(pruner TokenPruner) *ExpoNotifier {
	return &ExpoNotifier{
		client: &http.Client{Timeout: sendTimeout},
		pruner: pruner,
		apiURL: defaultExpoPushURL,
	}
}

type expoMessage struct {
	To    string `json:"to"`
	Title string `json:"title"`
	Body  string `json:"body"`
}

type expoReceipt struct {
	Status  string `json:"status"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoResponse struct {
	Data []expoReceipt `json:"data"`
}

// Send posts one message to Expo's push API. A DeviceNotRegistered receipt
// prunes the token and reports success (there's nothing left for the caller
// to retry); any other failure is returned as an error with the token left
// in place, since it may be transient.
func (n *ExpoNotifier) Send(ctx context.Context, token, title, body string) error {
	payload, err := json.Marshal([]expoMessage{{To: token, Title: title, Body: body}})
	if err != nil {
		return fmt.Errorf("pushnotify: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, n.apiURL, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("pushnotify: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	resp, err := n.client.Do(req)
	if err != nil {
		return fmt.Errorf("pushnotify: send: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("pushnotify: expo: %s", resp.Status)
	}

	var out expoResponse
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&out); err != nil {
		return fmt.Errorf("pushnotify: decode response: %w", err)
	}
	if len(out.Data) != 1 {
		return fmt.Errorf("pushnotify: expo returned %d receipts, want 1", len(out.Data))
	}

	receipt := out.Data[0]
	if receipt.Status == "ok" {
		return nil
	}
	if receipt.Details.Error == deviceNotRegistered {
		if err := n.pruner.PruneDeadPushToken(ctx, token); err != nil {
			return fmt.Errorf("pushnotify: prune dead token: %w", err)
		}
		return nil
	}
	return fmt.Errorf("pushnotify: expo: %s (%s)", receipt.Message, receipt.Details.Error)
}
