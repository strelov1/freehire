## Context

The in-app assistant is three moving parts across three repositories: the SPA chat
at `/my/assistant`, the `freehire-agent` daemon (a trimmed roy fork) that spawns the
harness, and the `freehire` CLI, which is the harness's only shell tool. The chain
ends at this repository's HTTP API.

Two facts explain the behaviour this change fixes:

- `roy-management` injects `FREEHIRE_TOKEN` into a session's environment **only for
  tailoring sessions**; the code says so outright — "A normal assistant session
  leaves it unset — its `freehire` calls are public reads."
- `GET /me/profile` is registered behind `mw.cookie` (`RequireAuth`), which reads
  `c.Cookies(CookieName)` and nothing else, so a bearer credential cannot reach it.

So the assistant has no identity, and the one profile endpoint would reject it even
if it had. What the assistant needs is the profile the user already curated:
specializations, skills, excluded skills and location preferences — plus seniority,
which lives not in the profile but in the structured résumé (`hardconstraint_inputs.go`
reads it from `resume.Structured` for exactly this reason).

## Goals / Non-Goals

**Goals:**

- One authenticated call returns everything the assistant needs to search on the
  user's behalf without interrogating them.
- The call is a normal part of the CLI's surface, usable by any key holder — not a
  side door built for one consumer.
- The response carries no contact details, so nothing personally identifying lands
  in a chat transcript by default.

**Non-Goals:**

- Narrowing what the assistant may do. The assistant authenticates with the user's
  session token, which grants full user rights; this change does not add a scope to
  constrain that.
- Changing the web app. The profile page keeps reading contacts from `/me/resume`.
- Giving the assistant write access to the profile. Writes stay cookie-only.

## Decisions

### The assistant authenticates with the session JWT, not a minted key

A server-side sandbox has no cookie jar, so "just use the cookie" is not available:
cookies are a browser mechanism. The two realistic options were minting a
short-lived API key (what CV tailoring does, via `mintTailoringKey`) or passing the
user's own `hire_token` — the value inside the cookie — into the session
environment, where the CLI sends it as a bearer credential. `resolveCredential`
accepts a session JWT as a bearer identity, so the second works with no change to
authentication anywhere.

We chose the JWT. It costs no new scope, no bootstrap endpoint and no SPA round
trip, and `roy-management` already holds the cookie because it verifies it at the
handshake.

The price is real and accepted: a bearer session resolves with `viaKey: false`, so
`scopeAllowed` never runs and the credential carries the user's full rights. A
minted key would have been narrower and individually revocable, where a JWT is
revocable only by bumping `token_version` — that is "sign out everywhere", which
takes the user's browser sessions with it.

### The contact-free `cv` block hangs off `/me/profile`, not a new endpoint

Because the assistant presents a session rather than a key, the server cannot tell
it apart from the browser; withholding fields based on the credential type would not
catch it. The boundary therefore has to be drawn by resource, not by caller.

`/me/profile` is the right resource: `userprofile`'s package documentation calls it
the user's "professional self", and contacts are not part of a professional profile.
They already live on `/me/resume`, which is where the profile page's contact card
reads them. Splitting this way keeps the endpoint honest — it is not "the endpoint
the agent may call", it is the endpoint that carries professional data — which is
also why the CLI can expose it to every key holder without qualification.

Consequence worth stating plainly: with a full-rights token this is **hygiene, not
enforcement**. The same token can reach `/me/cvs/:id`, which does carry contacts.
The projection stops contacts from arriving by accident, not by determination.

### The projection is a field whitelist, and `internal/pii` is not used

`internal/pii` is a client to a separate service, built for free text where the
location of personal data is unknown. Here the input is a typed struct whose contact
fields are known by name, so a network hop would buy nothing.

It is also unnecessary: `resumeextract.Extract` already severs contacts fail-closed
at extraction. The raw CV is redacted before the model sees it, the model is told
the contacts appear as `[REDACTED_…]` placeholders, and `full_name`/`email`/`phone`/
`links` are then filled deterministically from detection spans. The free-text fields
the model produced — `summary`, `highlights` — could not have picked up a name,
because the model never saw one.

Whitelist rather than blacklist so the failure mode is safe: a field added to
`Structured` later is withheld by default instead of leaking until someone notices.

### The read widens to `mw.key`; writes stay cookie-only

This is the pattern the CV endpoints already use, with the reasoning recorded there:
reads accept a key so an agent's CLI can fetch, mutations stay cookie-only because
the browser owns authoring. The same split applies here — a leaked key must not be
able to rewrite or clear someone's profile.

## Risks / Trade-offs

- **The assistant holds a full-rights credential** → Accepted deliberately. The
  session token is scoped to the user and revocable by "sign out everywhere"; the
  sandbox's shell allowlist still limits the harness to `freehire` invocations. If
  this proves too broad, the narrowing move is the minted key with a dedicated
  scope, which this change leaves open rather than precludes.
- **The token is a snapshot taken at spawn** → A session's environment is set once,
  and re-attaching does not refresh it, so a token invalidated by "sign out
  everywhere" leaves that chat permanently unauthenticated. Mitigation is
  instructional: the skill tells the agent to treat a `401` as "start a new chat"
  rather than trying to recover.
- **The profile response grows a field** → The generated TypeScript contract and any
  consumer that round-trips the response must be regenerated (`cmd/gen-contracts`).
  Existing fields are untouched, so readers that ignore unknown keys are unaffected.
- **Two copies of the `using-freehire` skill have drifted** → The agent's bundled
  copy still points at `freehire.dev` and lacks sections the CLI's copy has. Editing
  only one leaves the assistant reading stale instructions; both get the edit.

## Migration Plan

No schema change and no data migration. The endpoint's response is additive, so the
API can ship before the CLI and the agent. Order: this repository first, then the
CLI release, then the agent's environment injection and the skill edits — until the
last of those lands the assistant simply carries on as it does today.

## Open Questions

None. The scope question (minted key with a dedicated scope versus the session
token) was decided in favour of the session token, with the narrowing path left
available.
