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

// Notifier delivers one message to a destination over a channel. It is generic
// because the three engines carry different payloads — a Message for a digest,
// a slice for a reminder or nudge batch — and share this Router and the channel
// wrappers in channels.go.
type Notifier[M any] interface {
	Send(ctx context.Context, channel, dest string, m M) error
}

// Router dispatches a message to the notifier registered for its channel.
type Router[M any] map[string]Notifier[M]

var _ Notifier[Message] = (Router[Message])(nil)

// Send routes to the notifier registered for channel, or returns
// ErrChannelNotConfigured when none is.
func (r Router[M]) Send(ctx context.Context, channel, dest string, m M) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, m)
}
