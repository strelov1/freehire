## Context

Every LLM call in this system goes out on one credential, `LLM_API_KEY`, and that value is the
gateway's **master key** (verified 2026-08-01 by comparing hashes of the two). The gateway
therefore sees one anonymous spender, applies no budget to it — LiteLLM exempts the master key —
and hands the application an administrative credential it has no business holding.

What the gateway does keep is better than what we were about to build. `LiteLLM_DailyUserSpend`
and `LiteLLM_DailyTagSpend` both hold roughly six months of daily rows, carrying
`prompt_tokens`, `completion_tokens`, `spend`, `api_requests`, `successful_requests` and
`failed_requests` per day, per key, per model, per tag. Only `LiteLLM_SpendLogs` is pruned, to
three days, by a retention job on the gateway. The durable cost history already exists; it is
simply all filed under "anonymous".

The mechanism is proven on that same gateway: a sibling product already mints one virtual key
per tenant against it, with `max_budget` and `budget_duration` set. A probe key minted and
deleted during planning confirmed `/key/generate`, `/key/info` and `/key/delete` all answer,
and that `/key/info` returns `spend`, `max_budget`, `budget_duration` and a computed
`budget_reset_at`.

**Where the gateway lives is configuration and stays there.** Its URL and both credentials come
from the environment (`LLM_BASE_URL`, `LLM_API_KEY`, and the admin key added below). This
repository is public: no hostname, address, or key belongs in it, including in a comment or a
test fixture.

## Goals / Non-Goals

**Goals:**

- Every call made for a user is spent under a credential naming that user.
- Every call says which feature it served.
- A user can read their own spend.
- A user's credential dies with their account.
- Nothing a user does today starts failing because of any of it.

**Non-Goals:**

- Choosing a budget number, or refusing a call for exceeding one. The mechanism ships; the
  policy is chosen once the distribution exists.
- Reconciling LiteLLM's dollar budget with `internal/credits`' points. Credits price the
  product; a gateway budget is a fuse. Conflating them would make one of the two lie.
- Moving the workers off the master key. They should eventually run on a named service key,
  but that is a deployment change with no user-visible half and does not belong here.
- Dictation. Transcription bills by audio duration on a different endpoint; it can adopt the
  same credential later, but its unit is not tokens and pretending otherwise distorts the
  numbers this change exists to produce.

## Decisions

### The credential is resolved per call, not held per service

`internal/matchanalysis`, `internal/atscheck`, `internal/resumeextract` and the assistant all
take a `*llm.Client` at construction today. That client is process-wide and cannot name a
user. Rather than thread a second client through every constructor, `llm.Client` grows a
method that returns a clone bound to a given credential and tag, and each per-user call site
resolves the caller's credential immediately before calling.

*Alternative considered:* caching a built client per user. Rejected — `openai.New` builds a
struct and an HTTP client, which is nothing beside a model call that takes between 100ms and a
minute, and a cache keyed on user credentials needs an eviction policy and acquires a
stale-credential bug the moment a key is re-minted.

### A re-credentialed clone MUST NOT share the schema-model cache

`Client.WithTimeout` clones shallowly and deliberately shares `schemaModels` by pointer. That
cache is keyed on `schemaName + rendered format` and **not** on the credential
(`internal/llm/schema.go:118`). A clone that changed the token while sharing that cache would
serve user B's schema-bound call on the model built with user A's token — a cross-account
credential leak that no test asserting "the call succeeded" would catch.

The re-credentialing clone therefore allocates a fresh `modelCache`. The cost is that a
per-user schema-bound call rebuilds its schema-bound model each time, which is the same cheap
construction as above. This is the single most dangerous line in the change and the reason it
gets its own requirement-shaped test.

### A forgotten credential is rescued at the transport, not above it

If the gateway loses a key we hold, every call that user makes gets a `401` — and without
handling, their assistant simply stops working until something else notices. Detecting that
above the client means reading langchaingo's error text, which works until the library
rewords it and then fails silently. The status code does not move.

So the client's own transport does it: on a `401` it re-issues the request once on the
service credential and calls back so the stale value can be replaced. The user's call
completes and they see nothing. Once only — a gateway refusing the service credential too is
a misconfiguration, and looping on it would turn one bad key into a request storm.

A request whose body cannot be replayed returns the original refusal rather than inventing a
retry, because a half-sent body is not the call the caller made.

### The feature tag rides in a header

LiteLLM reads `x-litellm-tags` and files the request under each tag in `LiteLLM_DailyTagSpend`.
langchaingo has no way to add a header to a call, but `openai.WithHTTPClient` accepts a `Doer`,
and this package already injects into outgoing requests exactly that way (`schemaInjector`,
`internal/llm/schema.go:157`). Tagging is the same pattern applied to a header instead of a
body field.

Tags are a list, so an assistant turn sends both its feature and its preset
(`feature:assistant`, `preset:tailor`). The gateway files one row per tag, which is what makes
"which preset is expensive" answerable without a table of ours.

### Minting races are resolved by the database, and the loser's key is cleaned up

Two concurrent first calls from one account can both mint. The store writes with
`UPDATE users SET llm_key = $2 WHERE id = $1 AND llm_key IS NULL RETURNING llm_key`, so exactly
one wins; the loser reads back the winner's value, uses it, and **deletes the key it minted**
at the gateway. Leaving it would accumulate orphan credentials that spend nothing but appear in
every key listing forever.

### Everything about this fails open

Minting fails, the admin API is down, the stored key is rejected: the call proceeds on the
service credential and the failure is logged. A rejected key is additionally re-minted, so a
gateway that lost its database heals itself on the next call rather than bricking every
account until someone notices.

This is the same rule the follow-up endpoint and the OAuth revoke already follow. The thing
being protected is the answer the person asked for; the thing being sacrificed is our record of
who paid for it.

### The usage read authenticates as the user's own key, not as the administrator

`/key/info` answers either to the administrator naming a key in the query string, or to the
key itself presented as the bearer with no query at all. It must be the second, for two
independent reasons.

The gateway logs `$request_uri`. A credential in a query parameter is therefore written to
an access log in plaintext on every usage read, where it accumulates for as long as logs are
kept — and unlike the database copy, nothing rotates it. The bearer header is not logged.

And it removes administrative rights from the only gateway call a user's own request can
reach. A user key cannot mint keys (verified: the gateway answers `401`), so the worst a
compromised read path can do is read the line it was already showing.

The cost is that a refusal changes meaning with the credential. A `401` on the self-read means
the gateway has forgotten that key — mint a replacement. A `401` on an administrative call
means our own admin key is wrong. Conflating them would let one mistyped environment variable
read as "every account's key is stale" and set off a re-minting storm, so the client returns an
internal `errUnauthorized` and each caller classifies it. Both directions carry a test.

### `/me/usage` reads the gateway and never fails

`GET /key/info?key=<user key>` returns `spend`, `max_budget` and `budget_reset_at`. An account
with no key yet, and a gateway that cannot be reached, both answer `200` with zeroes. A usage
readout is informational; rendering an error where a zero belongs would make a proxy blip look
like a billing fault.

### The key is stored in plaintext, and that is a deliberate trade

We must present the value on every call, so it cannot be hashed. It could be encrypted with the
cipher the Gmail integration already uses, and is not: that would make an unrelated feature's
key material a boot requirement for all AI, to protect a credential whose entire power is to
spend inference against **our own** gateway, under a budget we set, revocable in one admin call
for every account at once.

The honest statement of the risk is that a database dump hands the holder that spend until the
keys are rotated. Rotation is `/key/delete` over the column plus clearing it, which is a script,
not a project. If a ceiling is ever configured, the blast radius is bounded by it.

### A departing account's credential is blocked, not deleted

`internal/accountdelete` already models the ordering: it collects blob keys *before*
`DeleteUser` because the keys live in rows about to vanish, and it revokes the Google grant
best-effort because a member leaving must not wait on a third party. The gateway credential
is the same shape and slots in beside the revoke.

What differs is what retiring it means. Deleting the key at the gateway takes the gateway's
own record of what was spent with it — and that record is the cost history this whole change
exists to produce. So deletion **blocks** instead: verified against the gateway, a blocked
key answers `401` on a chat call and stays fully readable through `/key/info` with
`blocked: true`.

What is left behind is a blocked credential labelled with an internal numeric id, which maps
to nobody once the account row is gone. The deletion is what anonymises it; the gateway side
carries no name, address or content.

Erasure is kept for the one credential with nothing to preserve: the loser of a minting
race, seconds old and never stored. Blocking that one would accumulate permanent junk in
every key listing for no gain.

### Configuration names the two credentials apart

`LLM_ADMIN_KEY` is the admin credential used only for `/key/*`. `LLM_API_KEY` stays the
credential inference runs on when there is no user. Today an operator will set both to the same
master key, and that is fine — the point is that once they are separate names, making
`LLM_API_KEY` a plain service key is a deployment change rather than a code change.

`LLM_ADMIN_URL` names the admin API separately from `LLM_BASE_URL`, because they are not the
same endpoint. Verified against the gateway: `/key/info` answers `200` while `/v1/key/info` and
`/v1/key/generate` both answer `404` — inference is served under `/v1`, administration at the
root. Deriving one from the other by trimming a suffix would encode a guess about how an
operator wrote a URL, and it would foreclose the better posture this separation allows: the
admin API does not have to be reachable on the same hostname as inference, or publicly at all.

Either of `LLM_ADMIN_URL` or `LLM_ADMIN_KEY` unset disables the whole feature: no minting, no
resolution, every call on `LLM_API_KEY` exactly as today. Local development and tests get that
for free.

`LLM_USER_MAX_BUDGET` and `LLM_USER_RPM_LIMIT` are unset by default and passed through to
`/key/generate` only when present.

## Risks / Trade-offs

- **A new per-user call site can silently forget to attribute.** It keeps working, just
  anonymously, which is the failure mode hardest to notice. Mitigated by routing every site
  through one helper in the handler package, so the thing to grep for is a single identifier —
  not by a type the compiler enforces, which would be a larger refactor than the problem
  warrants today.
- **`/me/usage` and attribution now depend on a host with a history.** That proxy has filled
  its disk twice and serves 500s when it does. Everything reads through fail-open paths, so the
  worst case is zeroes on a page and a gap in attribution, never a refused turn.
- **Spend is list-price, not an invoice.** LiteLLM prices from its model table, while the model
  pool behind it runs on free and scavenged upstream keys. The numbers compare features to each
  other honestly and must never be quoted as a bill.
- **Plaintext key at rest.** Stated above; bounded by a configured ceiling and mass-revocable.
- **The master key stays in the env.** Minting requires it. What changes is that it is no
  longer what a chat turn travels on — the exposure narrows from every inference path to one
  admin path.

## Migration Plan

One migration adding a nullable column to `users`. Expand-only: nothing reads it until the
resolve path ships, so it can be applied ahead of the deploy with no ordering hazard. Take the
next free number after re-checking `origin/main` — `0066` was the last when this was written and
numbers have collided in this repo before.

Rollback is unsetting `LLM_ADMIN_KEY`: minting stops, resolution stops, every call falls back to
`LLM_API_KEY`, and the column goes unread. No code needs reverting to make that true, which is
the property worth having on a change that touches every AI path at once.

Already-minted keys survive a rollback and are picked up again when the variable returns.

## Open Questions

- Should the workers get a named service key in the same deploy? It is one `/key/generate` and
  an env change, and it would make `LLM_API_KEY` stop being the master key everywhere rather
  than only on the user path. Held back because it is a deployment change this repo cannot test.
- Does `/me/usage` belong on a page? The spec requires the endpoint; whether the SPA renders it
  is a product call, and building the API first costs nothing either way.
