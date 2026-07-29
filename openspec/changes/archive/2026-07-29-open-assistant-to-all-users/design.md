## Context

`requireRollout` sits in front of all five `/api/v1/assistant/*` routes and refuses any
signed-in caller who is neither a moderator nor a beta tester. On production that is 358 of
360 accounts. The gate is the last piece of a policy the rest of the product has already
abandoned: `accountNav.ts` links "Agent" unconditionally, `/tailor` mounts the same chat
with `requireBeta={false}`, and `AssistantChat.svelte`'s `requireBeta` prop defaults to
`false` — so no host ever turned the UI mirror on.

The visible symptom is a triple `403` on one page load. `boot()` catches the refused session
list, keeps going with an empty array, and the emptiness is then read as "this user has no
chats" — which the next branch answers by `POST`ing a new session, earning a second refusal.
A gate the client cannot distinguish from an absence produces exactly this.

The gate's original justification is in the code and has expired: it was affordable to open
the assistant to nobody because inference was ours to pay for. `accountNav.ts` still carries
the counter-claim that the agent "runs on the user's own machine with their own Claude
subscription" — true before #1165 moved the agent in-process, false since.

## Goals / Non-Goals

**Goals:**

- Any signed-in user reaches the assistant, through `/my/assistant` and through the chat
  embedded in `/tailor`, with no membership test beyond authentication.
- The spec stops asserting a gate the code no longer has — `assistant-sessions` is the only
  spec that normatively requires it.
- The client stops carrying a mirror of a gate that does not exist, rather than being taught
  to mirror it correctly.

**Non-Goals:**

- **AI-credit metering for assistant turns.** Owed follow-up, deliberately not bundled: it is
  a design problem of its own (what a turn costs, whether tool calls count, what a `402`
  mid-stream looks like over SSE) and holding this change for it keeps 358 users at a wall.
- **Hardening `boot()`'s failure handling.** Its conflation of "the list was refused" with
  "the list was empty" is the bug that turned one `403` into three, but with the refusal
  gone the conflation has no trigger left in this code path. Fixing it here would be
  speculative work against a hypothetical future gate; leaving it costs nothing today.
  Noted as a seam, not built.
- **The `beta_tester` flag.** It stays on the model and on `/auth/me`. This change retires
  one consumer, not the mechanism.
- **The `cv-builder` spec's own rollout requirement.** It asserts a restriction that
  `/api/v1/me/cvs` does not implement — a pre-existing drift, out of this change's scope.

## Decisions

**Delete the gate rather than make it configurable.** An env-var or feature flag would
preserve the ability to re-close the assistant cheaply. Rejected: a flag is an undecided
policy stored in two places, and the failure mode we just lived through was precisely a
policy half-applied across layers. Re-closing it later is a revert of a small diff.

**Delete the UI mirror rather than repair it.** The alternative was to keep `requireBeta`,
default it to `true`, and align its condition with the backend's (`moderator || beta_tester`
— the current mirror checks only `beta_tester`, so it would have blocked a plain moderator
the API admits). Rejected because it solves the wrong problem: with the gate gone there is
nothing to mirror, and a prop that must be remembered by every host is the defect that
produced this incident. Removing it also removes the `{:else if !allowed}` notice, which has
no reachable state left.

**Keep `mw.key` untouched.** The routes stay authenticated by cookie, session JWT, or
full-scope API key. The spec's carrier requirement is edited only to stop deferring to a
rollout gate that no longer exists; its unauthenticated-caller scenario is unchanged, and
the scenario asserting a Bearer caller outside the rollout is refused is removed with the
requirement it referenced.

**Rewrite the rollout tests in place rather than delete them.** The case asserting a
non-member is refused inverts to assert they are served — that is the change's contract, and
it deserves a test that fails if the gate returns. The Bearer-carrier tests keep asserting
that widening the carrier does not widen authentication.

## Risks / Trade-offs

**Unmetered spend on 358 accounts.** → The real risk of this change, accepted knowingly. The
assistant is a bounded tool-calling loop, so a single turn is bounded, but the number of
turns is not. Mitigation is the follow-up metering task; until it lands, spend is watched
rather than capped, and re-closing the gate is a one-line revert.

**Load on the LLM proxy.** The in-process agent shares the litellm proxy with `enrich`,
whose 502s have historically traced to that proxy restarting or filling its disk. A jump in
assistant traffic lands on the same dependency. → Watch proxy error rate after deploy; the
revert path is the same one-liner.

**The spec's account-nav requirement mentions "beta/moderator gating".** It describes the
rail as listing the same items with the same gating as the sidebar — a statement about
sharing one visibility model, not about the assistant specifically. Left alone deliberately:
it stays true whether or not any item is gated.

## Migration Plan

Ordinary deploy, no migration, no data change. Backend and frontend land together; if they
land apart, either order is safe — an open API with a mirroring client just gates the UI for
non-members (today's behaviour), and an open client against a gated API is today's behaviour
exactly.

Rollback: revert the commit. The gate returns, and the only state created meanwhile is
assistant sessions belonging to newly-admitted users, which remain theirs and reappear
whenever the gate opens again.

## Open Questions

- Does opening the assistant change what the follow-up metering should charge for — a turn,
  a tool call, or tokens? Not blocking, but the answer is cheaper to find while usage from a
  wider audience is being observed.
