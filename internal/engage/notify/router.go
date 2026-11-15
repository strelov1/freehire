package notify

import (
	"context"
	"errors"
	"fmt"
)

// ChannelEmail delivers a digest as an email.
const ChannelEmail = "email"

// ErrChannelNotConfigured is returned by Router.Send when a subscription's channel
// has no registered notifier (e.g. email while unconfigured). The delivery loop
// soft-skips it: the matches stay pending and no failed attempt is counted.
var ErrChannelNotConfigured = errors.New("notify: channel not configured")

// ErrRecipientGone is returned by a Notifier that learned from the send itself that
// this recipient will not accept messages again (e.g. the Telegram bot was blocked).
//
// Unlike ErrChannelNotConfigured, which is absent for everyone, the engine also
// forgets the link before soft-skipping, so the recipient stops being deliverable
// instead of failing on every digest.
//
// It is a channel-level sentinel because internal/engage/telegramnotify imports this
// package and cannot be imported back; telegramnotify.SendToChat translates a 403
// into it, and the nudge and reminder engines alias it.
var ErrRecipientGone = errors.New("notify: recipient will not accept messages")

// Router is a Notifier that dispatches to the notifier registered for the
// subscription's channel, keeping the engine channel-agnostic. A channel with no
// notifier yields ErrChannelNotConfigured.
type Router map[string]Notifier

var _ Notifier = (Router)(nil)

// Send routes to the notifier registered for channel, or returns
// ErrChannelNotConfigured when none is.
func (r Router) Send(ctx context.Context, channel, dest string, m Message) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, m)
}
