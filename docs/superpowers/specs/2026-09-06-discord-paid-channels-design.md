# Closed Discord channels for paying subscribers

Date: 2026-09-06

## Problem

`freehire.me` sells Pro ($5) and Ultra ($19). The community Discord
(`discord.gg/sYnZksswR`) is entirely public, so a subscription buys nothing there.
We want a set of channels only paying subscribers can see, with access that appears
when they pay and disappears when they stop.

## Prior art considered

Two maintained open-source bots do this shape of work:
[StripeCord](https://github.com/Rodaviva29/StripeCord) (33★, MIT) and
[Androz2091/stripe-discord-bot](https://github.com/Androz2091/stripe-discord-bot)
(48★, no licence — unusable). Both are rejected for the same reason: each is a
long-lived Node daemon with its own database that decides who pays by **reading
Stripe directly**.

On this site "paying" is not a Stripe fact. `plan.TierOf` resolves it from
`users.pro_until` and `users.ultra_until`, which are themselves derived from three
source columns each — Stripe, RevenueCat (App Store), and granted promo time. A bot
reading only Stripe would be a second, disagreeing answer to a question this codebase
already answers once, and it would be wrong for every App Store subscriber.

Discord's native **Linked Roles** (role-connection metadata) was also considered and
rejected. It removes the need for a bot with guild permissions, but the role is only
ever granted when the member clicks through the Linked Roles tab themselves — we
could not grant access on payment — and keeping the metadata current requires storing
each user's OAuth **refresh token**, which is more secret material at rest than one
bot token. Discord's own guidance puts subscription gating on bot-assigned roles.

## Design

### The user's path

`/my/integrations` already hosts connect/disconnect cards for Gmail, Calendar and
Telegram (`web/src/lib/components/IntegrationsView.svelte`). Discord becomes a
fourth card with the same shape.

"Connect" redirects to `GET /api/v1/me/discord/connect`, which sends the user to
Discord's consent screen with scopes `identify` and `guilds.join`. The callback
`GET /api/v1/me/discord/callback` exchanges the code, reads the Discord user id,
stores the link, adds the member to the guild, and — if the account is paying —
grants the role. One click, no invite link.

The access token is used within that request and never stored. This mirrors the rule
`internal/identity/auth/oauth` states for sign-in providers, and is the reason Discord
does **not** join that registry: that package is sign-in only ("tokens are used once
to resolve who the user is and are never stored"), while this is a link on an already
authenticated account.

Both routes are behind `RequireAuth` (cookie only — a link is account-shaped, like
key management and password change). The callback carries the same signed state
cookie the OAuth flow uses (`internal/identity/auth/oauth/state.go`).

### What is stored

Migration `0144_discord_links.sql`:

```sql
CREATE TABLE public.discord_links (
    user_id         bigint NOT NULL PRIMARY KEY REFERENCES public.users(id) ON DELETE CASCADE,
    discord_user_id text   NOT NULL UNIQUE,
    linked_at       timestamptz NOT NULL DEFAULT now(),
    role_granted_at timestamptz
);
```

`discord_user_id` is `text`, not `bigint`: a Discord snowflake is transported as a
decimal string and its magnitude is documented as unbounded. (The 2026-09-04
`discord_links` table this replaces used `bigint`; it was dropped with the removed
contribution bot and nothing carries over.)

`UNIQUE` on `discord_user_id` is the substantive constraint: without it two freehire
accounts could point at one Discord account and share a single subscription's access.
A collision is refused with a message naming the conflict, not silently reassigned.

`role_granted_at` is NULL when the role is not currently held. It exists so the sync
worker can tell "never granted" from "granted and should be revoked" without asking
Discord.

### Who grants and revokes

A new run-once-and-exit worker, `cmd/discord-sync`, hourly. For each row it resolves
the tier with `plan.TierOf` and:

- paying → `PUT /guilds/{guild}/members/{user}/roles/{role}`
- not paying → `DELETE` the same path

Both are idempotent; Discord answers `204` either way. There is no gateway
connection and no daemon — role management is three plain REST calls, so this fits
the repository's cron-worker model without a new long-lived process.

**Why its own worker rather than a fourth pass in `cmd/billing-sync`:** the layering
guard forbids it. `billing` is in the `identity` block (layer 3); Discord is outbound
community engagement and belongs in `engage` (layer 7), and layer 3 may not import
layer 7. It is also the same separation `billing-sync`'s own doc comment argues for
between providers: a Discord outage must not stop payment reconciliation.

Bound one run with `DISCORD_SYNC_MAX_PER_RUN` (default 500) read through
`worker.EnvInt64`, matching `billing-sync`.

### Package layout

New package `internal/engage/discordlink`, added to the `engage` list in
`internal/platform/arch/layering/blocks.go` (a package in neither list fails the
guard). It holds:

- `discordlink.go` — the domain: what a link is, what tier warrants the role.
- `client.go` — the four Discord REST calls (token exchange, `@me`, join guild,
  grant/revoke role) over `internal/platform/safehttp`.
- `service.go` — link, unlink, sync-one.
- `repository.go` — the sqlc-backed store.

Handlers live in `internal/api/handler` beside the other integration routes.

### Configuration

Five values in `config.Config`, read from the environment:
`DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`, `DISCORD_BOT_TOKEN`,
`DISCORD_GUILD_ID`, `DISCORD_PAID_ROLE_ID`.

Incomplete credentials disable the feature exactly the way `TelegramBotToken` does:
the routes 404, the public config reports it disabled so the SPA omits the card, and
the worker is a no-op that never opens the pool.

### Configured by hand in Discord, not in code

Two steps no code performs, both recorded in `deploy/AGENTS.md`:

1. On each closed channel, deny `View Channel` to `@everyone` and allow it to `Paid`.
2. **Drag the bot's own role above `Paid` in Server Settings → Roles.** A bot cannot
   manage a role positioned above its own; this is the single most common failure of
   role-granting bots and it fails silently from the site's side.

### Failure handling

| Situation | Behaviour |
|---|---|
| User unlinks on `/my/integrations` | Role revoked in the same request, row deleted. Membership is left alone — we invited them, we do not evict them. |
| User leaves the guild themselves | Next sync gets `404 Unknown Member`. Not an error: clear `role_granted_at`, keep the row, move on. |
| Discord `429` | Honour `Retry-After`; the run ends when its bound is reached and the next hour resumes. |
| Subscription lapses at 03:00 | Role is gone within the hour, not instantly. Accepted. |
| Another account already linked that Discord id | `409` with a message naming the conflict. |

### Testing

- `internal/engage/discordlink`: unit tests over the tier→role decision and the
  client's response handling (`204`, `404 Unknown Member`, `429`) against an
  `httptest` server.
- `internal/api/handler`: integration-tagged tests for connect/callback/unlink,
  including the duplicate-`discord_user_id` refusal.
- `internal/platform/db`: integration-tagged tests for the queries.

No test talks to Discord.

## Out of scope

- A second role for Ultra. One `Paid` role for every paying tier; a per-tier split
  can be added later without reshaping anything here.
- Any bot presence in the server beyond role management: no slash commands, no
  gateway, no message reading.
- Backfilling existing subscribers. They link when they choose to.
