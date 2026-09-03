# Per-user LLM spend attribution

## Scope
`internal/ai/llmkey` — minting, reading and retiring the credential the LLM gateway knows an
account by, and the resolver that hands it out. The clone that actually spends under it
lives in `internal/platform/llm` (`Client.As`); the one place per-user calls resolve through is
`Bind`, in this package (`bind.go`) — moved here from `internal/api/handler`'s `userLLM` once
`cmd/auto-apply` became a second caller with no reason to import `internal/api/handler`.

## Always true
- **The credential is invisible to the user.** No page, no field, no setting, and it
  appears in no API response. Minted lazily on an account's first AI call.
- **Attribution can never fail a call.** An unmintable credential, an unreachable admin
  API, an unreadable database and a rejected key all fall back to the service credential.
  `Resolver.For` returns `""` rather than an error precisely so a call site cannot forget
  to handle it.
- **Work that belongs to nobody keeps the service credential.** A catalogue vacancy has no
  owner; attributing its enrichment to whoever triggered a run would put a cost on someone
  who did not incur it. `scope_test.go` enforces this by parsing imports — a background
  entrypoint that reaches for this package fails the build's tests.
- **Unconfigured is a normal deployment, not a degraded one.** Any of `LLM_ADMIN_URL`,
  `LLM_ADMIN_USERNAME`, `LLM_ADMIN_PASSWORD` or `LLM_ADMIN_TEMPLATE_KEY` unset ⇒ `New`
  returns nil, nothing is minted, and every call goes out on `LLM_API_KEY` exactly as it
  did before this existed. That is also the rollback. The template key counts as
  configuration for a reason — see below; without it minting succeeds and the credentials
  it produces are refused everything, which is a worse failure than not minting at all.

## The endpoints are not where you would guess
Administration lives under **`/api`**, inference under `/v1`. Every call this client makes
— `POST /api/governance/virtual-keys`, `PUT` and `DELETE` on
`/api/governance/virtual-keys/{id}`, and `GET /api/logs/stats` — authenticates as the
ADMINISTRATOR over HTTP Basic, not with a bearer token. `LLM_ADMIN_URL` is therefore
configured separately and never derived from `LLM_BASE_URL` — which also allows keeping
the admin API off the public host.

## A credential is two values, and both are load-bearing
`Credential` carries a secret and an id. The secret is a bearer token and the only thing a
model call needs; the id is opaque and the only thing block and delete accept — aimed at
the secret they answer 404. Storing one without the other yields a credential that can
spend but never be revoked, which is exactly what account deletion has to do. They are
written and cleared in the same statement (`users.llm_key`, `users.llm_key_id`).

Rows minted before migration 0119 hold a secret and no id. That is a real state, not a
fault: they keep working for inference, and the first refusal sends them through the
existing forget-and-re-mint path, which replaces them with a pair. Nothing backfills.

## A key with no provider policy is a dead key
This gateway denies every provider to a virtual key that carries no `provider_configs`, so
minting without one produces credentials refused on their first call. `Mint` therefore
reads the policy from the virtual key named by `LLM_ADMIN_TEMPLATE_KEY` and copies it.

That indirection is the point. The alternative — carrying the provider list in this
service's configuration — would put the provider vocabulary in two places and make adding
a provider a deployment of freehire rather than an edit to the gateway's own config.

One asymmetry to keep in mind when touching that copy: a read answers `key_ids` as null
where a write requires `["*"]`, and echoing the null back mints a key pinned to no
provider key at all.

## Three rules that are easy to get wrong

**The usage read authenticates as the administrator, scoped by the credential's id —
never AS the credential.** `Client.Activity` reads `GET /api/logs/stats` as the
administrator with the virtual key's id in the query string: opaque, not a secret, so it
can travel where the credential could not. The handler above it (`newUsageHandlers` in
`me_usage.go`) therefore DOES take the resolver, and takes it to read — `Stored`, never
`For`. Minting on a page view would issue a credential to every visitor who opened it out
of curiosity. An empty id short-circuits to zeroes without asking the gateway anything,
which is both the never-used-AI account and the row that predates 0119.

**A 401 on a per-user call means the gateway forgot the key; on an administrative call it
means our own admin key is wrong.** Conflating them would let one mistyped environment
variable read as "every account's key is stale" and set off a re-minting storm. The two
are kept apart structurally rather than by a sentinel: `do` classifies only 404 (→
`ErrUnknownKey`, treated by `Block`/`Delete` as already done) and sends every other
non-2xx, 401 included, to the generic `ErrUpstream` branch. The 401 that matters — the
gateway refusing a user's credential on an inference call — is caught in `internal/platform/llm`'s
transport, which retries once on the fallback and calls `Resolver.Forget` to clear the
row so the next call re-mints.

**Block, do not delete, a credential that has been spent with.** This survives the gateway
change but its reason has weakened: the usage log is keyed by the credential's id and
outlives the credential, so deleting no longer erases the history the way it used to. What
blocking still buys is legibility — a retired key stays in the gateway's listings. `Revoke`
(account deletion) blocks; `Delete` is reserved for a credential with nothing to preserve —
the loser of a minting race, seconds old and never stored.

## Minting races
Two concurrent first calls both mint. The conditional claim
(`WHERE id = $1 AND llm_key IS NULL`) admits exactly one; the loser adopts the winner's value
and **deletes the one it minted**, which would otherwise sit at the gateway forever spending
nothing and appearing in every listing.

An unreadable store is NOT an account without a credential. `read` returns both the value and
whether the store answered, because minting on a database fault would issue a fresh key per
request, each unstorable and each abandoned.

## Where the spend actually lands
In the gateway's own request log, one row per call, keyed by the credential's id.
`GET /api/logs/stats` aggregates it over a window; that is what `/me/usage` reads and why
this package writes no cost table of its own.

Two consequences worth knowing before trusting a figure:

- **The read is scoped to ONE credential.** The gateway offers no way to ask by account,
  so a key replaced mid-month leaves the earlier one's calls out of the total. Re-minting
  only happens when the gateway refuses a stored key, so this is rare — and the fix, a
  remembered list of retired ids, would keep dead credentials alive to make a counter
  marginally more complete.
- **The log write is asynchronous**, landing about five seconds after the call. Harmless
  over a month, fatal to any read-after-write assumption.

Per-feature cost is NOT an API question here. The `feature` dimension reaches the log row,
the OpenTelemetry span, and — only while `feature` is listed in the gateway's
`prometheus_labels` — the metrics. "What does tailoring cost" is therefore a Grafana query;
`/api/logs/histogram/cost/by-dimension` aggregates only by provider, team, customer, user
and business unit.

**The figures are list-price, not an invoice.** The gateway prices from its model table while
the pool behind it runs on mixed upstream keys. They compare features and periods to each
other honestly and must never be quoted as a bill.

## Adding a per-user LLM call
1. Resolve through `Bind` (this package, `bind.go`) — the single identifier to grep for.
   `internal/api/handler`'s `llmBinding.bind` is a thin per-request wrapper over it, not a
   second implementation.
2. Give it a feature tag — the constants live beside each caller (`internal/api/handler/
   user_llm.go` for request-driven surfaces, `tagAutoApplyDrafting` in
   `internal/api/atsapply/client.go` for `cmd/auto-apply`). They are bare words — the header
   `x-bf-dim-feature` names the dimension, so a `feature:` prefix in the value would file
   the spend under its own two-part label instead of alongside the others. One tag per
   thing a person (or a queue-driven attempt on their behalf) can ask for; do not tag two
   surfaces the same or the report stops answering the question it exists for.
3. Only `cmd/server` and `cmd/auto-apply` may import this package at all — `scope_test.go`
   enforces it structurally. A new binary that needs per-user attribution adds itself there
   by name, with its own justification; the check is deliberately not a pattern that would
   admit one by accident.
4. If the service holds its client at construction, give it a one-line `As(*llm.Client)`
   clone rather than threading a second client through its constructor.
5. Resolve BEFORE opening a stream. Minting is a network call, and making it after the
   headers are out stalls a stream the client is already reading.
