// Package pushnotify sends mobile push notifications through the Expo Push
// API. No APNs/FCM credentials are needed — Expo's relay handles both
// platforms from one token format, keyed to the app's own Expo project. This
// is intentionally a bare send path: retry/backoff belongs to whichever
// notification engine eventually adopts push as a channel, matching how
// email-notify and telegram-notify also carry no retry logic of their own.
//
// Expo's API is two stages, not one. Send's immediate response is a
// per-message TICKET, not a final delivery outcome — a token already known
// dead (from an earlier attempt) can show up as DeviceNotRegistered right
// away, but a token that just went dead for the first time (app freshly
// uninstalled, permission freshly revoked) is invisible until Expo has
// actually attempted delivery through APNs/FCM, which is only answered
// later via getReceipts. Send enqueues every successfully-sent ticket for a
// later CheckReceipts pass instead of assuming the ticket is the final word.
package pushnotify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// ErrTokenPruned is returned by Send when the token turned out to be dead
// and was removed instead of anything being delivered. A caller that only
// checks err != nil would treat this the same as a real failure, which is
// wrong for something like a self-test push endpoint — "your push arrived"
// and "the token you just registered was already dead" are different
// outcomes a caller may need to tell apart. Check with errors.Is.
var ErrTokenPruned = errors.New("pushnotify: token pruned, not delivered")

// defaultExpoPushURL and defaultExpoReceiptsURL are Expo's two endpoints.
const (
	defaultExpoPushURL     = "https://exp.host/--/api/v2/push/send"
	defaultExpoReceiptsURL = "https://exp.host/--/api/v2/push/getReceipts"
)

// sendTimeout bounds one round trip to Expo.
const sendTimeout = 10 * time.Second

// deviceNotRegistered is the Expo error code meaning the OS itself revoked
// the token (app uninstalled, permission pulled) — Expo will never
// successfully deliver to it again.
const deviceNotRegistered = "DeviceNotRegistered"

// receiptMinAgeMinutes is how long a ticket waits in the outbox before its
// receipt is checked — Expo's own guidance is that delivery through APNs/FCM
// needs time to resolve, so checking immediately would just find nothing yet.
const receiptMinAgeMinutes = 15

// receiptBatchSize bounds one CheckReceipts pass. Expo's getReceipts caps a
// single request at 1000 ids; this stays well under that.
const receiptBatchSize = 100

// Notifier sends one push message to a device token.
type Notifier interface {
	Send(ctx context.Context, token, title, body string, data map[string]string) error
}

// TokenPruner removes a token that's been reported permanently
// undeliverable. *db.Queries satisfies this via its generated
// PruneDeadPushToken method.
type TokenPruner interface {
	PruneDeadPushToken(ctx context.Context, token string) error
}

// TicketQueuer queues a successfully-sent ticket for a later receipt check.
type TicketQueuer interface {
	EnqueuePushTicket(ctx context.Context, token, ticketID string) error
}

// Ticket is one queued send ticket awaiting a receipt check.
type Ticket struct {
	ID       int64
	Token    string
	TicketID string
}

// TicketStore is the outbox CheckReceipts drains.
type TicketStore interface {
	// ClaimDuePushTickets returns tickets at least minAgeMinutes old, oldest
	// first, up to batchSize.
	ClaimDuePushTickets(ctx context.Context, minAgeMinutes, batchSize int32) ([]Ticket, error)
	// DeletePushTickets removes tickets whose receipt has been checked,
	// regardless of outcome — this queue has no retry bookkeeping of its own.
	DeletePushTickets(ctx context.Context, ids []int64) error
}

// SendToDevices sends one title/body/data message to every token, aggregating
// the per-device outcomes into a single result for a caller that has one
// recipient with several devices (unlike Telegram/email, which have exactly one
// destination). It reports success — nil — as long as at least one device
// received the message; a token pruned as dead is not itself a failure, but if
// every device ends up pruned or erroring (including the degenerate case of no
// tokens at all), the caller has nothing delivered and gets an error back.
func SendToDevices(ctx context.Context, notifier Notifier, tokens []string, title, body string, data map[string]string) error {
	var sent int
	var lastErr error
	for _, token := range tokens {
		if err := notifier.Send(ctx, token, title, body, data); err != nil {
			lastErr = err
		} else {
			sent++
		}
	}
	if sent > 0 {
		return nil
	}
	if lastErr != nil {
		return fmt.Errorf("pushnotify: no device received the message: %w", lastErr)
	}
	return errors.New("pushnotify: no device to send to")
}

// ExpoNotifier implements Notifier over the Expo Push API.
type ExpoNotifier struct {
	client      *http.Client
	pruner      TokenPruner
	queuer      TicketQueuer
	tickets     TicketStore
	apiURL      string
	receiptsURL string
}

// NewExpoNotifier returns a Notifier that sends through Expo's push API,
// queuing every successful send (via queuer) for a later CheckReceipts pass
// (drained from tickets), and pruning (via pruner) any token a receipt or an
// immediate send ticket reports as permanently dead.
func NewExpoNotifier(pruner TokenPruner, queuer TicketQueuer, tickets TicketStore) *ExpoNotifier {
	return &ExpoNotifier{
		client:      &http.Client{Timeout: sendTimeout},
		pruner:      pruner,
		queuer:      queuer,
		tickets:     tickets,
		apiURL:      defaultExpoPushURL,
		receiptsURL: defaultExpoReceiptsURL,
	}
}

type expoMessage struct {
	To    string            `json:"to"`
	Title string            `json:"title"`
	Body  string            `json:"body"`
	Data  map[string]string `json:"data,omitempty"`
}

// expoTicket is one entry in Send's response — an immediate acknowledgement,
// not a final delivery outcome (see the package doc).
type expoTicket struct {
	ID      string `json:"id"`
	Status  string `json:"status"`
	Message string `json:"message"`
	Details struct {
		Error string `json:"error"`
	} `json:"details"`
}

type expoSendResponse struct {
	Data []expoTicket `json:"data"`
}

// Send posts one message to Expo's push API. A ticket already reporting
// DeviceNotRegistered prunes the token immediately and reports success
// (there's nothing left to check later); an `ok` ticket is queued for a
// later CheckReceipts pass, since the ticket alone doesn't say the push was
// actually delivered; any other failure is returned as an error with the
// token left in place, since it may be transient.
func (n *ExpoNotifier) Send(ctx context.Context, token, title, body string, data map[string]string) error {
	var out expoSendResponse
	if err := n.postJSON(ctx, n.apiURL, []expoMessage{{To: token, Title: title, Body: body, Data: data}}, &out); err != nil {
		return err
	}
	if len(out.Data) != 1 {
		return fmt.Errorf("pushnotify: expo returned %d tickets, want 1", len(out.Data))
	}

	ticket := out.Data[0]
	if ticket.Status == "ok" {
		if err := n.queuer.EnqueuePushTicket(ctx, token, ticket.ID); err != nil {
			return fmt.Errorf("pushnotify: enqueue ticket: %w", err)
		}
		return nil
	}
	if ticket.Details.Error == deviceNotRegistered {
		if err := n.pruner.PruneDeadPushToken(ctx, token); err != nil {
			return fmt.Errorf("pushnotify: prune dead token: %w", err)
		}
		return ErrTokenPruned
	}
	return fmt.Errorf("pushnotify: expo: %s (%s)", ticket.Message, ticket.Details.Error)
}

// expoReceiptsResponse is getReceipts' shape: a map keyed by ticket id,
// unlike Send's positional array.
type expoReceiptsResponse struct {
	Data map[string]expoTicket `json:"data"`
}

// CheckReceipts claims a batch of tickets old enough for Expo to have a
// delivery answer, checks them, prunes any token a receipt reports
// DeviceNotRegistered, and removes every checked ticket from the outbox
// regardless of outcome. Intended to run on a schedule (cmd/push-receipts).
func (n *ExpoNotifier) CheckReceipts(ctx context.Context) error {
	due, err := n.tickets.ClaimDuePushTickets(ctx, receiptMinAgeMinutes, receiptBatchSize)
	if err != nil {
		return fmt.Errorf("pushnotify: claim due tickets: %w", err)
	}
	if len(due) == 0 {
		return nil
	}

	ids := make([]string, len(due))
	for i, t := range due {
		ids[i] = t.TicketID
	}
	receipts, err := n.getReceipts(ctx, ids)
	if err != nil {
		return fmt.Errorf("pushnotify: get receipts: %w", err)
	}

	processed := make([]int64, 0, len(due))
	for _, t := range due {
		r, ok := receipts[t.TicketID]
		if !ok {
			// No answer yet (or Expo no longer recognizes the id) — this is
			// not "checked, resolved", so the ticket stays queued rather
			// than losing its only detection window. The next scheduled
			// pass claims it again; no special retry bookkeeping needed
			// since it's already past the minimum age.
			continue
		}
		if r.Details.Error == deviceNotRegistered {
			if err := n.pruner.PruneDeadPushToken(ctx, t.Token); err != nil {
				return fmt.Errorf("pushnotify: prune dead token: %w", err)
			}
		}
		processed = append(processed, t.ID)
	}

	if err := n.tickets.DeletePushTickets(ctx, processed); err != nil {
		return fmt.Errorf("pushnotify: delete processed tickets: %w", err)
	}
	return nil
}

func (n *ExpoNotifier) getReceipts(ctx context.Context, ticketIDs []string) (map[string]expoTicket, error) {
	var out expoReceiptsResponse
	if err := n.postJSON(ctx, n.receiptsURL, map[string][]string{"ids": ticketIDs}, &out); err != nil {
		return nil, err
	}
	return out.Data, nil
}

// postJSON POSTs payload as JSON to url and decodes the JSON response into
// out. Shared by Send and getReceipts — Expo's two endpoints differ only in
// URL, request shape, and response shape, not in how the HTTP round trip
// itself is done.
func (n *ExpoNotifier) postJSON(ctx context.Context, url string, payload, out any) error {
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("pushnotify: encode request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
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

	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(out); err != nil {
		return fmt.Errorf("pushnotify: decode response: %w", err)
	}
	return nil
}
