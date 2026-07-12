## Context

Filter-subscription alerts (`internal/notify` engine, `cmd/notify` cron) match new
jobs to a user's saved searches and deliver them as a per-subscription digest.
Delivery goes through a single `notify.Notifier` implemented once, for Telegram
(`internal/telegramnotify`). The engine already anticipated more channels: the
`Notifier.Send(ctx, channel, dest, digest)` signature carries a `channel`, the
`subscriptions` table already has `channel` + `destination` columns with a
`UNIQUE (saved_search_id, channel)` constraint, and `recipient()` already falls
back to `destination` for any non-`telegram` channel. This change fills that seam
with email over AWS SES.

The apply service (`../freehire-apply`) already integrates AWS SES for *inbound*
mail; its `internal/ingest/source_ses.go` establishes the house pattern:
build the AWS client from `awsconfig.LoadDefaultConfig` and resolve credentials
from the default chain, never app config.

## Goals / Non-Goals

**Goals:**
- Add `email` as a second delivery channel for subscription digests, selectable
  per saved search from the existing subscriptions UI.
- Keep the matching engine channel-agnostic — a new channel is a new `Notifier`
  plus a router entry, not an engine change.
- Deliver to the user's account email with no new user input and no per-address
  verification flow.
- Fail safe: an unconfigured email channel leaves Telegram delivery untouched.

**Non-Goals:**
- Custom per-subscription email addresses (would require address verification).
- Bounce/complaint processing (SES→SNS feedback loop).
- Any DB migration — existing columns/constraints suffice.
- Reworking Telegram delivery.

## Decisions

### Router over a fat multi-channel Notifier
Introduce `notify.Router` — a `map[string]Notifier` whose `Send` dispatches by
`channel` to the registered implementation (unknown/absent channel → a sentinel
the engine treats as a soft-skip). `cmd/notify` builds the router, registering
Telegram when the bot is configured and email when SES is configured.
*Alternative:* make the engine hold `map[channel]Notifier` directly — rejected;
the router keeps the engine's dependency a single `Notifier` (unchanged) and
isolates channel selection in one small, testable unit.
*Alternative:* one Notifier with an internal switch — rejected; it re-couples the
engine to concrete channels, the opposite of the `Notifier` seam's intent.

### Resolve the account email live, keep `destination` NULL
`GetSubscriptionForDelivery` joins `users.email`; `recipient()` gains an `email`
branch returning that address (`destination` stays NULL, exactly like Telegram
resolves the live `chat_id`). *Alternative:* store the email in `destination` at
subscribe time — rejected; it snapshots an address that goes stale when the user
changes their account email, and invites a custom-address feature we are
explicitly deferring. Live resolution mirrors the existing Telegram pattern and
needs no schema change.

### `internal/emailnotify` as the sibling of `telegramnotify`
A new package implementing `notify.Notifier`: `render(Digest)` builds an HTML +
text body (capping the job list with an "and N more" tail, links to
`<origin>/jobs/<slug>?utm_source=email`), and `Send` posts it via
`sesv2.SendEmail`. The SES call sits behind a tiny interface so the unit test
injects a fake, matching `telegramnotify/notifier_test.go`. Salary formatting is
small and self-contained; it is duplicated rather than prematurely extracted (the
two renderers differ in markup), leaving a note if a third channel appears.

### Email template: `html/template`, table layout, inline styles
Unlike the Telegram renderer's manual `html.EscapeString`, the HTML body is built
with `html/template` so every interpolated field (job title, company, saved-search
name) is contextually auto-escaped — the injection guard is structural, not a
call the author can forget. The markup follows email-client constraints:
a single centered ~600px `<table>`, all styling **inline** (no `<style>`/external
CSS, no JS, no remote images), a system font stack. Structure, top to bottom:
- a hidden **preheader** line (inbox preview text),
- a **freehire** brand header,
- the digest heading `🔔 N new jobs for "<search>"`,
- one **row per job**: the title as a link to the on-platform job page, then
  ` — Company` and a ` · salary` suffix when known (same fields as Telegram, no
  new query columns),
- an **"and N more" → View all** tail when the list is capped,
- a **footer** with a "Manage alerts" link to `<origin>/my/notifications` and the
  freehire wordmark.
The subject is `N new jobs for "<search>"`. A plain-text alternative mirrors the
same content (title / company / salary / link per job) so non-HTML clients and
spam scorers see a real body. The template is a package-level `template.Must`
parse (compiled once). The digest job cap is a package constant (start at 20,
matching Telegram's `DigestCap`); the true total still drives the "and N more" tail.

### Config-gated, credentials from the default chain
New optional knobs `AWS_REGION` + `NOTIFY_EMAIL_FROM`. Both empty ⇒ email channel
not registered. Credentials come from the default AWS chain (env/role), never
config — same as apply. This makes the feature a no-op in dev and until ops is
provisioned.

## Risks / Trade-offs

- **SES sandbox blocks sending to arbitrary recipients** → Request SES production
  access before enabling in prod; until then only verified addresses receive
  mail. Documented as a deploy prerequisite; the channel stays disabled via
  config until ready, so nothing breaks.
- **Account email may be unverified (password signups)** → Sending job digests to
  the address a user registered with is low-risk (self-addressed, opt-in per
  saved search). Accepted; custom addresses (which would need verification) are
  out of scope.
- **No bounce handling yet** → At current volumes a hard-bounce won't threaten the
  sending reputation immediately; SES→SNS feedback is a noted seam to add before
  scale.
- **Stale account-email snapshot avoided** → Live join means a changed email just
  works on the next pass; the trade-off is one extra join column in the delivery
  query, which is negligible.

## Migration Plan

No DB migration. Deploy order:
1. **Ops (`../freehire-ops`):** add the SES sending identity + DKIM for
   `freehire.dev`, publish the DNS records, add an IAM principal with
   `ses:SendEmail` scoped to the From address, and request SES production access.
2. Ship the binary (email channel still dormant — no env set).
3. Set `AWS_REGION` + `NOTIFY_EMAIL_FROM` (+ AWS creds) in the `notify` worker env
   once SES is verified and out of sandbox. The next cron pass begins delivering
   email digests.
- **Rollback:** unset `NOTIFY_EMAIL_FROM`/`AWS_REGION`; the channel goes dormant
  and email subscriptions soft-skip (matches stay pending), Telegram unaffected.

## Open Questions

- Exact From identity: `notifications@freehire.dev` vs a `mail.` subdomain — an
  ops/DNS choice that does not affect the code (it reads `NOTIFY_EMAIL_FROM`).
- Whether to show an "email pending SES provisioning" state in the UI before prod
  access lands, or simply not surface email until it's live. Default: surface the
  toggle; deliveries soft-skip until configured.
