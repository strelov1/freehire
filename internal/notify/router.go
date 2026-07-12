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
