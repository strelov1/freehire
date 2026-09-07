# Discord paid channels

Binds a freehire account to a Discord account, and keeps one role on the community server in
step with the subscription. The closed channels are gated on that role, in Discord, by hand —
no code here knows which channels exist.

## The one rule worth stating twice

**Whether an account pays is `plan.TierOf`, never a payment provider.** Those columns are
derived from three sources each — Stripe, RevenueCat (the app stores) and granted promo time
— so a component that asked Stripe would deny access to every store subscriber and everyone
holding a support grant. This is also why the two maintained open-source bots for this job
were rejected: each decides who pays by reading Stripe.

Every paying tier gets the SAME role. `WarrantsPaidRole` is the only place that decides, so a
future Ultra-only channel is a second role resolved there rather than a condition spread
across the worker and the link path.

## Shape

| File | What it is |
|---|---|
| `discordlink.go` | The decision, with no I/O: who warrants the role, and what reconciliation owes one binding (`Reconcile` → `ActionNone`/`Grant`/`Revoke`). |
| `client.go` | Discord's REST API: token exchange, `/users/@me`, join guild, grant/revoke role. |
| `service.go` | The use cases — link, unlink, `Sync`. |
| `repository.go` | The sqlc-backed store. |

There is **no bot process**. Everything Discord needs from us is a request/response, so this
runs inside an HTTP handler (linking) and `cmd/discord-sync` (reconciliation, hourly). Nothing
holds a gateway connection or reads a message.

A plain `http.Client`, not `internal/platform/safehttp`: the host is a fixed vendor endpoint
from operator configuration, not user input — the same reasoning
[socialdigest](../socialdigest/AGENTS.md) gives for the digest webhook.

## Traps

- **`ActionNone` must stay free.** Two of the four reconciliation cases need no Discord call
  at all. Without that the hourly timer would rewrite every role we manage, every hour, to
  change nothing.
- **But every row is still stamped.** `synced_at` is what moves a row to the back of the
  queue; the reconciliation page is ordered by it `NULLS FIRST`. Skip the stamp for the
  settled accounts and they pin the front of the queue forever, starving everyone behind the
  per-run bound.
- **`ErrUnknownMember` is an absence, not a failure.** Leaving a server is an ordinary thing
  to do. Counting it as a failure turns the hourly run red for it and buries the failures that
  matter.
- **`50013 Missing Permissions` almost always means the role hierarchy** — the bot's own role
  must sit ABOVE the paid role in Server Settings → Roles, or Discord refuses every grant.
  The client's error names that cause, because on its own the code reads as a generic
  permission problem and costs an hour of guessing.
- **Unlink deletes the binding even when the revoke fails.** A user must always be able to
  undo a link they made; an orphaned role is an operator's problem, a link that will not go
  away is theirs, every time they open the page.
- **The OAuth access token is never stored.** It is used inside the callback request for two
  calls and finished with. This is why Discord is NOT an entry in
  [`internal/identity/auth/oauth`](../../identity/auth/oauth/AGENTS.md): that registry answers
  "who is signing in", while this links an already-authenticated account.

## Configuration

Five values, all load-bearing: `DISCORD_CLIENT_ID`, `DISCORD_CLIENT_SECRET`,
`DISCORD_BOT_TOKEN`, `DISCORD_GUILD_ID`, `DISCORD_PAID_ROLE_ID`. Any one missing means the
feature is off — `config.Settings.DiscordPaidAccessConfigured` is the single decider, so the
routes (which 404 by not being mounted), the SPA card and the worker cannot disagree.

`DISCORD_DIGEST_WEBHOOK_URL` is a different Discord integration with a different credential.
Neither switches the other on.

## What is done by hand

The channel permissions and the bot's position in the role list are configured in Discord and
recorded in [deploy/AGENTS.md](../../../deploy/AGENTS.md). No code performs them, and nothing
here is verifiable on production until somebody with the server's admin rights has.
