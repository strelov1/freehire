package notify

import (
	"context"
	"log"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/strelov1/freehire/internal/engage/pushnotify"
)

// The pieces the three engines share beyond the Router: the one push wrapper
// (pushnotify imports nothing from engage, so it can live here), the destination
// rule and the claim release. The Telegram and email wrappers sit one layer up in
// internal/engage/batchnotify, because those channel packages implement Notifier.
// Each engine keeps its own message type and copy; its Notifier is
// notify.Notifier[[]ItsMessage], one call per account batch.

// Push delivers a batch as ONE mobile push, fanned out to every device the user
// has registered. dest is the user id — Recipient resolves it live from
// HasPushDevice rather than a stored per-item destination.
type Push[M any] struct {
	push   *pushnotify.UserSender
	render func(m M) (title, body, slug string)
}

// PushTokenLister lists a user's registered push devices.
type PushTokenLister = pushnotify.TokenLister

// NewPush builds a Push notifier resolving devices through tokens and sending
// through transport. render names the one item a message deep-links to by its
// slug, or "" when the message carries none or several — the rule every engine
// follows, because a deep link needs one destination.
func NewPush[M any](tokens PushTokenLister, transport pushnotify.Notifier, render func(m M) (title, body, slug string)) *Push[M] {
	return &Push[M]{push: pushnotify.NewUserSender(tokens, transport), render: render}
}

// Send fans ONE notification for the whole message out to all the user's devices.
// The engine hands it a whole batch, which is never empty.
func (n *Push[M]) Send(ctx context.Context, _ string, dest string, m M) error {
	title, body, slug := n.render(m)
	var data map[string]string
	if slug != "" {
		data = map[string]string{"slug": slug}
	}
	return n.push.Send(ctx, dest, title, body, data)
}

// RecipientInfo is the account-and-channel state Recipient decides on. The nudge
// and reminder engines fill it from their own generated delivery rows, which
// differ in shape but carry exactly these fields.
type RecipientInfo struct {
	UserID         int64
	TelegramChatID pgtype.Int8
	AccountEmail   string
	HasPushDevice  bool
}

// Recipient resolves the destination for a channel and reports whether a message
// is deliverable over it right now: Telegram needs a linked chat, email the
// account email, push a registered device (dest is then the user id); any other
// channel is undeliverable.
func Recipient(channel string, r RecipientInfo) (string, bool) {
	switch channel {
	case ChannelTelegram:
		if !r.TelegramChatID.Valid {
			return "", false
		}
		return strconv.FormatInt(r.TelegramChatID.Int64, 10), true
	case ChannelEmail:
		if r.AccountEmail == "" {
			return "", false
		}
		return r.AccountEmail, true
	case ChannelPush:
		if !r.HasPushDevice {
			return "", false
		}
		return strconv.FormatInt(r.UserID, 10), true
	}
	return "", false
}

// Release drops the lease on a claimed item so it is retried promptly on a later
// pass. It runs on a context detached from the run's, bounded at 10s, so a
// cancelled run (SIGTERM) cannot strand the claim until the lease expires.
func Release(ctx context.Context, engine string, id int64, release func(ctx context.Context, id int64) error) {
	releaseCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()
	if err := release(releaseCtx, id); err != nil {
		log.Printf("%s: release claim %d: %v", engine, id, err)
	}
}
