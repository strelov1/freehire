## Why

Filter-subscription job alerts deliver only over Telegram today, which requires
users to link a chat with the bot. Email is the lowest-friction channel almost
every user already has, so offering it as a second delivery channel lets people
enable job alerts without Telegram — reaching users the current channel misses.

## What Changes

- Add an **email delivery channel** for filter-subscription digests, alongside
  Telegram. A user can enable email alerts per saved search from the same
  "My subscriptions" surface.
- New `internal/emailnotify` package implementing `notify.Notifier`: renders a
  digest to an HTML + plain-text email and sends it via **AWS SES v2**
  (`sesv2.SendEmail`) from a configured `notifications@freehire.dev`-style
  address. Credentials resolve from the default AWS chain (never app config),
  mirroring the apply service's `source_ses.go`.
- A `notify.Router` that dispatches a digest to the right channel `Notifier` by
  `channel`, so the delivery engine stays channel-agnostic. `cmd/notify`
  registers Telegram (when the bot is configured) and email (when SES is
  configured); an unconfigured channel soft-skips rather than failing.
- Email deliveries go to the user's **account email**, resolved live at delivery
  time (`GetSubscriptionForDelivery` joins `users.email`); `subscriptions.destination`
  stays NULL for email, mirroring how Telegram resolves the live `chat_id`. No
  custom-address input and no per-address verification flow.
- Config knobs `AWS_REGION` + `NOTIFY_EMAIL_FROM`; both empty ⇒ email channel
  disabled and the worker still delivers Telegram. Adds `aws-sdk-go-v2` config +
  `sesv2` to `go.mod`.
- Frontend: an email toggle per saved search in `SavedSearchesView`.
- No DB migration — the `channel`/`destination` columns and the
  `UNIQUE (saved_search_id, channel)` constraint already accommodate a second
  channel.

## Capabilities

### New Capabilities
- `email-notify`: render a subscription digest to an email and send it via AWS
  SES to the subscriber's account email; the email-channel sibling of
  `telegram-notify`.

### Modified Capabilities
- `filter-subscriptions`: the subscription channel allowlist accepts `email`;
  the delivery engine routes each digest to a per-channel notifier and resolves
  the account email as the destination for email subscriptions.

## Impact

- **Code (this repo):** new `internal/emailnotify`; `internal/notify` (Router,
  `ChannelEmail`, `recipient` email branch); `internal/subscription`
  (`validChannels` gains `email`); `internal/db/queries/subscriptions.sql`
  (`GetSubscriptionForDelivery` joins `users.email`) + regenerated sqlc;
  `internal/config`; `cmd/notify`; `web/` (`SavedSearchesView` + subscription
  client state).
- **Dependencies:** `github.com/aws/aws-sdk-go-v2/{config,service/sesv2}`.
- **Ops (external, `../freehire-ops`):** an SES sending identity for
  `freehire.dev` + DKIM DNS records; an IAM principal permitting `ses:SendEmail`
  scoped to the From address; and an **SES production-access request** (out of
  sandbox) so mail can be sent to arbitrary account emails. These are deployment
  prerequisites, documented in tasks — the email channel stays disabled until
  `AWS_REGION`/`NOTIFY_EMAIL_FROM` are set in the worker env.
- **Out of scope (noted seams):** custom per-subscription email addresses (would
  need verification); bounce/complaint handling via SES→SNS.
