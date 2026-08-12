# Per-user LLM spend attribution

## Scope
`internal/llmkey` — minting, reading and retiring the credential the LLM gateway knows an
account by, and the resolver that hands it out. The clone that actually spends under it
lives in `internal/llm` (`Client.As`); the one place per-user calls resolve through is
`userLLM` in `internal/handler`.

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
- **Unconfigured is a normal deployment, not a degraded one.** No `LLM_ADMIN_URL` or no
  `LLM_ADMIN_KEY` ⇒ `New` returns nil, nothing is minted, and every call goes out on
  `LLM_API_KEY` exactly as it did before this existed. That is also the rollback.

## The endpoints are not where you would guess
Administration lives at the gateway **root**, inference under `/v1`: every call this
client makes — `/key/generate`, `/key/block`, `/key/delete`, `/user/daily/activity` —
is a root path on the ADMIN key. `LLM_ADMIN_URL` is therefore configured separately and
never derived from `LLM_BASE_URL` — which also allows keeping the admin API off the
public host.

## Three rules that are easy to get wrong

**The usage read authenticates as the administrator, scoped by account id — never AS
the key.** `Client.Activity` reads `GET /user/daily/activity` on the admin key with the
account id in the query string: an internal number, not a secret, so it can travel where
a credential could not. The handler above it (`newUsageHandlers` in `me_usage.go`) takes
no resolver on purpose — the read needs no key, cannot mint one, and still reports a
month during which the key was replaced.

**A 401 on a per-user call means the gateway forgot the key; on an administrative call it
means our own admin key is wrong.** Conflating them would let one mistyped environment
variable read as "every account's key is stale" and set off a re-minting storm. The two
are kept apart structurally rather than by a sentinel: `do` classifies only 404 (→
`ErrUnknownKey`, treated by `Block`/`Delete` as already done) and sends every other
non-2xx, 401 included, to the generic `ErrUpstream` branch. The 401 that matters — the
gateway refusing a user's credential on an inference call — is caught in `internal/llm`'s
transport, which retries once on the fallback and calls `Resolver.Forget` to clear the
row so the next call re-mints.

**Block, do not delete, a credential that has been spent with.** The gateway's record of what
a key spent hangs off that key, so deleting it takes the cost history along. `Revoke` (account
deletion) blocks; `Delete` is reserved for a credential with nothing to preserve — the loser
of a minting race, seconds old and never stored.

## Minting races
Two concurrent first calls both mint. The conditional claim
(`WHERE id = $1 AND llm_key IS NULL`) admits exactly one; the loser adopts the winner's value
and **deletes the one it minted**, which would otherwise sit at the gateway forever spending
nothing and appearing in every listing.

An unreadable store is NOT an account without a credential. `read` returns both the value and
whether the store answered, because minting on a database fault would issue a fresh key per
request, each unstorable and each abandoned.

## Where the spend actually lands
`LiteLLM_DailyUserSpend` and `LiteLLM_DailyTagSpend` hold months of daily rows — tokens,
cost, request and failure counts, per key, per model, per tag. Only `LiteLLM_SpendLogs` is
pruned (to a few days). This is why the change writes no cost table of its own.

**The figures are list-price, not an invoice.** The gateway prices from its model table while
the pool behind it runs on mixed upstream keys. They compare features and periods to each
other honestly and must never be quoted as a bill.

## Adding a per-user LLM call
1. Resolve through `userLLM` in `internal/handler` — the single identifier to grep for.
2. Give it a `feature:` tag from the constants in `user_llm.go`. One tag per thing a person
   can ask for; do not tag two surfaces the same or the report stops answering the question
   it exists for.
3. If the service holds its client at construction, give it a one-line `As(*llm.Client)`
   clone rather than threading a second client through its constructor.
4. Resolve BEFORE opening a stream. Minting is a network call, and making it after the
   headers are out stalls a stream the client is already reading.
