> **Shipped, then withdrawn on 2026-09-04.** The bot and its account link are removed: the
> search box now takes a pasted job link from every page, signed in or not, which is the
> friction this was buying down. Nothing described below is in the codebase — read it as
> history. The Discord SERVER is unaffected; only the integration is gone.

## Why

freehire already channels URL contributions through one shared pipeline from four surfaces
(website, Telegram, browser extension, CLI). The user runs a public Discord server and wants
the same low-friction contribution path there: post a board/job link in a channel, get it added
to the catalog, optionally earn AI credits for linking their account — without opening a second,
divergent intake path or weakening the existing authenticated-only contribution rule.

## What Changes

- Add a Discord bot delivered as an interaction webhook (not a Gateway/WebSocket client) exposing
  two slash commands: `/link <token>` to link a Discord identity to a freehire account, and
  `/contribute <url>` to submit a link.
- `/contribute` from an unlinked Discord identity imports nothing — the bot replies prompting the
  user to link first, identical to the Telegram bot's behavior today. No anonymous/unauthenticated
  contribution path is introduced; `link-contributions`' existing "authenticated only" requirement
  is preserved unchanged, not relaxed.
- A linked user's `/contribute` runs the exact same `intakeService.Resolve` sequence every other
  surface uses, tagged with a new `discord` surface value.
- New `discord_links` table mirroring `telegram_links` (one freehire user ↔ one Discord user id,
  most-recent-link-wins, no separate uniqueness constraint on the Discord id).
- New config gate (bot token, application id, Ed25519 public key, guild id) — the feature stays
  fully inert until all are set, mirroring the Telegram bot's all-or-nothing config gate.
- New oneshot `cmd/discord-register-commands` binary to (re-)register the two slash commands with
  Discord's API — one-time infra action, not a per-request or cron path, needs no `DATABASE_URL`.

## Capabilities

### New Capabilities
- `discord-account-link`: linking a Discord identity to a freehire account via a one-time token
  exchanged through the bot's `/link` command, plus the interaction webhook's Ed25519 signature
  authentication and the feature's all-or-nothing config gate. The Discord-specific counterpart
  to `telegram-notify`'s linking requirements — notification/digest delivery is explicitly out of
  scope for this capability.

### Modified Capabilities
- `link-contributions`: add a "Contribute a board from Discord" requirement mirroring the existing
  "Contribute a board from Telegram" requirement — same intake sequence, same authenticated-only
  rule, new surface tag and unlinked-identity prompt.

## Impact

- `internal/handler`: new `discord.go` (interaction webhook, `/link` and `/contribute` handlers),
  reusing `intakeService` and the existing outcome-to-reply mapping unchanged.
- `internal/discordbot` (new package): Ed25519 signature verification, deferred-response follow-up
  PATCH, command-registration PUT — mirrors `internal/telegramnotify`'s client shape.
- `internal/contribution`: add a `SurfaceDiscord` constant alongside the existing surface values.
- `internal/config`: `DiscordBotToken`, `DiscordApplicationID`, `DiscordPublicKey`,
  `DiscordGuildID`.
- New migration: `discord_links` table (structural mirror of `telegram_links`).
- `cmd/discord-register-commands`: new oneshot binary.
- No new third-party Go dependency — Ed25519 via stdlib `crypto/ed25519`, HTTP via `net/http`,
  matching the existing `telegramnotify.Client` style.
