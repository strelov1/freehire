## 1. Data model & config

- [x] 1.1 Add `discord_links` migration — structural mirror of `telegram_links`
  (`user_id bigint NOT NULL` PK/FK→`users(id)` ON DELETE CASCADE, `discord_id bigint NOT NULL`,
  `linked_at timestamptz DEFAULT now() NOT NULL`, no separate uniqueness on `discord_id`). Check
  the actual latest file under `migrations/` and this repo's migration-ledger convention
  (`AGENTS.md` / `internal/migrate`) before assigning a number — numbers have collided here
  before.
- [x] 1.1a In the same (or a follow-up) migration, widen `link_contributions_surface_check`
  (added in `migrations/0050_link_contributions_surface.sql`) to admit `'discord'`: `ALTER TABLE
  link_contributions DROP CONSTRAINT link_contributions_surface_check`, then re-`ADD CONSTRAINT`
  with `'discord'` added to the allowed array. Without this, `RecordIntake` with
  `SurfaceDiscord` fails a DB CHECK, not a Go-level error. `link_contributions` is small, so a
  plain (not `NOT VALID`) `ADD CONSTRAINT` is fine here — this is not the large-table case
  `AGENTS.md` warns about for `ADD CONSTRAINT`.
- [x] 1.2 Add sqlc queries in `internal/db/queries/discord_links.sql`: `UpsertDiscordLink`,
  `GetDiscordLink` (by `user_id`), `GetUserIDByDiscordID` (by `discord_id`, `ORDER BY linked_at
  DESC LIMIT 1`), `DeleteDiscordLink`. Run `make sqlc`.
- [x] 1.3 Add `DiscordBotToken`, `DiscordApplicationID`, `DiscordPublicKey`, `DiscordGuildID` to
  `internal/config`, following the `TelegramBotToken`-family pattern (plain env vars, no
  defaults).

## 2. internal/discordbot client package

- [x] 2.1 `VerifySignature(publicKeyHex string, timestamp, body []byte, signatureHex string)
  bool` — Ed25519 verification over `timestamp || body` using stdlib `crypto/ed25519`.
- [x] 2.2 `EditOriginalResponse(ctx, applicationID, interactionToken string, content string)
  error` — `PATCH
  https://discord.com/api/v10/webhooks/{application_id}/{interaction_token}/messages/@original`.
- [x] 2.3 `RegisterCommands(ctx, botToken, applicationID, guildID string, commands []Command)
  error` — `PUT
  https://discord.com/api/v10/applications/{application_id}/guilds/{guild_id}/commands`.
- [x] 2.4 Interaction payload types: PING/APPLICATION_COMMAND request shape, command option
  parsing (string options for `/link`'s `token` and `/contribute`'s `url`), response types
  (PONG, deferred-with-source, immediate ephemeral).
- [x] 2.5 `DiscordLinkTokens` — short-TTL signed token issue/parse, same shape as
  `telegramnotify.LinkTokens` (same JWT secret, same TTL constant pattern).

## 3. Contribution surface

- [x] 3.1 Add `SurfaceDiscord` to `internal/contribution/contribution.go` alongside
  `SurfaceWeb`/`SurfaceTelegram`/`SurfaceExtension`/`SurfaceCLI`, and include it in
  `NormalizeSurface`.

## 4. internal/handler/discord.go — webhook, linking, contribution

- [x] 4.1 `discordHandlers` struct + constructor, mirroring `telegramHandlers`: nil/inert unless
  all four Discord config values are set (see spec `discord-account-link` — "Feature is disabled
  when unconfigured").
- [x] 4.2 `POST /api/v1/discord/interactions` — verify signature first (reject before parsing
  the interaction type), respond PONG to PING, dispatch APPLICATION_COMMAND by command name.
- [x] 4.3 `POST /me/discord/link` (cookie-only) — mint a `DiscordLinkTokens` token, return it
  plus the `/link <token>` instruction text (no t.me-style URL — see design.md "Linking"
  decision).
- [x] 4.4 `GET /me/discord` / `DELETE /me/discord` — status/unlink, mirroring
  `TelegramLinkStatus`/`UnlinkTelegram` exactly.
- [x] 4.5 `/link <token>` command handler — parse token, upsert `discord_links`, reply
  immediately (non-deferred) with an ephemeral confirmation or failure message.
- [x] 4.6 `/contribute <url>` command handler — respond deferred (type 5) immediately; in a
  bounded-timeout background goroutine (mirror `telegramContribTimeout`), resolve the invoking
  Discord user id via `GetUserIDByDiscordID`:
  - unlinked → skip intake entirely, `EditOriginalResponse` with a "link your account first"
    message (no import, no record — see spec `link-contributions` "Unlinked identity is prompted
    to link").
  - linked → `intakeService.Resolve(ctx, userID, url, contribution.SurfaceDiscord)`, then
    `EditOriginalResponse` with the outcome.
- [x] 4.7 Extract the outcome-to-reply-text mapping currently private to
  `intakeReply` in `internal/handler/telegram.go` into a shared, surface-agnostic helper (see
  design.md "Open Questions") so both Telegram and Discord render `intakeOutcome` the same way
  without duplicating the switch.
- [x] 4.8 Wire `discordHandlers.register` into the router alongside `telegramH.register`.

## 5. cmd/discord-register-commands

- [x] 5.1 New oneshot binary: reads `DISCORD_BOT_TOKEN`, `DISCORD_APPLICATION_ID`,
  `DISCORD_GUILD_ID` (no `DATABASE_URL`), calls `discordbot.RegisterCommands` with the `/link`
  and `/contribute` definitions, exits non-zero on failure — matching the `cmd/*` oneshot
  convention.

## 6. Frontend

- [x] 6.1 Add `discordStatus` / `discordLink` / `discordUnlink` client functions to
  `web/src/lib/api.ts`, mirroring `telegramStatus`/`telegramLink`/`telegramUnlink`.
- [x] 6.2 Add a compact "Link Discord for contribution credits" affordance — investigate whether
  it belongs on `ContributeLandingView.svelte` (contribution flow) or a settings surface; unlike
  Telegram's link (which doubles as a reminder-delivery channel), this is contribution-only, so
  it does not belong in `ReminderSettings.svelte`. Show the returned token + `/link <token>`
  instruction, and linked/unlinked status.

## 7. Tests

- [x] 7.1 `internal/handler/discord_test.go` — signature verification (valid/invalid/missing),
  PING→PONG, command-option parsing for both commands.
- [x] 7.2 `internal/handler/discord_integration_test.go` (build-tagged) — mirrors
  `telegram_contribution_integration_test.go`: linked user's `/contribute` runs the full intake
  sequence and rewards correctly; unlinked identity imports nothing; `/link` with a valid token
  upserts `discord_links`; expired/invalid token is refused; feature-disabled state (partial
  config) leaves endpoints inert.
- [x] 7.3 Run `go vet -tags=integration ./...` (constructor signatures changed) and the full
  tagged suite for `internal/handler` before pushing, per this repo's `AGENTS.md`.

## 8. Docs

- [x] 8.1 Add a `internal/handler` or top-level note on the new env vars
  (`DISCORD_BOT_TOKEN`/`DISCORD_APPLICATION_ID`/`DISCORD_PUBLIC_KEY`/`DISCORD_GUILD_ID`) and the
  `cmd/discord-register-commands` one-time setup step, following how the Telegram bot's setup is
  documented.
