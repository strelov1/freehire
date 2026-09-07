## Why

The community Discord is entirely public, so a Pro or Ultra subscription buys nothing
there. A members-only space is a perk that costs no LLM spend to deliver and gives Pro
a reason to exist beyond daily allowances — but only if access appears when someone pays
and disappears when they stop, without an operator moving people by hand.

## What Changes

- A signed-in user can link their Discord account from `/my/integrations`, alongside the
  existing Gmail, Calendar and Telegram cards. One click: consent, and they are on the
  server with the paid role if they are paying.
- A new `discord_links` table records the binding, one Discord account to at most one
  freehire account.
- A new hourly worker `cmd/discord-sync` grants the paid role to linked accounts that
  are paying and revokes it from those that are not, resolving "paying" through the
  existing `plan.TierOf` (Stripe + RevenueCat + granted promo time), never by reading a
  payment provider directly.
- A new `internal/engage/discordlink` package holds the domain, the Discord REST client
  and the store, and is registered in the layering table.
- Five new environment values. Incomplete credentials disable the feature the way
  `TELEGRAM_BOT_TOKEN` does: routes 404, the SPA omits the card, the worker never opens
  the pool.
- Channel permissions and the bot's role position are configured by hand in Discord and
  recorded in `deploy/AGENTS.md`. No code performs them.

No breaking changes: nothing today depends on Discord beyond the outbound digest
webhook, which this does not touch.

## Capabilities

### New Capabilities

- `discord-paid-access`: linking a Discord account to a freehire account, and keeping the
  paid role on that Discord account in step with the freehire subscription.

### Modified Capabilities

<!-- None. TierOf is consumed as it stands; no existing requirement changes. -->

## Impact

- **New:** `internal/engage/discordlink`, `cmd/discord-sync`, migration
  `0144_discord_links.sql`, queries in `internal/platform/db/queries/`.
- **Modified:** `internal/platform/config` (five values, public-config flag),
  `internal/api/handler` (three routes), `internal/platform/arch/layering/blocks.go`
  (the new package), `web/src/lib/components/IntegrationsView.svelte` (a fourth card),
  `deploy/AGENTS.md` and the systemd unit + timer under `deploy/`.
- **External:** a Discord application with a bot must be created and invited to the
  guild by hand before any of this does anything on production.
- **Dependencies:** none added. The four Discord REST calls go over a plain `http.Client`
  — the host is a fixed vendor endpoint from operator configuration rather than user
  input, so there is no SSRF surface for `internal/platform/safehttp` to guard, which is
  the reasoning `internal/engage/socialdigest` already records for the digest webhook. No
  Discord library and no gateway connection.
