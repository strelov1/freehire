package notify

import (
	"context"
	"errors"
	"fmt"
)

// ChannelEmail delivers a digest as an email (via AWS SES); the email-channel
// sibling of ChannelTelegram.
const ChannelEmail = "email"

// ErrChannelNotConfigured is returned by Router.Send when a subscription's channel
// has no registered notifier (e.g. the email channel while SES is unconfigured).
// The delivery loop treats it as a soft-skip — the matches stay pending and no
// failed attempt is counted — rather than a delivery failure to dead-letter.
var ErrChannelNotConfigured = errors.New("notify: channel not configured")

// ErrRecipientGone is returned by a Notifier whose channel has learned, from the
// send itself, that this recipient will not accept messages again — the Telegram
// bot was blocked or removed, and the chat is permanently closed to us.
//
// Distinct from ErrChannelNotConfigured, which is about the channel being absent
// for everyone, though the delivery loop treats both as a soft-skip. What it adds
// is a side effect: the engine forgets the link before skipping, so the recipient
// stops being deliverable at all rather than failing once per digest forever.
//
// A channel-level sentinel rather than the transport's own: internal/telegramnotify
// imports this package for Digest, so it cannot be imported back. Each engine
// declares its own and its Telegram notifier translates, exactly as they already do
// for ErrChannelNotConfigured.
var ErrRecipientGone = errors.New("notify: recipient will not accept messages")

// Router is a Notifier that dispatches a digest to the per-channel notifier
// registered for the subscription's channel, so the matching engine stays
// channel-agnostic (it depends only on Notifier). A channel with no registered
// notifier yields ErrChannelNotConfigured.
type Router map[string]Notifier

// Compile-time guarantee that Router satisfies the channel abstraction it muxes.
var _ Notifier = (Router)(nil)

// Send routes to the notifier registered for channel, or returns
// ErrChannelNotConfigured when none is.
func (r Router) Send(ctx context.Context, channel, dest string, d Digest) error {
	n, ok := r[channel]
	if !ok {
		return fmt.Errorf("%w: %q", ErrChannelNotConfigured, channel)
	}
	return n.Send(ctx, channel, dest, d)
}
