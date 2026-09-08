Each task runs the full micro-cycle: write a failing test first, make it pass,
refactor, run the `simplify` pass, re-run tests, review the diff, and only then
check the box.

## 1. Groundwork

- [x] 1.1 Create the worktree off a freshly fetched `origin/main` (local `migrations/` is behind: it stops at `0145`, `origin/main` is at `0151`)

## 2. The signed token

The layering table entry lands in this group, not before it. The guard is
symmetric: `TestEveryPackageInTheRepoIsAssignedToABlock` fails on a package with no
entry, and `TestBlockTableNamesNoPackageThatDoesNotExist` (`repo_test.go:137`) fails
on an entry with no package. Neither half can ship alone.

- [ ] 2.1 Define the four groups (`alerts`, `activity`, `news`, `essential`) as a closed vocabulary in `internal/engage/emailprefs`, with `essential` rejected by the minter — an essential mail must have no link to mint
- [ ] 2.2 `Mint(userID int64, group Group) string` — `base64url(userID "." group "." HMAC-SHA256)`, keyed by the JWT secret under the fixed salt `"email-prefs-v1"`
- [ ] 2.3 `Parse(token string) (int64, Group, error)` — constant-time signature comparison; distinct sentinel errors for malformed, bad-signature, and unknown-group, all rendered identically by the caller
- [ ] 2.4 Table test: a minted token round-trips; a token with a flipped user id, a flipped group, a truncated signature, or a signature from a different secret is refused; a token minted under the JWT secret alone (no salt) is refused
- [ ] 2.5 Add `emailprefs` to the `engage` block in `internal/platform/arch/layering/blocks.go`, then confirm both halves of the guard and `depguard`: `go test ./internal/platform/arch/layering/` and `golangci-lint run`

## 3. Schema

- [ ] 3.1 Add the migration (take the next free number, re-check against `origin/main` at PR time — `main` has held three `0144` files at once): `ALTER TABLE public.notification_settings ADD COLUMN alerts_email_enabled boolean NOT NULL DEFAULT true, ADD COLUMN news_email_enabled boolean NOT NULL DEFAULT true;`
- [ ] 3.2 `pnpm check:sql` passes on the added file
- [ ] 3.3 Extend the `notification_settings` read/write queries in `internal/platform/db/queries/reminders.sql` to carry both columns, then `make sqlc` — never hand-edit `internal/platform/db/*.sql.go`

## 4. Transport: one send path

- [ ] 4.1 Introduce `emailnotify.Message{From, To, Subject, HTML, Text, ReplyTo, Headers, Attachments}` and `(*Client).Send(ctx, Message)`, mapping `Headers` onto `sesv2/types.Message.Headers`
- [ ] 4.2 Test that a message with no optional parts sends no `ReplyToAddresses`, no custom headers, and no attachments — the current `SendWithReplyTo` already documents that an empty reply-to must send no header at all
- [ ] 4.3 Test that reply-to, headers, and attachments all reach one SES call together
- [ ] 4.4 Delete `Send`, `SendWithReplyTo`, and `SendWithAttachments`; update the six consumer interfaces (`emailnotify.Sender`, `emailnotify.AttachmentSender`, `broadcast.Sender`, `onboarding.Sender`, `report.EmailSender`, `referral.EmailSender`) and `engage/mailpreview`'s capture
- [ ] 4.5 `go vet -tags=integration ./...` — the tagged tests in these packages are not compiled by `go test ./...`

## 5. The mail shell

- [ ] 5.1 Add `UnsubscribeURL` to `mailtpl.Body`; the footer renders "Unsubscribe" plus the settings link when it is set, and neither when `Essential` is true
- [ ] 5.2 Test all three footer states: essential (no links), non-essential with a URL (both links), non-essential without a URL (this must be unreachable — assert the render panics or fails loudly rather than silently omitting the link)
- [ ] 5.3 Confirm `engage/mailpreview` still renders every mail; eyeball the footer in light and dark

## 6. Wire each sender to its group

- [ ] 6.1 `emailnotify/notifier` (digests) mints an `alerts` token, sets `Body.UnsubscribeURL`, and sets both `List-Unsubscribe` headers
- [ ] 6.2 `reminder`, `nudge`, `report` do the same for `activity`
- [ ] 6.3 `broadcast`, `onboarding`, `referral/pinger` do the same for `news`
- [ ] 6.4 `emailnotify/authmailer` stays essential: no token, no URL, no headers — assert this, do not assume it
- [ ] 6.5 Test that the `List-Unsubscribe` header carries both the URL and a `mailto:` alternative, and that `List-Unsubscribe-Post: List-Unsubscribe=One-Click` is present verbatim

## 7. Honour the switches at selection time

- [ ] 7.1 `queries/broadcast.sql` and `queries/onboarding.sql`: move from `COALESCE(ns.enabled, true)` to `COALESCE(ns.news_email_enabled, true)` — this is the split that stops a person losing their nudges when they decline campaigns
- [ ] 7.2 The digest selection gains `COALESCE(ns.alerts_email_enabled, true)`
- [ ] 7.3 Leave the six `JOIN ... AND ns.enabled` nudge predicates untouched; add a test pinning that a missing rule row still means no nudges and yes campaigns
- [ ] 7.4 `make sqlc`, then integration tests over the changed queries

## 8. Public endpoints

- [ ] 8.1 `GET /api/v1/email-prefs?t=` returns `{"data": {email, alerts_enabled, activity_enabled, news_enabled, searches:[{id,name,active}]}}` — no session issued, no other account field present
- [ ] 8.2 `PATCH /api/v1/email-prefs?t=` writes the three group switches and per-subscription `active` flags; it may only deactivate a subscription, never create or reactivate one it was not given
- [ ] 8.3 `POST /api/v1/email-prefs/one-click?t=` accepts the `List-Unsubscribe=One-Click` form body, turns off only the token's group, and returns 200 on a repeat
- [ ] 8.4 An invalid, tampered, or deleted-account token yields one generic failure across all three routes — assert no address or saved-search name appears in any of those responses
- [ ] 8.5 Register the three routes on the public group with a rate limiter, following the existing public-route limiter setup
- [ ] 8.6 Integration tests for all of the above (these live behind `//go:build integration`)

## 9. The public page

- [ ] 9.1 `web/src/routes/unsubscribe/+page.svelte` — public, `noindex`, three group switches, the saved-search list under alerts, and "Unsubscribe from everything"
- [ ] 9.2 A one-click POST's confirmation view names the group that was turned off and links to the full page
- [ ] 9.3 Invalid-token state renders a generic message with no account detail
- [ ] 9.4 `pnpm --dir web lint` and `pnpm --dir web test` (a fresh worktree needs `svelte-kit sync` first, or all web tests fail)

## 10. The authenticated page keeps parity

- [ ] 10.1 `ReminderSettings.svelte` gains the `alerts` and `news` switches beside the existing one, so the signed-in view shows the same three preferences
- [ ] 10.2 The authenticated settings endpoint reads and writes both new columns

## 11. The guard

- [ ] 11.1 A test that walks the module's AST for every `mailtpl.Body` composite literal and fails unless it sets `Essential: true` or `UnsubscribeURL`, naming the offending file and line — no hand-maintained list of senders, which proves consistency rather than coverage
- [ ] 11.2 Verify the guard by mutation: add a bare `mailtpl.Body{}` in a scratch file, confirm the test fails and names it, then remove it

## 12. Ship

- [ ] 12.1 Full local suite: `gofmt -l .` silent, `go vet ./...`, `go test ./...`, `go vet -tags=integration ./...`, `go test -tags=integration ./internal/engage/... ./internal/api/handler/`
- [ ] 12.2 Open the PR; re-check the migration number against `origin/main` first
- [ ] 12.3 Deploy the migration, then the code; confirm with `release.sh`
- [ ] 12.4 Send one real mail to a live address and verify in Gmail: the client shows its own Unsubscribe control, the footer link opens the page without a session, and one click turns off only that group
- [ ] 12.5 Reply to the complainant with his own preference link
- [ ] 12.6 A week later, read SES complaint/bounce rates and Gmail Postmaster spam rate; record whether one-click should widen to all non-essential mail
