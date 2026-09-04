## Context

freehire's board-contribution pipeline (`internal/contribution`, `internal/handler/intake.go`)
already has four surfaces feeding one sequence: `intakeService.Resolve` — catalog lookup, then
`contribution.Service.Inspect`, then `linkimport.Importer.Import`, then
`contribution.RecordIntake` (which awards AI credits on the first contribution of a board).
`internal/contribution/AGENTS.md` documents this as load-bearing: "there is no 'contribute a
board' endpoint any more... a second door onto the same flow is a second behaviour waiting to
drift." The `link-contributions` spec independently requires that only an authenticated caller
may contribute at all — an unauthenticated request records nothing and is rejected.

The closest existing precedent is the Telegram bot (`internal/handler/telegram.go` +
`internal/telegramnotify`): a user links their chat once via a deep-link token
(`t.me/<bot>?start=<token>`), and thereafter any link they paste into the bot DM is resolved
through the shared intake sequence with `surface = "telegram"`. An unlinked chat gets no import —
just a prompt to link first. This design reproduces that exact contract for Discord, changing
only the transport (Discord has no DM deep-link equivalent) and the delivery mechanism
(HTTP interaction webhook instead of a long-polled/webhook bot API).

`cmd/server` is currently the only long-running process in the codebase; every other binary under
`cmd/` is a run-once cron worker. This design deliberately avoids introducing the first persistent
outbound connection (a Discord Gateway/WebSocket client) into that process.

## Goals / Non-Goals

**Goals:**
- Let a Discord user in the operator's existing server contribute a job/board link with the same
  outcome — import, dedup, review-queue fallback, AI-credit reward on first-of-board — as every
  other surface, once they've linked their Discord identity to their freehire account.
- Keep the interaction webhook stateless per-request, consistent with the rest of `cmd/server`'s
  Fiber handlers — no new persistent connection, no new background goroutine lifecycle beyond the
  bounded-timeout pattern `telegramContribTimeout` already establishes.
- Reuse `intakeService.Resolve` unchanged. No new entry point into the contribution pipeline that
  bypasses recording or authentication.

**Non-Goals:**
- Anonymous/unauthenticated contribution. Decided explicitly during design: `/contribute` from an
  unlinked Discord identity imports nothing, mirroring Telegram's current behavior and leaving
  `link-contributions`' "authenticated only" requirement untouched. (An earlier draft of this
  design considered a `ResolveAnonymous` path that imported without recording a contribution row;
  rejected because it still imported into the catalog under no owner, which violates the spirit of
  that requirement even where it doesn't violate its literal HTTP-endpoint wording.)
- Discord OAuth2 (`identify` scope) account linking. A token-exchanged `/link <token>` command was
  chosen instead, reusing the existing JWT-mint pattern (`telegramnotify.LinkTokens`-shaped) rather
  than standing up a new OAuth client, redirect, and callback.
- Free-form message scanning (posting a bare link without a slash command). Requires a Discord
  Gateway connection and the privileged `MESSAGE_CONTENT` intent; rejected in favor of the
  webhook-only `/contribute` command specifically to avoid the first persistent WebSocket client
  in this codebase.
- Discord notifications/digests (the `internal/telegramnotify` equivalent for job-alert delivery).
  Out of scope — this change is contribution-intake only.
- Multi-guild support or a publicly listed bot. Single-server deployment against the operator's
  existing Discord server; commands are guild-scoped, not global.

## Decisions

### Transport: HTTP interaction webhook, not a Gateway bot
Discord offers two ways to receive slash-command invocations: a persistent Gateway (WebSocket)
connection, or an HTTP endpoint Discord POSTs to per interaction ("Interactions Endpoint URL").
The webhook model requires no third-party client library (`crypto/ed25519` + `net/http` suffice),
no session/heartbeat/reconnect logic, and matches the existing `POST /telegram/webhook` shape
almost exactly — an unauthenticated POST guarded by a signature check, mounted alongside it in
`internal/handler`. Chosen over the Gateway model because free-form message scanning (the only
capability that requires it) is explicitly out of scope, and because `cmd/server` has no existing
pattern for a persistent outbound connection to maintain, retry, and shut down cleanly.

### Authentication: Ed25519 signature, not a shared secret
Discord signs every interaction payload with Ed25519 over `timestamp + body`
(`X-Signature-Ed25519` / `X-Signature-Timestamp` headers), verified against the application's
public key. This differs from Telegram's shared-secret header compare
(`X-Telegram-Bot-Api-Secret-Token`, constant-time string equality) but serves the identical role —
the only unauthenticated POST in the API is authenticated by what the platform itself signs. No
new dependency: `crypto/ed25519.Verify` is stdlib.

### Deferred response + follow-up PATCH, not a synchronous reply
Discord requires an ACK within 3 seconds. Because `intakeService.Resolve` may fetch and parse a
whole ATS board (the same reason `telegramContribTimeout` is a generous 60s), `/contribute` must
respond immediately with a deferred-response interaction (type 5), then complete the intake in a
background goroutine bounded by a timeout mirroring `telegramContribTimeout`, and finally `PATCH`
the deferred message via `https://discord.com/api/v10/webhooks/{application_id}/{interaction_token}/messages/@original`.
This endpoint is authorized by the interaction token itself, not the bot token — the background
goroutine only needs to carry the token forward, the same way Telegram's goroutine carries
`chatID` forward to `sendTelegram`.

### Linking: token-exchanged `/link <token>` command, not Discord OAuth2
Telegram's deep link (`t.me/<bot>?start=<token>`) works because opening it both launches Telegram
*and* pre-fills the `/start` command. Discord has no equivalent single-tap flow into a slash
command with an argument attached. Two ways to close that gap: (a) mint a token on the site and
have the user manually type `/link <token>` in the channel, or (b) implement Discord's OAuth2
authorization-code flow (`identify` scope) as a "Link Discord" button. (a) reuses the existing
JWT-mint/short-TTL/no-server-side-store mechanism (`telegramnotify.LinkTokens`) verbatim in shape;
(b) would introduce an entirely new OAuth client, redirect, and callback for a capability
(discovering the caller's Discord user id) the interaction payload already hands us for free once
the bot exists. Chosen (a) — less new surface area, and the manual copy/paste step is a one-time
cost, not a per-contribution one.

### No anonymous contribution path
See "Non-Goals" above. `link-contributions`' "authenticated only" requirement stays intentionally
unmodified; this change adds a new requirement to it ("Contribute a board from Discord") that is
structurally identical to the Telegram one, rather than carving out an exception.

### Data model: `discord_links` as a structural mirror of `telegram_links`
```sql
CREATE TABLE discord_links (
    user_id    bigint NOT NULL,
    discord_id bigint NOT NULL,
    linked_at  timestamp with time zone DEFAULT now() NOT NULL
);
-- PK(user_id), FK user_id -> users(id) ON DELETE CASCADE, no uniqueness on discord_id
```
Matches `telegram_links`' actual constraints (verified against `migrations/0001_init.sql`) exactly:
primary key on `user_id` only (so linking again overwrites in place, "most recent wins"), no
separate uniqueness constraint on the external identity column. `GetUserIDByDiscordID` mirrors
`GetUserIDByTelegramChat`'s `ORDER BY linked_at DESC LIMIT 1` shape for symmetry, though with the
PK on `user_id` a given `discord_id` can only ever be attached to one row's worth of the *same*
user at a time — the ordering matters only in the same edge case it matters for Telegram (a
`discord_id` that was re-linked to a different user after an unlink).

### Command registration: a oneshot binary, not runtime auto-registration
Slash-command definitions rarely change once shipped. Registering them at every `cmd/server`
startup would mean every deploy makes a Discord API call for something that is almost always a
no-op — and a failure there has no good failure mode inside a request-serving process's boot path.
A small `cmd/discord-register-commands` binary (needs `DISCORD_BOT_TOKEN`,
`DISCORD_APPLICATION_ID`, `DISCORD_GUILD_ID`; no `DATABASE_URL`) run by hand when commands change
matches the existing `cmd/*` oneshot convention and keeps `cmd/server`'s boot path free of a
call to an external API that has nothing to do with serving a request.

## Risks / Trade-offs

- **[Risk]** A stale/rotated `DISCORD_PUBLIC_KEY` silently breaks signature verification (every
  interaction gets rejected as unauthenticated) with no visible symptom besides Discord showing
  "the application did not respond" in the channel. **Mitigation**: log the rejection reason (not
  just a 401) so it's visible in `journalctl`, matching how Telegram's webhook-secret mismatch is
  distinguishable in logs from an inert feature.
- **[Risk]** The deferred-response follow-up PATCH can itself fail (network blip, expired
  interaction token — Discord tokens for follow-ups are valid 15 minutes) after the intake work
  already completed (and possibly already recorded a reward). **Mitigation**: same posture as
  Telegram's `sendTelegram` — log and swallow; the contribution/reward already succeeded or failed
  independently of whether the confirmation message reaches the user.
- **[Trade-off]** Requiring linking before any contribution (vs. the originally discussed anonymous
  mode) means a drive-by visitor gets no value from typing `/contribute` — they must first visit
  the site, mint a token, and run `/link`. Accepted: it's the same friction Telegram already has,
  and preserves the existing authenticated-only invariant without a spec exception.
- **[Trade-off]** Guild-scoped command registration means the bot only works in the one configured
  server. Acceptable per the stated single-server scope; expanding later means registering the same
  commands per additional `DISCORD_GUILD_ID`, not a redesign.

## Migration Plan

1. Add migration for `discord_links` (number assigned at implementation time — check the actual
   latest file in `migrations/` and cross-check the prod migration ledger convention noted in this
   repo's `AGENTS.md`; numbers have collided here before).
2. Ship `internal/discordbot`, `internal/handler/discord.go`, the `SurfaceDiscord` constant, and
   the new config fields — all inert until `DISCORD_BOT_TOKEN` / `DISCORD_APPLICATION_ID` /
   `DISCORD_PUBLIC_KEY` / `DISCORD_GUILD_ID` are set, exactly like the Telegram bot's all-or-nothing
   gate. Safe to deploy ahead of configuring the bot.
3. Register the Discord application, invite the bot to the operator's server, set the Interactions
   Endpoint URL to `POST /api/v1/discord/interactions`, and set the four env vars.
4. Run `cmd/discord-register-commands` once to publish `/link` and `/contribute` to the guild.
5. Rollback: unset the env vars (feature goes inert immediately, same as pulling
   `TELEGRAM_BOT_TOKEN`) — no data migration to reverse; the `discord_links` table is harmless if
   left in place.

## Open Questions

- Exact migration number — deferred to implementation (see Migration Plan step 1).
- Whether `/contribute`'s reply text should live in a shared helper with `intakeReply` (currently
  private to `internal/handler/telegram.go`) or be duplicated with the same switch shape. Leaning
  toward extracting a shared, surface-agnostic helper since the vocabulary
  (found/tracked/imported/review/queued) is already surface-agnostic — to be confirmed during
  implementation rather than blocking this design.
