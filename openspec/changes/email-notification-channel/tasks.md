## 1. Dependencies & config

- [x] 1.1 Add `github.com/aws/aws-sdk-go-v2/config` and `.../service/sesv2` to `go.mod` (`go get`), tidy, and confirm `go build ./...`.
- [x] 1.2 Add optional config knobs `AWSRegion` (`AWS_REGION`) and `NotifyEmailFrom` (`NOTIFY_EMAIL_FROM`) to `internal/config`; both empty ⇒ email disabled. Document them in `.env.example`.

## 2. Channel routing in the engine

- [x] 2.1 Add `ChannelEmail = "email"` const to `internal/notify` and a `notify.Router` (`map[string]Notifier`) whose `Send` dispatches by channel; an unregistered channel returns a sentinel the delivery loop treats as a soft-skip (matches stay pending, no attempt counted). Unit-test dispatch + unknown-channel soft-skip.
- [x] 2.2 Extend `recipient()` with an `email` branch that returns the account email from the delivery row; keep `destination` NULL. Unit-test that an email subscription resolves the account email and a missing email soft-skips.

## 3. Account-email resolution (DB)

- [x] 3.1 Modify `GetSubscriptionForDelivery` in `internal/db/queries/subscriptions.sql` to also select `users.email`; run `make sqlc` and commit generated code.
- [x] 3.2 Wire the new email field through `internal/notify` delivery (`GetSubscriptionForDeliveryRow` → `recipient()`); adjust the `Store` interface/fakes as needed.

## 4. Email notifier (`internal/emailnotify`)

- [x] 4.1 Create the package with a `Notifier` implementing `notify.Notifier`: `render(Digest)` → subject (`N new jobs for "<search>"`) + HTML body + plain-text alternative, following the email template in design.md (`html/template` auto-escaping, centered ~600px table, inline styles, hidden preheader, brand header, one row per job linking to `<origin>/jobs/<slug>?utm_source=email` with company + salary suffixes, "and N more → View all" tail, footer with a Manage-alerts link to `<origin>/my/notifications`). Unit-test render (subject, auto-escaping of a hostile title, cap + "and N more", salary suffix, plain-text alternative parity).
- [x] 4.2 Implement `Send` via `sesv2.SendEmail` behind a small SES-client interface (From = configured address, To = dest); build the client from `awsconfig.LoadDefaultConfig` (default credential chain). Unit-test `Send` success + error propagation with a fake SES client.
- [x] 4.3 Add a compile-time `var _ notify.Notifier = (*Notifier)(nil)` assertion.

## 5. Wire the worker

- [x] 5.1 In `cmd/notify`, build a `notify.Router`, register the Telegram notifier when the bot is configured and the email notifier when `AWS_REGION`+`NOTIFY_EMAIL_FROM` are set, and pass the router to `notify.New`. Keep "no channels configured ⇒ exit 0 (nothing to deliver)" behavior.

## 6. Subscription channel allowlist

- [x] 6.1 Add `ChannelEmail = "email"` to `internal/subscription` and include it in `validChannels`. Unit-test that an `email` subscription is accepted and an unknown channel returns `ErrInvalidChannel`.

## 7. Frontend

- [x] 7.1 Extend the subscription client state (`web/src/lib/savedSearches.svelte.ts` / `notifications.svelte.ts`) to support the `email` channel (subscribe/unsubscribe, per-saved-search enabled state).
- [x] 7.2 Add an email alert toggle per saved search in `SavedSearchesView.svelte` alongside the Telegram control (email always "enabled"; no address input). Verify via svelte-check + visual.

## 8. Ops prerequisites (`../freehire-ops`, deploy-time)

- [x] 8.1 Add SES sending identity + DKIM for `freehire.dev` (or a `mail.` subdomain) in terraform; publish the DNS records in the zone.
- [x] 8.2 Add an IAM principal with `ses:SendEmail`/`ses:SendRawEmail` scoped to the From address; provision its credentials into the `notify` worker env.
- [x] 8.3 Request SES production access (out of sandbox). Document that the email channel stays config-disabled until this lands.

## 9. Verify

- [x] 9.1 `go build ./... && go vet ./... && go test ./...` green; run the `notify` worker locally with email unset (Telegram path unaffected) and with a fake/SES-configured path to confirm dispatch. Confirm `.env.example` documents the new knobs.
