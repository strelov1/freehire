package emailnotify

import (
	"errors"
	"fmt"
	"strings"
	"sync/atomic"

	"github.com/aws/aws-sdk-go-v2/service/sesv2/types"
)

// ErrSendingPaused is what Send returns once SES has refused the whole account
// and the client was built WithSendingBreaker. No SES call was made, and no
// recipient was judged: a caller must neither count the attempt against the item
// nor close it out, and should stop its pass.
var ErrSendingPaused = errors.New("emailnotify: ses is not sending for this account")

// breaker latches on the first account-level refusal. It is safe for concurrent use.
type breaker struct{ tripped atomic.Bool }

// accountRefused reports whether err says SES will not send for the account
// itself, as opposed to refusing one message or address.
func accountRefused(err error) bool {
	var paused *types.SendingPausedException
	var suspended *types.AccountSuspendedException
	if errors.As(err, &paused) || errors.As(err, &suspended) {
		return true
	}
	// A paused account answers MessageRejected "Sending paused for this account."
	// A bad address or an unverified identity is the same type with another
	// message and must not stop everybody else's mail.
	var rejected *types.MessageRejected
	if errors.As(err, &rejected) && rejected.Message != nil {
		return strings.Contains(strings.ToLower(*rejected.Message), "sending paused")
	}
	return false
}

// WithSendingBreaker returns the Client with a per-run circuit breaker: after SES
// reports that the account cannot send, every later Send returns ErrSendingPaused
// without calling SES. Only the cron workers enable it; the long-lived API server
// must not, because a latch there would outlive the outage it recorded.
func (c *Client) WithSendingBreaker() *Client {
	out := *c
	out.breaker = &breaker{}
	return &out
}

// SoftSkipIfPaused turns ErrSendingPaused into skip, the caller's own "channel not
// configured" sentinel, so an engine that soft-skips an unavailable channel leaves
// the item pending and counts no delivery attempt toward dead-lettering. Any other
// error passes through unchanged.
func SoftSkipIfPaused(err, skip error) error {
	if errors.Is(err, ErrSendingPaused) {
		return fmt.Errorf("%w: %w", skip, err)
	}
	return err
}
