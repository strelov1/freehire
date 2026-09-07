## 1. Schema and store

- [x] 1.1 Add migration `0144_discord_links.sql`: `discord_links` with `user_id` PK
      referencing `users(id)` ON DELETE CASCADE, `discord_user_id text NOT NULL UNIQUE`,
      `linked_at timestamptz NOT NULL DEFAULT now()`, `role_granted_at timestamptz`.
      Verify with `pnpm check:sql`.
- [x] 1.2 Add queries in `internal/platform/db/queries/discord_links.sql`: link (insert),
      unlink (delete by user), get by user, list a bounded page for reconciliation joined
      to `users.pro_until`/`ultra_until`, set and clear `role_granted_at`. Run `make sqlc`.
- [x] 1.3 Integration-tagged tests for the queries, including that a second binding of the
      same `discord_user_id` is rejected by the constraint.

## 2. Configuration

- [x] 2.1 Add `DiscordClientID`, `DiscordClientSecret`, `DiscordBotToken`, `DiscordGuildID`,
      `DiscordPaidRoleID` to `internal/platform/config`, with a predicate reporting whether
      the feature is fully configured — modelled on the Telegram block above it.
- [x] 2.2 (Folded into 5.4.) There is no global public-config flag to add: the Telegram
      pattern reports `{enabled, linked}` from the feature's own status endpoint, and Discord
      follows it. `Settings.DiscordPaidAccessConfigured` is the single decider, unit-tested
      for the all-five and any-one-missing cases.

## 3. `internal/engage/discordlink` — domain and client

- [x] 3.1 Create the package and register it in the `engage` list in
      `internal/platform/arch/layering/blocks.go`; confirm the layering test passes.
- [x] 3.2 `discordlink.go`: the link type and the tier→role decision (any paying tier
      warrants the role; free does not), with unit tests.
- [x] 3.3 `client.go`: token exchange, `GET /users/@me`, `PUT /guilds/{g}/members/{u}`,
      `PUT`/`DELETE /guilds/{g}/members/{u}/roles/{r}` over `internal/platform/safehttp`.
      Tests against `httptest` covering `204`, `404 Unknown Member` (an absence, not an
      error), `50013 Missing Permissions` (surfaced naming the role hierarchy), and `429`
      with `Retry-After`.

## 4. `internal/engage/discordlink` — service

- [x] 4.1 `repository.go`: the store interface and its sqlc-backed implementation.
- [x] 4.2 Link: exchange the code, read the Discord user id, store the binding, join the
      guild, grant the role when the tier is paying. A `discord_user_id` already bound to
      another account is a typed conflict error. Unit tests with a fake client and store.
- [x] 4.3 Unlink: revoke the role, delete the binding; the binding is deleted even when the
      revoke call fails. Unit test both paths.
- [x] 4.4 Sync-one: grant when paying and not recorded as granted, revoke when not paying
      and recorded as granted, do nothing otherwise; clear the grant record on
      `404 Unknown Member`. Unit tests for each branch, including that a repeat run over an
      unchanged account writes nothing.

## 5. HTTP routes

- [ ] 5.1 `GET /api/v1/me/discord/connect` behind `RequireAuth` (cookie only): redirect to
      Discord with `identify guilds.join` and the signed state cookie. 404 when the feature
      is unconfigured.
- [ ] 5.2 `GET /api/v1/me/discord/callback`: verify state, complete the link, redirect back
      to `/my/integrations`. 409 on a conflicting binding; refuse a missing or mismatched
      state without writing.
- [ ] 5.3 `DELETE /api/v1/me/discord` behind `RequireAuth`: unlink.
- [ ] 5.4 Report link status in whatever `/my/integrations` already reads, so the card can
      render connected/disconnected.
- [ ] 5.5 Integration-tagged handler tests: happy path, API key refused with 401, conflicting
      binding, bad state, and all three routes 404 when unconfigured.

## 6. Worker

- [ ] 6.1 `cmd/discord-sync`: `worker.Main`, no-op without credentials, bounded by
      `DISCORD_SYNC_MAX_PER_RUN` (default 500) via `worker.EnvInt64` so an unreadable value
      fails the run naming the value.
- [ ] 6.2 Doc comment in the style of `cmd/billing-sync`: what it reconciles, why it is its
      own binary rather than a `billing-sync` pass, and what it needs.
- [ ] 6.3 Test the run loop over a fake service: the bound is honoured and a stopped run
      leaves the rest for the next one.

## 7. Web

- [ ] 7.1 Add the Discord card to `web/src/lib/components/IntegrationsView.svelte` following
      the Telegram card: connect, connected badge, disconnect, error text. Hidden when the
      feature is off.
- [ ] 7.2 Add the client calls to the integrations store beside the Telegram ones.
- [ ] 7.3 Run `pnpm --dir web lint` and the web tests.

## 8. Operations and documentation

- [ ] 8.1 Add the systemd unit and hourly timer for `discord-sync` under `deploy/`, and note
      in `deploy/AGENTS.md` that they must be copied to the host by hand.
- [ ] 8.2 Document the manual Discord setup in `deploy/AGENTS.md`: create the application and
      bot, invite with `Manage Roles`, create `Paid`, **drag the bot's role above `Paid`**,
      set the closed channels' permissions.
- [ ] 8.3 Add `internal/engage/discordlink/AGENTS.md` and its row in the module table in
      `CLAUDE.md`; add the worker's entry to the worker-gotchas list. Verify with
      `pnpm check:links`.

## 9. Verification

- [ ] 9.1 `gofmt -l .` clean, `go vet ./...`, `go test ./...`,
      `go vet -tags=integration ./...`, `golangci-lint run`.
- [ ] 9.2 `go test -tags=integration ./...` for the packages this change touches.
