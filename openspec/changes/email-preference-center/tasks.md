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

- [x] 2.1 Define the four groups (`alerts`, `activity`, `news`, `essential`) as a closed vocabulary in `internal/engage/emailprefs`, with `essential` rejected by the minter — an essential mail must have no link to mint
- [x] 2.2 `Mint(userID int64, group Group) (string, error)` — `<id>.<group>.<base64url(HMAC-SHA256)>`, keyed by the JWT secret under the fixed salt `"email-prefs-v1"`. Only the signature is base64: the other two parts are already URL-unreserved, so an outer encoding would add a decode step and hide nothing. Mint returns an error rather than a bare string, so refusing to mint for `essential` is enforced by the type
- [x] 2.3 `Parse(token string) (int64, Group, error)` — constant-time comparison; ONE `ErrInvalidToken` sentinel with the cause wrapped as context, so a log can tell a rotated secret from junk traffic while the response stays generic. Mint refuses with a separate `ErrCannotMint`: a mint failure is our bug, a parse failure is a stranger's junk, and one sentinel for both would page nobody or page everybody
- [x] 2.4 Table test: a minted token round-trips (including `MaxInt64`); a flipped user id, flipped group, truncated signature, non-base64 signature, or signature from another secret is refused; a signature over the *unsalted* secret is refused; a real session token does not parse here and a token from here does not parse as a session; `Mint` is deterministic, which is the only thing that pins the never-expires property the legal argument rests on
- [x] 2.5 Add `emailprefs` to the `engage` block in `internal/platform/arch/layering/blocks.go`, then confirm both halves of the guard and `depguard`: `go test ./internal/platform/arch/layering/` and `golangci-lint run`

## 3. Schema

- [x] 3.1 Add the migration (take the next free number, re-check against `origin/main` at PR time — `main` has held three `0144` files at once): `ALTER TABLE public.notification_settings ADD COLUMN alerts_email_enabled boolean NOT NULL DEFAULT true, ADD COLUMN news_email_enabled boolean NOT NULL DEFAULT true;`
- [x] 3.2 `pnpm check:sql` passes on the added file
- [x] 3.3 `make sqlc` — `migrations/` is sqlc's schema source, so the migration alone moves the generated code. No query needs editing: `GetNotificationSettings` (`SELECT *`) and `UpsertNotificationSettings` (`RETURNING *`) pick the columns up on the read side by themselves, and the upsert's `ON CONFLICT DO UPDATE SET` names its columns explicitly — leaving the two new ones out of that list is what makes them survive a write from the authenticated settings page, so adding them there would be the bug, not the fix. The narrow writer for the group switches lands in task 8, with its caller.

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

The token is a never-expiring bearer credential and nginx logs query strings
(`deploy/nginx/snippets/freehire-app.conf:14`), so it rides in the URL only where
it has no alternative — see the risk entry in `design.md`.

- [ ] 8.1 `GET /api/v1/email-prefs?t=` returns `{"data": {email, alerts_enabled, activity_enabled, news_enabled, searches:[{id,name,active}]}}` — no session issued, no other account field present. The query is unavoidable here: a link in an email has nowhere else to carry it
- [ ] 8.2 `PATCH /api/v1/email-prefs` writes the three group switches and per-subscription `active` flags; it may only deactivate a subscription, never create or reactivate one it was not given. **The token goes in the body, not the query** — this call has a body already, so there is no reason to log the credential
- [ ] 8.3 `POST /api/v1/email-prefs/one-click?t=` accepts the `List-Unsubscribe=One-Click` form body, turns off only the token's group, and returns 200 on a repeat. The token must stay in the query here: RFC 8058 fixes the body, so the URL is the only place Gmail can carry it — task 12.x switches the access log off for this path instead
- [ ] 8.4 Add the narrow group-switch writer to `queries/reminders.sql` (INSERT … ON CONFLICT DO UPDATE SET only the two group columns) and `make sqlc`. Deliberately NOT folded into `UpsertNotificationSettings`, which is a full replace: routing both writers through it would make the authenticated settings page clobber a choice made from an unsubscribe link, and vice versa
- [ ] 8.5 Iterate `emailprefs.SilenceableGroups()` for "unsubscribe from everything" — never a hand-written `{alerts, activity, news}`, which is the list-checked-against-a-list trap the AST guard exists for
- [ ] 8.6 An invalid, tampered, or deleted-account token yields one generic failure across all three routes — assert no address or saved-search name appears in any of those responses. `Parse` succeeding proves only that we minted the token, never that the account still exists, so the lookup is the caller's job. Decide deliberately whether to equalise the timing: a deleted-account refusal costs a database round-trip and a bad-signature refusal returns at once
- [ ] 8.7 Register the three routes on the public group with a rate limiter, following the existing public-route limiter setup
- [ ] 8.8 Integration tests for all of the above (these live behind `//go:build integration`)

## 9. The public page

- [ ] 9.1 `web/src/routes/unsubscribe/+page.svelte` — public, `noindex`, one switch per `emailprefs.SilenceableGroups()` entry, the saved-search list under alerts, and "Unsubscribe from everything"
- [ ] 9.5 Strip `?t=` from the address bar after the first read, so the token does not travel on into history, a screenshot, or a pasted URL. Use `onRouterReady` — `replaceState` inside `onMount` throws only in a production build, where the error is also unreadable
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
- [ ] 12.3 Turn the access log off for the unsubscribe location in `deploy/nginx/snippets/freehire-app.conf` (the file already does this twice, so the shape exists) — the one-click POST has no way to keep its token out of the URL, so this is the only place left to keep a never-expiring credential out of a bulk store. **Nothing in `deploy/` deploys itself**: copy it to the host and reload nginx, then confirm with `./deploy/check-drift.sh`
- [ ] 12.4 Deploy the migration, then the code; confirm with `release.sh`
- [ ] 12.5 Send one real mail to a live address and verify in Gmail: the client shows its own Unsubscribe control, the footer link opens the page without a session, and one click turns off only that group
- [ ] 12.6 Reply to the complainant with his own preference link
- [ ] 12.7 A week later, read SES complaint/bounce rates and Gmail Postmaster spam rate; record whether one-click should widen to all non-essential mail
