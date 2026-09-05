## Context

See proposal.md for motivation and this session's own spike (verified against this repo's
real endpoints and a real LLM call: a run paused on `step.WaitForEvent`, a signal sent
minutes later resumed the same run, the decision landed in Postgres).

Relevant existing pieces:
- `POST /me/auto-apply/:queueId/{tailor,review}` (`internal/api/handler/auto_apply_tailor.go`,
  `auto-apply-tailored-resume`) — both `mw.key` (cookie or full-scope Bearer API key), both
  scoped by ownership: `GetAutoApplyQueueEntryForReview` resolves by `(id, user_id)`, so a
  human caller can only ever act on ITS OWN queue entries. That change's own design.md named
  the caller shape as "an automation platform... calling freehire's own API with a per-user
  API key" — this design revisits that (see Decisions: a per-user credential turned out to be
  the wrong shape for a caller with no human to hold a secret for it).
- `cmd/mail-ingest` — the one existing precedent for a long-lived (`Restart=always`) daemon
  outside `cmd/server`, everything else in `cmd/` being run-once-and-exit.
- `internal/api/ratelimit` — the shared Redis-backed rate limiter every other throttled route
  already uses (`ratelimit.Middleware(throttler, keyFunc, limit, window)`); the orchestrator's
  own call budget reuses it rather than inventing a second limiting mechanism.

## Goals / Non-Goals

**Goals:**
- Make `auto-apply/submit` → tailor → durable wait → review a real, running sequence, not
  documentation of an assumed caller.
- Keep the tailor/review HTTP endpoints themselves completely unchanged — the orchestrator is
  a caller, not a reason to touch their contract, ownership rule, or response shape.
- Self-host Inngest on infrastructure this repo already runs (Postgres), not a new managed
  cloud dependency.

**Non-Goals:**
- Mid-run, per-requirement pause inside the tailoring LLM loop (proposal.md's own Non-Goal —
  restated here because it is the design-level reason `step.Run("tailor", ...)` is one opaque
  call rather than several).
- What emits `auto-apply/submit` in production (`auto-apply-tailored-resume` tasks.md's own
  deferred item 2.1) — this design assumes the event already carries a `queueId` for an
  entry that exists, and stops at "nothing emits it yet in production; this session's spike
  emitted it by hand for verification."
- Per-candidate credentials or scoping of any kind for the orchestrator's own calls (see
  Decisions) — there is exactly one shared secret for the whole process, so there is nothing
  per-candidate to build, show, or revoke.

## Decisions

**One shared, static secret for the whole orchestrator process — not a per-user
credential.** A per-user credential was the first design here and was abandoned
mid-implementation: the ownership check on both endpoints resolves a caller's user id from
whatever authenticated it (a session cookie or an `api_keys` hash), so a per-user credential
would need to be a *live, re-presentable* secret — but the only storage shape that fits how
`api_keys` verifies a credential (a one-way SHA-256 hash) can never give that secret back to
mint it once, hand it to a human, and never need it again; there is no human here, and the
SAME run needs to present the SAME credential again after `step.WaitForEvent` pauses for
however long the candidate takes to decide, possibly across a process restart. Making the
secret recoverable again (reversible encryption, or threading it through Inngest's own step
memoization) is real weight for a caller that is not actually a second user needing its own
scoped identity — it is one trusted internal process. So: one static value
(`AUTO_APPLY_ORCHESTRATOR_SECRET`), configured once, presented as a Bearer credential by
`cmd/auto-apply-orchestrate` and compared in constant time by the two auto-apply routes.
Because this caller is authenticated by the deployment's own secret rather than by acting
AS the candidate, the routes resolve which user's entry to act on from the queue entry
itself (`GetAutoApplyQueueEntryByID`, no owner filter) rather than from `(id, user_id)`
ownership — see below for what replaces that check.

**Alternatives considered:**
- *A per-user credential, hashed like `api_keys`.* Abandoned for the unrecoverable-secret
  reason above — discovered mid-implementation of exactly this table and its resolver.
- *A per-user credential, stored reversibly (plaintext like `internal/ai/llmkey`, or
  encrypted via `internal/platform/tokencrypt`).* Solves the recoverability problem but adds
  a table, a mint-lazily resolver, and (for the encrypted variant) a new encryption key —
  real machinery for a caller that is one trusted process, not N independent identities.
  Rejected in favor of the shared secret once the ownership check was re-examined (see
  below): per-user scoping was never load-bearing for THIS caller, only for a human's own
  cookie or API key.
- *A real, visible `api_keys` row per candidate.* Rejected: it would appear in that
  candidate's own `/me/api-keys` listing and be self-service deletable, so a candidate could
  silently break their own auto-apply with no signal why.

**Losing the ownership check for this caller is compensated by resolving the entry's owner
from the record itself, plus a rate limit on the whole shared secret.** The static secret
proves "this is the trusted orchestrator process," not "this is candidate X" — so the
routes cannot reuse `GetAutoApplyQueueEntryForReview`'s `(id, user_id)` predicate for this
caller; instead they read the entry by id alone and act as ITS OWN owner (a caller who
knows the secret can act on any queue entry, but only ever as that entry's real owner —
never as an owner it names itself, so it cannot reach an entry it should not know about
unless it already knows that entry's id). The compensating control is
`internal/api/ratelimit`, keyed on the fixed process identity (not per-user or per-IP,
since there IS no per-user identity for this caller): a shared secret leak or a bug that
loops these two calls is bounded to one budget for the whole deployment, the same shape
`contributionLimiter` already gives per-user routes.

**The two auto-apply routes need their own auth gate that accepts this secret —
discovered mid-implementation, not in the original plan.** `mw.key`
(`auth.RequireAuthOrKey`) only ever resolves a session cookie or a live `api_keys` row, so
without a fallback the orchestrator's own secret would be refused by the very endpoints it
exists to call, and task 6.5's manual end-to-end check would fail on step one. A small
route-local gate (`internal/api/handler`, not the `auth` package — this is not a user
identity, so it does not belong beside `RequireAuthOrKey`) checks the presented Bearer
token against the configured secret in constant time before falling through to the normal
`mw.key` resolution for a human caller; a match marks the request as the system caller
(a request-scoped flag, not a `LocalsUserID`, since the entry read that follows is what
supplies the user id) and the review/tailor handlers branch on it to choose
`GetAutoApplyQueueEntryByID` over `GetAutoApplyQueueEntryForReview`. It replaces `mw.key`
only on `POST /me/auto-apply/:queueId/{tailor,review}` — every other route stays on the
unwidened gate.

**The orchestrator calls the existing HTTP endpoints — it does not import `internal/candidate/
cv` or `internal/ai/assistant` directly.** This is a caller, exactly the role
`auto-apply-tailored-resume`'s own design.md already wrote for "the external pipeline";
reaching into those packages directly would duplicate the plan-metering, session-bootstrap,
and turn-claim logic those handlers already own, and would let this change's own tests drift
from what the endpoints actually guarantee. The cost is one extra HTTP hop per step, which
is immaterial next to a 55-second LLM call.

**A new long-lived worker, `cmd/auto-apply-orchestrate`, not a cron job.** Inngest's SDK
serves an HTTP handler the Inngest server calls back into to execute each step — it must
stay reachable between steps, which for a pause spanning days rules out a run-once-and-exit
process entirely. `cmd/mail-ingest` is the one existing precedent for this daemon shape in
this fleet (`Restart=always`, not on a timer).

**Self-hosted Inngest server, its own Postgres DATABASE on the SAME Postgres instance.**
The spike's own finding: `inngest start --postgres-uri` needs no dedicated datastore
cluster. A separate database (not schema) on the instance freehire's own Postgres already
runs keeps Inngest's own migrations and tables from ever colliding with freehire's, while
adding no new datastore PROCESS to run or monitor.

**Both the orchestrator's own calls into `hire` and the event publish
(`PostAutoApplyReview` → `auto-apply/review.decided`) use a plain `http.Client` with a
bounded timeout — NOT `internal/platform/safehttp`.** That package's guard specifically
BLOCKS loopback/private-network addresses (it exists for outbound fetches of
attacker-influenced URLs — Telegram posts, ATS board links); every one of its current
callers targets a genuinely external, untrusted host. Both calls this change makes target
addresses that are internal by design (`hire`'s own API, the self-hosted Inngest event API
at `INNGEST_EVENT_API_URL`) — `safehttp` would refuse to connect to either. Discovered
mid-implementation: the original plan named `safehttp` by analogy ("every other
server-to-server call") without checking what its guard actually targets.

## Risks / Trade-offs

- **A new always-on process is new operational surface** (a systemd unit that must stay up,
  not merely run on a schedule) → mitigated by following `cmd/mail-ingest`'s already-
  established pattern rather than inventing a new one.
- **Self-hosted Inngest is new infrastructure with its own failure modes** (its own event
  loss, its own upgrade path) → mitigated by keeping it to one process against freehire's
  existing Postgres, no dedicated cluster, and by every run's own worst case (tailor fails,
  or the wait times out) already degrading to "the entry sits unreviewed," the same
  human-notices-eventually outcome an unresolved queue entry already has today.
- **The shared secret authenticates a caller without per-user scoping** (unlike every other
  credential on these two routes, it can act as any entry's owner once it knows that
  entry's id) → mitigated by never accepting a caller-supplied user id (the owner always
  comes from the entry itself, never from the request) and by the process-wide rate limit
  bounding how much a leaked secret or a runaway loop can do regardless of which entries it
  targets.

## Migration Plan

Purely additive: a new binary, a new systemd unit, a new self-hosted Inngest instance and
its own database, one new config secret, one new read query
(`GetAutoApplyQueueEntryByID`). No schema migration, no existing table, endpoint contract,
or response shape changes. Deploy order: stand up the self-hosted Inngest server and its
database → deploy `cmd/auto-apply-orchestrate` pointed at it, with
`AUTO_APPLY_ORCHESTRATOR_SECRET` set identically on it and on `cmd/server` → verify with one
manually-published `auto-apply/submit` event end to end (mirroring this session's own
spike) before task 2.1's future trigger ever emits one for real. Rollback is stopping the
new systemd unit; nothing else in the fleet depends on it existing.
