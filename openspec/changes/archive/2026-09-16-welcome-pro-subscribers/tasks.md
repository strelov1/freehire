## 1. Schema

- [x] 1.1 Add migration `migrations/0160_users_pro_welcome_sent_at.sql`: nullable
      `users.pro_welcome_sent_at timestamptz`, no backfill.
- [x] 1.2 Add the sqlc queries the worker needs in `internal/platform/db/queries/users.sql`
      (or `plan.sql`): a bounded, oldest-unwelcomed-first page of accounts where
      `pro_welcome_sent_at IS NULL` and (`pro_until` or `ultra_until`) reaches beyond now,
      and a `SetProWelcomeSent(id) ` write guarded by `pro_welcome_sent_at IS NULL`. Run
      `make sqlc` and commit the regenerated `internal/platform/db`.

## 2. `tier` on the signed-in user

- [x] 2.1 Add `Tier plan.Tier` (JSON `tier`) to `userResponse` in
      `internal/api/handler/auth.go`.
- [x] 2.2 Add `h.toUserResponseWithTier(ctx, u)` on `authHandlers`, wrapping the still-pure
      `toUserResponse` and resolving the tier via `h.queries.GetPlanUntils` +
      `plan.TierOf`, defaulting to `plan.TierFree` when `queries` is unset or the read
      fails (logged, never failing the response). Updated every response call site
      (register, login, oauth callback, timezone/language updates, password recovery,
      `Me`) to call it instead of the bare `toUserResponse`.
- [x] 2.3 Integration test (`internal/api/handler/auth_tier_integration_test.go`,
      `-tags=integration`): a free account's `/auth/me` carries `tier: "free"`; an account
      granted Pro carries `tier: "pro"`.
- [x] 2.4 Add `tier: 'free' | 'pro' | 'ultra'` to the `User` interface in
      `web/src/lib/types.ts`.

## 3. Header badge

- [x] 3.1 Add a small "PRO"/"ULTRA" badge to the desktop profile icon in
      `web/src/lib/components/HeaderMenu.svelte` (the `CircleUser` link, `hidden
      sm:inline-flex`), driven by `currentUser()?.tier`, absent for `free` and for a
      signed-out visitor.
- [x] 3.2 Source-text audit test (`HeaderMenu.test.ts`, matching the repo's existing
      no-DOM-testing convention — see `jobActionStrip.test.ts`'s own comment): pins the
      badge's `free` default, that it renders only for a non-free tier, that it attaches
      to the desktop-only profile icon rather than the mobile drawer's Profile row, and
      that the icon's accessible name reflects the tier. `npx svelte-check` also confirmed
      no type errors from the new `User.tier` field.

## 4. Welcome email

- [x] 4.1 Create `internal/engage/prowelcome` (package `prowelcome`): a `Sender` interface
      matching `emailnotify.Message`, and a `Mailer` built the same way
      `internal/engage/onboarding.Mailer` is (`NewMailer(sender, from, replyTo, baseURL,
      links)`), reusing `internal/application/mailtpl.Layout` for the shell.
- [x] 4.2 Write the welcome email's copy and rendering: personal/first-person tone, states
      the tier (Pro/Ultra) and the date it runs to; links to the Discord community and to
      the founder's public profile; includes the ordinary footer (unsubscribe/prefs)
      `mailtpl.Layout` already provides. `render`/`Send` refuse `plan.TierFree`
      (`ErrNotPaying`) — this letter has nothing to say to an account that isn't paying.
- [x] 4.3 Render tests (`mailer_test.go`, mirrors `emailnotify/notifier_test.go`'s
      fake-sender pattern): subject/HTML/text mention the correct tier and the entitlement's
      end date; `ReplyTo` is the configured human inbox, not the sending address; Discord
      and LinkedIn links are present; both `render` and `Send` refuse `TierFree`.
- [x] 4.4 `internal/engage/prowelcome/runner.go`: `Store`/`mailer` interfaces (testable
      without Postgres, same shape `onboarding.Store` uses), `Runner.Run` pages candidates
      via the 1.2 query, resolves each row's tier with `plan.TierOf` (Ultra's `until` wins
      when both are live, same rule `plan.TierOf` itself uses), sends via `Mailer`, and on
      success claims `SetProWelcomeSent`; a failed send OR a failed claim is counted and
      left unclaimed so a later run retries — deliberately the opposite of
      `onboarding.Runner.deliver`'s "burn the slot on failure", which is right for a
      courtesy sequence and wrong for a payment confirmation. `runner_test.go` covers both
      outcomes plus the tier-resolution and listing-error-aborts-the-pass cases.
- [x] 4.5 `cmd/pro-welcome-mail/main.go` on the `internal/platform/worker` bootstrap
      (`Bootstrap`/`Main`), reading `AWS_REGION`/`NOTIFY_EMAIL_FROM`/`ONBOARDING_REPLY_TO`/
      `FRONTEND_ORIGIN`/`JWT_SECRET` (the same set `cmd/onboarding` reads — one reply
      inbox, not a second one that could disagree) plus `PRO_WELCOME_MAX_PER_RUN` (default
      200, via `worker.EnvInt32`); a missing transport var is a no-op that never opens the
      pool, a missing reply-to is a hard failure (same as `cmd/onboarding`), and a failed
      send exits 1 so the timer surfaces it.
- [x] 4.6 Integration test (`store_integration_test.go`, `-tags=integration`, real
      Postgres): a newly-paying account is welcomed exactly once across two runs; a failed
      send leaves the row selectable and a later run retries it successfully; a free
      account is never a candidate; an upgrade (both Pro and Ultra live) resolves to Ultra
      end to end through the real SQL, not just the Go-side unit test.

## 5. Deploy record (no host changes)

- [x] 5.1 Added `deploy/systemd/freehire-pro-welcome-mail.service` and
      `freehire-pro-welcome-mail.timer` (every 10 minutes, `Persistent=true`), matching
      `freehire-discord-sync.*`'s shape — plus a second `EnvironmentFile=` for
      `.env.notify` (`NOTIFY_EMAIL_FROM` lives only there per `deploy/AGENTS.md`'s
      "two-file env split" trap). Added `pro-welcome-mail` to `release.sh`'s worker-binary
      build list, to the "seven workers that send mail" checklist in `deploy/AGENTS.md`,
      to `internal/platform/arch/layering/blocks.go` (new `prowelcome` package, `engage`
      block) and to the `internal/engage/AGENTS.md`/root `AGENTS.md` package lists — the
      layering guard (`go test -tags=integration,llmlive
      ./internal/platform/arch/layering/...`) passes. Installing the unit on host2 and
      running `systemctl daemon-reload` is a manual follow-up per `deploy/AGENTS.md`
      ("nothing here deploys itself") — noted for the PR description, not done here.

## 6. Verification

- [x] 6.1 `gofmt -l` on every changed/new Go file (clean); `go vet ./...` (clean); full
      `go test ./...` (clean, exit 0).
- [x] 6.2 `go vet -tags=integration ./...` (clean); ran every new/changed
      `-tags=integration` test against real Postgres via Docker
      (`internal/api/handler` TestMe_ReportsTier, `internal/engage/prowelcome`'s four
      Postgres-backed runner tests) — all pass.
- [x] 6.3 Frontend: `svelte-check` (0 errors), full `vitest run` (163 files / 1888 tests
      passing, including the new `HeaderMenu.test.ts`), `eslint` on the changed files —
      all clean.
- [x] 6.4 Substituted for a manual smoke test (no AWS SES credentials available in this
      environment, and a real send would not be appropriate here): the
      `store_integration_test.go` Postgres-backed tests exercise the exact same path a
      manual run would — insert a paying user, run the worker, confirm exactly one send
      and the column stamped, run again, confirm no second send, and confirm a failed
      send leaves the row retryable. Also ran `node scripts/check-migrations.mjs` (squawk,
      0 issues on the new migration), `shellcheck deploy/bin/release.sh` (0 issues) and
      `node scripts/check-doc-links.mjs` (357 links, all resolve) since this change edits
      those surfaces too.
