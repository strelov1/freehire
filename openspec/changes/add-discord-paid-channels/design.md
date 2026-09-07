## Context

The brainstormed design this change implements is
[docs/superpowers/specs/2026-09-06-discord-paid-channels-design.md](../../../docs/superpowers/specs/2026-09-06-discord-paid-channels-design.md);
this document records the decisions that shape the code.

Current state: the community Discord (`discord.gg/sYnZksswR`) is public. The only Discord
code in the repository is the outbound digest webhook (`internal/engage/socialdigest`). A
`discord_links` table existed for the contribution bot and was dropped in migration 0134
along with the bot; nothing carries over from it.

Constraints that decide most of what follows:

- **`plan.TierOf` is the only answer to "is this account paying".** It reads
  `users.pro_until` and `users.ultra_until`, each derived from three source columns —
  Stripe, RevenueCat (App Store) and granted promo time. Any second answer is wrong for
  some paying users.
- **The block layering is enforced twice** (`depguard` and
  `internal/platform/arch/layering`). `billing` is `identity`, layer 3; outbound community
  engagement is `engage`, layer 7. Layer 3 may not import layer 7.
- **`internal/identity/auth/oauth` is sign-in only** and says so: "tokens are used once to
  resolve who the user is and are never stored."
- Every binary but three is a run-once-and-exit cron worker.

## Goals / Non-Goals

**Goals:**

- Access to closed channels appears when a subscription starts and disappears when it ends,
  without an operator touching Discord.
- One source of truth about who pays.
- No new long-lived process, no new third-party dependency.
- The feature is invisible and inert on a deployment without credentials.

**Non-Goals:**

- A separate Ultra-only channel. One `Paid` role covers every paying tier.
- Any bot presence beyond role management: no slash commands, no gateway connection, no
  message reading.
- Backfilling existing subscribers into the server. They link when they choose to.
- Instant revocation. Within the hour is the contract.

## Decisions

### Our own code, not an existing bot

Rejected [StripeCord](https://github.com/Rodaviva29/StripeCord) (33★, MIT, actively
maintained) and [Androz2091/stripe-discord-bot](https://github.com/Androz2091/stripe-discord-bot)
(48★, **no licence**, so unusable regardless). Each is a long-lived Node daemon with its own
database that decides who pays by reading Stripe directly. On this site that is the wrong
question — it would deny access to every App Store subscriber and everyone holding granted
promo time. Adopting one means running a second, disagreeing source of truth next to
`TierOf`, and a second datastore, to save perhaps 150 lines.

### Bot-assigned role, not Discord Linked Roles

Discord's native Linked Roles (role-connection metadata) would remove the need for a bot
with guild permissions entirely: we publish a metadata key, the server admin builds a role
around it. Rejected for two reasons. The role is only ever granted when the member clicks
through the Linked Roles tab themselves — we could not grant on payment, only wait. And
keeping the metadata current requires storing each user's OAuth **refresh token**, which is
strictly more secret material at rest than one bot token. Discord's own guidance puts
subscription gating on bot-assigned roles.

### Discord is not an entry in the OAuth registry

The registry in `internal/identity/auth/oauth` exists to answer "who is signing in" and
discards the token immediately. This flow links an already-authenticated account and needs
the token for a second call (`guilds.join`). Putting it in the registry would either
weaken that package's stated rule or bend the flow around a Provider interface that returns
an `Identity` and nothing else. It gets its own package.

### Its own worker, not a fourth `billing-sync` pass

`cmd/billing-sync` is the obvious home and the layering guard forbids it: `billing` is
layer 3, `discordlink` is layer 7. It is also the separation `billing-sync`'s own doc
comment argues for between providers — a Discord outage must not stall payment
reconciliation. `cmd/discord-sync`, hourly, `Type=oneshot`, bounded by
`DISCORD_SYNC_MAX_PER_RUN` (default 500) read through `worker.EnvInt64`.

### Four REST calls, no library

Token exchange, `GET /users/@me`, `PUT /guilds/{g}/members/{u}`, and
`PUT`/`DELETE /guilds/{g}/members/{u}/roles/{r}`. All four go over
`internal/platform/safehttp`. A Discord library (`disgoorg/disgo` is the healthy Go one)
would bring a gateway client and a command framework we would never start, to wrap four
URLs. Revisit if we ever want a bot that reads messages.

### `discord_user_id` is `text`

A Discord snowflake is transported as a decimal string and its magnitude is not contractually
bounded to 64 bits. The dropped 0134 table used `bigint`; that was a latent bug, not a
precedent. `UNIQUE` on this column is the constraint that stops one subscription serving
several people, and it is enforced in the schema rather than only in code.

### `role_granted_at` records the grant

Nullable timestamp: NULL means the role is not currently held. It exists so reconciliation
can tell "never granted" from "granted, now must be revoked" without asking Discord about
every account on every run.

### Unlink deletes the row even if Discord refuses

A user must always be able to undo a link they made. If the revoke call fails, the binding
is deleted anyway and the failure is logged; an orphaned role is an operator's problem,
whereas a link that cannot be removed is the user's.

### Absent credentials mean absent feature

Modelled exactly on `TelegramBotToken`: routes 404, the public config reports it disabled so
the SPA omits the card, the worker exits 0 without opening the pool. This is what lets the
change merge and deploy before the Discord application exists.

## Risks / Trade-offs

- **The bot's role sits below `Paid` in the hierarchy** → Discord refuses every grant, and
  it looks like our bug. Mitigation: an explicit ordered step in `deploy/AGENTS.md`, and the
  worker logs Discord's `50013 Missing Permissions` with that cause named rather than as a
  generic failure.
- **Nothing is verifiable until a Discord application exists**, which only a human with the
  server's admin rights can create. Mitigation: every test runs against `httptest`; the
  live check is a documented post-deploy step, not a prerequisite for merging.
- **Revocation lags by up to an hour.** Accepted: this is a chat perk, not an entitlement
  worth a webhook path of its own.
- **Discord rate limits** (`429`) → honour `Retry-After`; the per-run bound means a
  throttled run simply covers fewer accounts and the next hour resumes.
- **`guilds.join` looks broad to a cautious user.** Accepted in exchange for one-click
  onboarding; the consent screen names the server, and the alternative costs a second manual
  step for everyone.
- **Someone leaves the guild while keeping the link.** Reconciliation gets
  `404 Unknown Member`; treated as absence, not failure, so it does not turn every run red.

## Migration Plan

1. Merge and deploy with no credentials set. Feature is inert; the migration adds an empty
   table.
2. By hand, in Discord: create the application and bot, invite it with `Manage Roles`, create
   the `Paid` role, drag the bot's role **above** it, and set the closed channels' permissions.
3. Set the five environment values on the host, restart, install the timer.
4. Verify with one real account: link, confirm the role and channel access, cancel, confirm
   the role is gone after a run.

Rollback: unset `DISCORD_BOT_TOKEN`. Routes 404 and the worker stops acting; nobody's role
changes, so nobody loses access mid-subscription. The table stays.

## Open Questions

None blocking. Whether Ultra eventually gets its own channel is a product decision that this
design leaves room for: a second role id and a tier comparison, no reshaping.
