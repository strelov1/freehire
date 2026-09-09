## Why

A subscriber replied to a campaign asking why there is no way to turn the mail off
without signing in, and said he did not think that was legal. He is right on both
counts. Every non-essential mail we send closes with a link to `/my/notifications`,
which is behind the account gate (`internal/application/mailtpl/mailtpl.go:330`); no
message carries a `List-Unsubscribe` header, because the SES transport sends a bare
`SendEmail` with no custom headers at all (`internal/engage/emailnotify/client.go`).

That fails three separate bars:

- **CAN-SPAM** requires an opt-out that costs the recipient nothing beyond replying
  or visiting a single page. Requiring an account login is an extra step it does not
  permit. **GDPR Art. 7(3)** requires withdrawing consent to be as easy as giving it.
- **Gmail and Yahoo bulk-sender rules** (in force since February 2024) require
  RFC 8058 one-click unsubscribe — `List-Unsubscribe` plus
  `List-Unsubscribe-Post: List-Unsubscribe=One-Click`. Without them our mail is
  scored as a sender that will not let people leave, which costs deliverability for
  the digests candidates actually asked for.
- **The one flag we do have is overloaded and reads two opposite ways.**
  `notification_settings.enabled` gates lifecycle nudges through an inner join
  (`queries/nudges.sql:12` — a missing row means *do not send*) and gates campaigns
  and the onboarding drip through `COALESCE(ns.enabled, true)`
  (`queries/broadcast.sql:20`, `queries/onboarding.sql:23` — a missing row means
  *send*). So a person cannot stop letters from the founder without also stopping
  their application follow-up reminders, and a person who never opened the settings
  page has no row at all, which makes the campaign gate unconditionally true for
  them. That is exactly the complainant's position.

## What Changes

- **A public email preference page reachable from every non-essential mail**, with
  no sign-in. Its link carries a signed token naming one user and one mail group.
  The page shows the address, three group switches — job alerts, notifications,
  news — a per-saved-search switch under job alerts, and one "unsubscribe from
  everything" button. It shows nothing else about the account.
- **Every non-essential mail gains `List-Unsubscribe` and `List-Unsubscribe-Post`
  headers**, so the client's own unsubscribe button works. A one-click POST turns
  off only the group the mail belongs to, never the whole account.
- **The mail groups become explicit**: `alerts` (saved-search digests), `activity`
  (saved-job reminders, lifecycle nudges, reports), `news` (campaigns, the
  onboarding sequence, referral pings). Verification and password-reset mail is
  `essential` and carries no unsubscribe affordance, as today.
- **`notification_settings.enabled` stops being overloaded.** Two columns join it —
  `alerts_email_enabled` and `news_email_enabled` — so campaigns can be declined
  without silencing application reminders. `enabled` keeps its present meaning and
  its present readings for `activity` only; no existing row changes value.
- **BREAKING (internal only)**: `emailnotify.Client` today exposes three
  near-identical send methods (`Send`, `SendWithReplyTo`, `SendWithAttachments`).
  They collapse into one `Send(ctx, Message)` taking a struct, which is where the
  new headers live. Six consumer-side interfaces and the mail-preview capture are
  updated with it. No wire or schema contract changes.
- **A guard test** walks the module's AST and fails if any `mailtpl.Body` literal
  neither sets `Essential: true` nor receives an unsubscribe URL, so a mail added
  later cannot ship without a way out of it.

## Capabilities

### New Capabilities

- `email-preference-center`: the signed-token unsubscribe link, the public
  preference page and its endpoints, the RFC 8058 one-click target, the
  `List-Unsubscribe` headers, and the three-group mail taxonomy that decides which
  switch a given mail answers to.

### Modified Capabilities

- `notification-settings`: the single account-level flag splits into three
  independent group switches. The existing `enabled` flag narrows to the `activity`
  group only, and two new flags own `alerts` and `news`. The "new accounts default
  to enabled" requirement is restated per group, since the two new flags default to
  on for every account while `enabled` keeps the behaviour it has today.
- `email-notify`: every rendered non-essential mail SHALL carry an unsubscribe URL
  in its footer and `List-Unsubscribe` headers on the message; the footer's present
  unconditional "Turn off these notifications" link to the account page is replaced.
- `filter-subscriptions`: a saved-search subscription becomes deactivatable through
  a signed link without authentication, and the whole `alerts` group gains a master
  switch above the per-subscription ones.

## Impact

**New**: `internal/engage/emailprefs` (mint/parse the signed token — no storage),
`internal/api/handler/emailprefs.go` (three unauthenticated, rate-limited routes),
`web/src/routes/unsubscribe/+page.svelte` (public, noindex).

**Schema**: one migration adding `alerts_email_enabled` and `news_email_enabled` to
`public.notification_settings`, both `NOT NULL DEFAULT true`. Additive; no backfill;
no existing row's behaviour changes.

**Modified**: `internal/engage/emailnotify` (transport struct, headers),
`internal/application/mailtpl` (an `UnsubscribeURL` field and the footer that reads
it), and the seven senders that render through it — `emailnotify/notifier`,
`reminder`, `nudge`, `report`, `broadcast`, `onboarding`, `referral/pinger` — plus
`engage/mailpreview`. Audience queries in `queries/broadcast.sql`,
`queries/onboarding.sql`, and the digest selection in `queries/notifications.sql`
gain the new gates. `internal/platform/arch/layering/blocks.go` gains the new
package.

**Deliverability**: adding the headers is the half of the Gmail/Yahoo bulk-sender
requirement we currently fail; it should improve inbox placement for the digests,
not just satisfy the rule.

**Not in scope**: Telegram, push, and webhook channels keep their existing controls.
This change is about email, which is the only channel a stranger can receive without
having connected anything.
