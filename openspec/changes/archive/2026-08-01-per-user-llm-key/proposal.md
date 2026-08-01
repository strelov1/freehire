## Why

freehire calls its LLM gateway with the gateway's **master key**. Verified on 2026-08-01: the
credential the application runs on is byte-for-byte the gateway's own administrative one.
Three consequences, all live:

- **Nobody is attributable.** Every call — a chat turn, a tailoring run, a match analysis,
  an enrichment batch — lands in one anonymous pool. There is no answer to "what did this
  account spend" or "which feature is expensive".
- **No spend limit is possible.** LiteLLM does not apply `max_budget` to the master key, so
  no ceiling can be placed on the current credential at all.
- **The application holds admin access.** That key mints and deletes any virtual key, reads
  every tenant's spend, and rewrites budgets. It sits in the app's env file.

The proxy already keeps what we were about to build a table for. `LiteLLM_DailyUserSpend`
and `LiteLLM_DailyTagSpend` hold 24 837 and 14 648 rows spanning 2026-02-11 to today, with
tokens, cost, request counts and failure counts per day. Only `LiteLLM_SpendLogs` is pruned
(to three days). We do not need to write cost history down; we need to **say who we are**
when we spend.

This supersedes `meter-the-assistant-turn`, which proposed an `assistant_turn_usage` table
for one feature. That table would have answered less, for one feature, in tokens rather than
cost, and would not have bounded anything.

## What Changes

- Every signed-in user gets their own LiteLLM virtual key, minted lazily on their first AI
  call and stored against their account. **They never see it**: no setup, no page, no field.
- Calls made **on behalf of a user** go out on that user's key: the assistant turn and its
  follow-ups, CV tailoring, match analysis, CV extraction, the ATS review, the autofill
  planner.
- Calls that belong to **no user** keep the master key: `cmd/enrich`, `cmd/tg-extract`,
  embeddings, mail classification. A catalogue vacancy is nobody's, and inventing an owner
  for it would be a lie in the report.
- The master key also stays for what it is actually for — the admin API that mints, reads
  and deletes user keys.
- Each call carries a feature tag (`x-litellm-tags`), so the proxy's existing daily tag
  aggregate answers "which feature costs what" without a table of ours.
- `GET /api/v1/me/usage` reports the caller's own spend for the current period, read from
  the gateway.
- A user's key is deleted with their account, and re-minted automatically if the gateway
  ever stops recognising it.
- **No budget is set by default.** `max_budget` and `rpm_limit` become configuration, unset
  out of the box. The mechanism ships armed but not loaded — choosing a ceiling before seeing
  the distribution is the same guess this change exists to remove.
- **Nothing is refused.** If the gateway is unreachable or the key cannot be minted, the call
  proceeds exactly as it does today, on the master key. The fuse fails open.

## Capabilities

### New Capabilities

- `llm-spend-attribution`: naming the account behind each LLM call at the gateway, keeping
  unowned work on the service credential, and reporting a caller's own spend back to them.

### Modified Capabilities

None. No user-visible behaviour changes: no turn is refused, no response shape moves, and no
existing requirement gains or loses a clause. `assistant-agent-runtime` keeps its contract —
which credential a call travels on is invisible to it. The requirement that would modify it
is a *refusal*, and this change deliberately does not add one.

## Impact

**New code.** `internal/llmkey` — the admin client over `/key/generate`, `/key/info`,
`/key/delete`, plus the resolve-or-mint path and its cache. A migration adding the key column
to `users`. A handler for `/me/usage`.

**Modified code.** `internal/llm` grows a way to clone a client onto a different credential
and to tag a call. `cmd/server` passes the admin settings. Each per-user LLM call site
resolves the caller's key before calling; the worker entrypoints are untouched.

**Configuration.** `LLM_ADMIN_KEY` (the master key, for the admin API) separates from
`LLM_API_KEY` (the service credential inference runs on when there is no user). Today they
are the same value; naming them apart is what later lets the service credential stop being
the master key.

**Security.** The user-facing inference path stops carrying admin rights. The master key
remains in the env because minting requires it — but it is no longer what a chat turn
travels on.

**Schema.** One column on `users`. Expand-only; nothing reads it until the resolve path
ships.

**Not in scope, deliberately:** choosing a budget number, refusing a call on an exhausted
budget, and reconciling LiteLLM's dollar budget with `internal/credits`' points. Credits
price the product; a gateway budget is a fuse. They are not the same instrument and this
change does not pretend otherwise.
