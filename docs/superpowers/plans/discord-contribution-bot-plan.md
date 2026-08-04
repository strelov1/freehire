# Implementation plan: discord-contribution-bot

Source of truth for requirements: `openspec/changes/discord-contribution-bot/{proposal.md,design.md,tasks.md,specs/**/*.md}`
in this worktree. Every task below assumes those files have been read.

## Global Constraints

These bind every task. Copy this section verbatim into each reviewer's context.

- **No anonymous contribution path.** A `/contribute` invocation from a Discord identity not
  linked to any freehire user MUST import nothing and MUST NOT call
  `intakeService.Resolve` or any part of the intake pipeline — only reply prompting the user to
  link. This preserves `link-contributions`' existing "authenticated only" requirement unchanged.
  Do not add any "resolve without recording" variant to `intakeService`.
- **`intakeService.Resolve` (internal/handler/intake.go) is called unchanged** — same signature,
  same behavior, for the new Discord surface exactly as it already is for
  web/telegram/extension/cli. Do not modify `intake.go`.
- **Feature is fully inert unless ALL of `DiscordBotToken`, `DiscordApplicationID`,
  `DiscordPublicKey`, `DiscordGuildID` are set** — mirrors the Telegram bot's all-or-nothing
  config gate (`internal/handler/telegram.go`'s `telegramEnabled`/`webhookSecured` pattern).
  A deployment must never reach a state where the bot is live but the interactions webhook is
  unauthenticated.
- **`discord_links` mirrors `telegram_links` exactly**: `user_id bigint NOT NULL` (PK, FK ->
  `users(id)` ON DELETE CASCADE), `discord_id bigint NOT NULL`, `linked_at timestamptz DEFAULT
  now() NOT NULL`. No separate uniqueness constraint on `discord_id`. Query semantics mirror
  `GetUserIDByTelegramChat`'s `ORDER BY linked_at DESC LIMIT 1` "most recent wins" shape.
- **Migration numbering**: run `ls migrations/ | tail -3` at the time you write the migration and
  use the next sequential number — do not assume a number from this plan text, the repo's
  migration numbers have collided before (see `AGENTS.md`).
- **`link_contributions_surface_check`** (added in `migrations/0050_link_contributions_surface.sql`)
  is a Postgres CHECK constraint enumerating `('web','telegram','extension','cli','unknown')`.
  Adding the `discord` surface requires widening it (`DROP CONSTRAINT` + re-`ADD CONSTRAINT`
  with `'discord'` in the array) in the same migration file as `discord_links`, or a second one
  applied together. `link_contributions` is a small table — a plain `ADD CONSTRAINT` (not
  `NOT VALID`) is fine; this is not the large-table case `AGENTS.md` warns about.
- **`go build ./...` and `go vet ./...` must stay clean after every task.**
- **Changing any handler constructor signature requires `go vet -tags=integration ./...`**
  before that task is considered done — plain `go test ./...` does not compile build-tagged
  files (152 of them across 13 packages, `internal/handler` alone has 65). Docker is available
  in this environment, so the actual tagged integration suite
  (`go test -tags=integration ./internal/handler/...`) can and should be run wherever a task
  touches `internal/handler`, not just vet-checked.
- **Mirror existing patterns, don't invent new ones.** `internal/handler/telegram.go` +
  `internal/telegramnotify/client.go` are the direct precedent for everything in this plan.
  When in doubt about a naming/shape/error-handling choice, match what those two files already
  do.
- **Terse commit messages, no comments explaining WHAT** (only WHY, and only when non-obvious) —
  per this repo's `AGENTS.md` code style.

## Task 1: Data model & config

Add the `discord_links` table and the surface-check widening in one migration, generate sqlc
queries for it, and add the new Discord config fields.

**Migration** (new file `migrations/00NN_discord_links.sql`, NN = next sequential number per
Global Constraints):
```sql
CREATE TABLE public.discord_links (
    user_id    bigint NOT NULL,
    discord_id bigint NOT NULL,
    linked_at  timestamp with time zone DEFAULT now() NOT NULL
);

ALTER TABLE ONLY public.discord_links
    ADD CONSTRAINT discord_links_pkey PRIMARY KEY (user_id);

ALTER TABLE ONLY public.discord_links
    ADD CONSTRAINT discord_links_user_id_fkey FOREIGN KEY (user_id) REFERENCES public.users(id) ON DELETE CASCADE;

ALTER TABLE public.link_contributions
    DROP CONSTRAINT link_contributions_surface_check;

ALTER TABLE public.link_contributions
    ADD CONSTRAINT link_contributions_surface_check
    CHECK ((surface = ANY (ARRAY['web'::text, 'telegram'::text, 'extension'::text, 'cli'::text, 'discord'::text, 'unknown'::text])));
```
Read `migrations/0001_init.sql` (search `telegram_links`) to confirm the exact DDL style/ordering
convention (table, then PK constraint, then FK constraint as separate `ALTER TABLE` statements)
before writing this file, and follow it.

**sqlc queries** — new file `internal/db/queries/discord_links.sql`, following
`internal/db/queries/telegram_links.sql`'s shape exactly (read it first):
- `UpsertDiscordLink` (insert ... on conflict (user_id) do update, mirrors `UpsertTelegramLink`)
- `GetDiscordLink` (`:one`, by `user_id`)
- `GetUserIDByDiscordID` (`:one`, `SELECT user_id FROM discord_links WHERE discord_id = $1 ORDER BY linked_at DESC LIMIT 1`)
- `DeleteDiscordLink` (by `user_id`)

Run `make sqlc` after adding the query file (regenerates `internal/db`).

**Config** — in `internal/config/config.go`, add to the `Settings` struct and `Load()`:
```go
DiscordBotToken       string
DiscordApplicationID  string
DiscordPublicKey      string
DiscordGuildID        string
```
read from `DISCORD_BOT_TOKEN`, `DISCORD_APPLICATION_ID`, `DISCORD_PUBLIC_KEY`,
`DISCORD_GUILD_ID` via plain `os.Getenv` — no defaults, matching the `TelegramBotToken` field
group (read that block in `config.go` first for placement/comment style).

**Done when**: `go build ./...` clean, `make sqlc` ran without diffing anything unexpected,
migration file present and follows the `telegram_links` DDL shape. No dedicated test for this
task — correctness is verified by the integration test in Task 5 exercising the real schema.

## Task 2: internal/discordbot client package

New package `internal/discordbot`, the Discord-side counterpart to `internal/telegramnotify`.
Read `internal/telegramnotify/client.go` first for the file's shape/style (small client struct,
`NewClient`/`NewClientWithBase` pair — the `WithBase` variant exists specifically so tests can
point the client at an `httptest.Server` instead of the real API; do the same here) and
`internal/telegramnotify/linktokens.go` (or wherever `LinkTokens` lives — grep for it) for the
token issue/parse/TTL shape.

Build:
- `VerifySignature(publicKeyHex string, timestamp, body []byte, signatureHex string) bool` —
  decode both hex strings, Ed25519-verify `append(timestamp, body...)` against the decoded
  public key using stdlib `crypto/ed25519`. Return `false` on any decode failure, never panic.
- `Client` type with `NewClient(botToken string)` / `NewClientWithBase(botToken, baseURL string)`,
  and:
  - `EditOriginalResponse(ctx context.Context, applicationID, interactionToken, content string) error`
    — `PATCH {base}/webhooks/{applicationID}/{interactionToken}/messages/@original` with JSON
    body `{"content": content}`. No bot-token auth header needed (the interaction token
    authorizes this call) — do not send an `Authorization` header for it.
  - `RegisterCommands(ctx context.Context, applicationID, guildID string, commands []Command) error`
    — `PUT {base}/applications/{applicationID}/guilds/{guildID}/commands` with `Authorization: Bot
    {botToken}` and the JSON-encoded command array.
  - Default base URL `https://discord.com/api/v10`.
- Interaction payload types (JSON-tagged structs) for: the inbound interaction request (type,
  application command name + string options, member/user with Discord user id, token field),
  PING (type 1) and its PONG reply (type 1), APPLICATION_COMMAND (type 2), the deferred response
  (type 5, `DEFERRED_CHANNEL_MESSAGE_WITH_SOURCE`), and an immediate ephemeral response (type 4
  with `data.flags = 64`). Keep these minimal — only the fields this bot actually reads/writes,
  not a full Discord API SDK.
- `Command` type — name, description, and an options list (string option, required) sufficient
  to describe `/link token:<string>` and `/contribute url:<string>`.
- `DiscordLinkTokens` — same shape as `telegramnotify.LinkTokens` (signed JWT, short TTL, no
  server-side store, `Issue(userID int64) (string, error)` / `Parse(token string) (int64, error)`
  or whatever that type's exact method names are — match them for consistency). Reuse the same
  JWT secret parameter and a TTL constant analogous to `telegramLinkTTL` (10 minutes) — name it
  appropriately for this package.

**Tests** (`internal/discordbot/*_test.go`, table-driven, no network/DB needed):
- `VerifySignature`: valid signature passes; tampered body fails; wrong timestamp fails; garbage
  hex fails without panicking.
- `DiscordLinkTokens`: issue then parse round-trips the user id; an expired token fails to parse;
  a token signed with a different secret fails to parse.
- `EditOriginalResponse` / `RegisterCommands`: use `httptest.NewServer` (via `NewClientWithBase`)
  to assert the request method, path, and body shape; a non-2xx response returns an error.

**Done when**: `go build ./...` and `go test ./internal/discordbot/...` are clean.

## Task 3: Discord handler — linking surface

New file `internal/handler/discord.go`. Read `internal/handler/telegram.go` in full first — this
task mirrors its `telegramHandlers` struct, `newTelegramHandlers`, `telegramEnabled`,
`webhookSecured`, `LinkTelegram`, `TelegramLinkStatus`, `UnlinkTelegram`, and the top half of
`TelegramWebhook` (everything up to where it dispatches into contribution handling) as closely
as the Discord API's shape allows. Also add the trivial `SurfaceDiscord` constant now (see below)
since this task is the first consumer of it.

Build:
- `internal/contribution/contribution.go`: add `SurfaceDiscord = "discord"` alongside the
  existing `Surface*` constants, and add it to the `NormalizeSurface` switch's accepted list.
- `discordHandlers` struct: `queries *db.Queries`, a `*discordbot.Client`, a
  `*discordbot.DiscordLinkTokens`, the four config strings needed at request time
  (`discordPublicKey`, `discordApplicationID`, etc. — only what's actually read per-request;
  don't store what you don't use), `frontendOrigin string`, `intake *intakeService`.
- `newDiscordHandlers(queries, jwtSecret, botToken, applicationID, publicKey, guildID,
  frontendOrigin string, intake *intakeService) *discordHandlers` — nil-out/leave-inert fields
  unless all four Discord config values are non-empty (see Global Constraints — this is the
  all-or-nothing gate, no partial-enable state).
- `discordEnabled() bool` — mirrors `telegramEnabled`.
- `register(api fiber.Router, mw middleware)`:
  - `api.Post("/me/discord/link", mw.cookie, h.LinkDiscord)`
  - `api.Get("/me/discord", mw.cookie, h.DiscordLinkStatus)`
  - `api.Delete("/me/discord", mw.cookie, h.UnlinkDiscord)`
  - `api.Post("/discord/interactions", h.DiscordInteraction)` — the only unauthenticated POST,
    guarded by Ed25519 signature verification inside the handler (verify BEFORE parsing the
    interaction body as JSON — reject on bad signature without looking at the payload shape).
- `LinkDiscord` — cookie-only, mints a token via `DiscordLinkTokens.Issue`, returns
  `{"data": {"token": "<token>", "instructions": "..."}}` (NOT a URL — Discord has no deep-link
  equivalent; the SPA shows the token and tells the user to run `/link <token>` in the server).
- `DiscordLinkStatus` — mirrors `TelegramLinkStatus`: `{"enabled": ..., "linked": ...,
  "discord_id": ...}` via `GetDiscordLink`.
- `UnlinkDiscord` — mirrors `UnlinkTelegram`, calls `DeleteDiscordLink`, 204, idempotent.
- `DiscordInteraction` — verify the Ed25519 signature over the raw request body (read the raw
  body via Fiber before any JSON parsing — check how Fiber exposes the raw body, e.g.
  `c.Body()`) against `X-Signature-Ed25519` / `X-Signature-Timestamp` headers and the configured
  public key; on failure, `403`. On success, parse the interaction. If `type == 1` (PING), reply
  `{"type": 1}`. If `type == 2` (APPLICATION_COMMAND), dispatch by command name: `"link"` ->
  handle inline (below); `"contribute"` -> Task 4. Any other/unknown command name: reply with a
  generic error (type 4, ephemeral) rather than panicking on an unrecognized shape.
- `/link` command handling (inline in `DiscordInteraction` or a small helper method): read the
  `token` string option, parse it via `DiscordLinkTokens.Parse`; on success, resolve the
  invoking Discord user id (`member.user.id` in a guild, `user.id` in a DM — check both, guild
  is the expected case here) and `UpsertDiscordLink`; reply type 4, ephemeral (`flags: 64`),
  with a confirmation or failure message. This is a synchronous, non-deferred reply (the DB
  upsert is fast — no need for the deferred-response dance Task 4 uses for `/contribute`).

**Tests** (`internal/handler/discord_test.go`, no DB — use a fake/nil queries where the linking
endpoints allow it, or scope tests to what's reachable without one; read
`internal/handler/telegram_test.go` for the existing signature-guard test style and mirror it):
- Missing/invalid Ed25519 signature -> 403, no processing.
- Valid PING -> PONG (`{"type":1}`).
- `/link` with a valid token upserts the link and replies ephemeral-success (this may need a
  lightweight DB — if `internal/handler`'s existing test helpers include an in-memory or
  fake-queries pattern for handler unit tests, use it; otherwise defer full-flow verification to
  Task 5's integration test and keep this test to signature/PING/command-routing only).
- `/link` with an expired/garbage token -> failure reply, no DB write attempted.
- Feature-disabled (partial config) -> linking endpoints report `enabled: false`, interactions
  endpoint responds `404`.

**Done when**: `go build ./...`, `go vet ./...`, and `go test ./internal/handler/... ./internal/contribution/...`
are clean. Do NOT wire `discordHandlers.register` into `handler.go` yet — Task 4 does that once
`/contribute` exists too, so the router only gains the new routes once the handler is complete.

## Task 4: Discord handler — contribution surface

Extends `internal/handler/discord.go` from Task 3. Read the rest of
`internal/handler/telegram.go` (`handleTelegramContribution`, `processTelegramContribution`,
`intakeReply`, `jobURL`, `companyURL`, and the `telegramContribTimeout` constant) — this task is
the Discord-shaped mirror of all of it.

Build:
- Extract `intakeReply`'s `switch out.Status { ... }` body (and the `jobURL`/`companyURL`
  helpers it uses) out of `internal/handler/telegram.go` into a shared, surface-agnostic
  function — e.g. `func renderIntakeOutcome(out intakeOutcome, frontendOrigin string) string` in
  `internal/handler/intake.go` or a new small `internal/handler/intake_reply.go` — so both
  `telegramHandlers` and `discordHandlers` call the same function instead of duplicating the
  switch. Update `telegram.go`'s call site accordingly; its existing tests must keep passing
  unchanged (the extraction must be behavior-preserving).
- `/contribute` command handling: on receiving the interaction, reply immediately with type 5
  (deferred, no `data` payload needed). Then, in a goroutine bounded by a timeout constant
  mirroring `telegramContribTimeout` (60s, same rationale — intake may fetch a whole ATS board):
  1. Look up the invoking Discord user id via `GetUserIDByDiscordID`.
  2. Not found (`pgx.ErrNoRows`) -> `EditOriginalResponse` with a message prompting the user to
     link first (mirror `processTelegramContribution`'s unlinked-chat message). Do NOT call
     `intakeService.Resolve` or any intake/import code in this branch — see Global Constraints.
  3. Found -> read the `url` string option, call
     `s.intake.Resolve(ctx, userID, url, contribution.SurfaceDiscord)`, then
     `EditOriginalResponse` with `renderIntakeOutcome(out, h.frontendOrigin)` (the extracted
     helper from above).
  4. A `Resolve` error (not a normal outcome — a storage failure) logs and edits with a generic
     "something went wrong" message, mirroring `processTelegramContribution`'s error branch.
- Wire `discordHandlers.register(api, mw)` into `internal/handler/handler.go` alongside
  `telegramH.register(api, mw)` (grep `handler.go` for where `telegramH` is constructed and
  registered, and add the equivalent `discordH := newDiscordHandlers(...)` +
  `discordH.register(api, mw)` calls with the new config fields from Task 1's `Settings`).

**Tests** (extend `internal/handler/discord_test.go`):
- `/contribute` command-option parsing (the `url` string option is read correctly).
- Command dispatch: unknown command name doesn't panic and replies with a generic error.
- `renderIntakeOutcome` (or whatever the extracted helper is named): table-driven test over every
  `intakeOutcome.Status` value or a targeted subset, confirming the extraction didn't change
  Telegram's existing wording (compare against what `intakeReply` used to produce, or add a test
  that both `telegramHandlers` and `discordHandlers` render the same outcome to the same text
  where the surface doesn't matter). This is the "no regression from the extraction" check —
  `internal/handler`'s EXISTING telegram tests must still pass; if the extraction moved logic
  telegram's tests exercised, verify they still cover it post-move.

Full DB-backed coverage (linked user's `/contribute` actually rewards, etc.) belongs in Task 5's
integration test, not here.

**Done when**: `go build ./...`, `go vet ./...`, `go test ./internal/handler/... ./internal/contribution/...`
clean, and `internal/handler/telegram_test.go` + any other existing telegram-related unit tests
still pass unmodified (confirms the `intakeReply` extraction didn't regress Telegram).

## Task 5: Integration tests

New file `internal/handler/discord_integration_test.go` (`//go:build integration`), mirroring
`internal/handler/telegram_contribution_integration_test.go` — read it in full first, including
`startPostgres(t)` and how it stubs the outbound Telegram API via `httptest.NewServer` +
`NewClientWithBase`. Do the same for Discord (`discordbot.NewClientWithBase`), and additionally
construct Discord's Ed25519-signed request headers using a real generated keypair (generate one
in the test with `ed25519.GenerateKey`, configure the handler with the public key, sign requests
with the private key) since, unlike Telegram's static shared-secret header, Discord interactions
must carry a valid signature to reach the handler at all.

Cover, per `openspec/changes/discord-contribution-bot/specs/**/*.md`'s scenarios:
- Linked user's `/contribute` on a readable-vacancy link: full flow, deferred ack then a
  follow-up `EditOriginalResponse` call with a link to the posting (assert on the stub server
  receiving the PATCH).
- Linked user's `/contribute` on a novel board: contribution recorded, AI credits awarded — query
  the DB directly to confirm (mirrors how the Telegram integration test asserts reward).
- Linked user's `/contribute` on a board already contributed: no double reward.
- Unlinked identity's `/contribute`: no `link_contributions` row created, no credits, follow-up
  message prompts linking. Assert directly against the DB that nothing was written — this is the
  test that actually proves the "no anonymous path" constraint holds.
- `/link` with a valid token: `discord_links` row created/updated.
- `/link` with an expired or garbage token: no row written, failure reply.
- Feature-disabled (one of the four config values missing): `/me/discord` reports disabled,
  `/api/v1/discord/interactions` responds 404.

**Verification for this task** (run both, from the module root):
```
go vet -tags=integration ./...
go test -tags=integration ./internal/handler/...
```
Docker is available in this environment — actually run the tagged suite, don't just vet-check
it. Paste the pass/fail summary into the task report.

**Done when**: both commands above are clean.

## Task 6: cmd/discord-register-commands

New oneshot binary `cmd/discord-register-commands/main.go`. Read one or two existing simple
`cmd/*/main.go` binaries first (e.g. `cmd/migrate` or another small oneshot worker — check
`internal/worker` bootstrap conventions per `AGENTS.md`'s "worker.Bootstrap is mandatory" note,
though this binary does NOT need `DATABASE_URL` or the DB-touching parts of that convention —
confirm from the worker package whether `Bootstrap` requires a DB connection unconditionally; if
it does, this binary should NOT use it, since design.md is explicit this needs no
`DATABASE_URL`).

Behavior: read `DISCORD_BOT_TOKEN`, `DISCORD_APPLICATION_ID`, `DISCORD_GUILD_ID` from the
environment (fail fast, exit non-zero, if any is empty — this binary has exactly one job and no
reasonable partial-config mode), construct the `/link` and `/contribute` `discordbot.Command`
definitions (matching Task 3/4's option shapes: `/link token:<string required>`, `/contribute
url:<string required>`), call `discordbot.NewClient(botToken).RegisterCommands(ctx,
applicationID, guildID, commands)`, log success/failure, exit non-zero on error.

**Done when**: `go build ./...` clean and the binary builds standalone
(`go build ./cmd/discord-register-commands`). No integration test needed — it's a thin CLI
wrapper over `discordbot.RegisterCommands`, which Task 2 already covers with an `httptest`-backed
unit test.

## Task 7: Frontend

Read `web/src/lib/api.ts` around the existing `telegramStatus`/`telegramLink`/`telegramUnlink`
functions (grep for them) and `web/src/lib/components/ContributeLandingView.svelte` first.

Build:
- `web/src/lib/api.ts`: add `discordStatus()`, `discordLink()`, `discordUnlink()`, mirroring the
  Telegram trio's shapes exactly but hitting `/api/v1/me/discord`, `/api/v1/me/discord/link`
  (returns `{token, instructions}`, not a URL — see Task 3), `/api/v1/me/discord` DELETE. Add the
  corresponding TypeScript types (mirror `TelegramStatus`) if the codebase's convention is to
  type these responses (check how `TelegramStatus` is defined/imported and follow the same
  pattern).
- A compact "Link Discord for contribution credits" UI affordance. Investigate whether it fits
  on `ContributeLandingView.svelte` (this is a contribution-only feature, unlike Telegram's link
  which doubles as a reminder-delivery channel — do NOT put it in `ReminderSettings.svelte`,
  that component's linking flow is for a different feature). Show: linked/unlinked status; when
  unlinked, a button that calls `discordLink()` and displays the returned token plus the
  `/link <token>` instruction text; when linked, an unlink control. Keep it minimal — no new
  design-system components, reuse whatever this repo's existing settings-toggle/status pattern
  is (look at how `ReminderSettings.svelte` renders its Telegram linked/unlinked state for the
  visual pattern, without touching that file).

**Done when**: `pnpm --dir web run build` (or this repo's equivalent web build/typecheck command
— check `web/AGENTS.md` or `package.json` scripts if `pnpm build` isn't it) succeeds with no new
type errors. No new backend calls needed for this task — the three endpoints already exist from
Task 3.

## Task 8: Docs

Add a short note — in `internal/handler/AGENTS.md` if there's a Telegram-config section to mirror
there, otherwise the top-level `AGENTS.md`'s worker/config listing, whichever already documents
`TELEGRAM_BOT_TOKEN` et al. (grep for it first) — covering the four new env vars
(`DISCORD_BOT_TOKEN`, `DISCORD_APPLICATION_ID`, `DISCORD_PUBLIC_KEY`, `DISCORD_GUILD_ID`) and the
one-time `cmd/discord-register-commands` setup step, in the same terse style as the existing
Telegram bot documentation. Do not invent a new docs section/file if an existing one already
covers Telegram bot setup — extend that one.

**Done when**: the doc change is present and consistent with the file's existing style; no code
changes in this task.
