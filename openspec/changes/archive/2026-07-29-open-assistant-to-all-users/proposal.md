## Why

The assistant refuses 358 of the 360 accounts on production, but the product no longer
behaves as if it were restricted: the account nav links "Agent" for everyone, and the
tailoring workspace mounts the same chat with its UI gate explicitly switched off. What a
non-member actually meets is not a closed door but a wall of `403`s — opening
`/my/assistant` fires three refused requests in one page load, because the client reads the
refusal of the session list as "you have no chats yet" and answers it by trying to create
one.

The gate's stated reason has also expired. `requireRollout` was written when inference was
billed to us and the assistant was free; the nav comment beside it still claims the agent
"runs on the user's own machine with their own Claude subscription", which stopped being
true when #1165 moved the agent in-process. The gate now protects a decision nobody has
re-made, and half the product already routes around it.

## What Changes

- **BREAKING** (for the spec, not for any caller): every `/api/v1/assistant/*` route drops
  `requireRollout`. Authentication is unchanged — `mw.key` still resolves a cookie, a
  session JWT, or a full-scope API key, and an unauthenticated caller is still refused.
  What disappears is the second check that turned a signed-in non-member into a `403`.
- The UI stops mirroring a gate that no longer exists: the `requireBeta` prop and the
  `allowed` derivation come out of `AssistantChat.svelte`, along with the restricted-rollout
  notice they guarded, and `/tailor` stops passing `requireBeta={false}` to switch off a
  gate that is gone.
- The stale justification in `accountNav.ts` is replaced with what is actually true: the
  agent runs in our backend, and the spend it incurs is ours.
- The rollout tests in `assistant_integration_test.go` are rewritten: the case that asserted
  a non-member is refused now asserts they are served, and the Bearer-carrier case keeps
  asserting that widening the carrier does not widen *authentication*.

Deliberately unchanged: the `beta_tester` flag itself. It stays on the user model and on
`/auth/me` — this change retires one consumer of it, not the mechanism.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `assistant-sessions`: the requirement "The assistant is gated to the beta-tester group" is
  removed, and the session-carrier requirement stops deferring to a rollout gate — its
  scenario asserting that a Bearer caller outside the rollout is refused goes with it.

## Impact

- `internal/handler/assistant.go` — `requireRollout` and its wiring in `register`.
- `internal/handler/assistant_integration_test.go` — the rollout cases.
- `internal/handler/AGENTS.md`, root `AGENTS.md` — both describe the assistant as gated.
- `web/src/lib/assistant/AssistantChat.svelte`, `web/src/routes/tailor/[slug]/+page.svelte`,
  `web/src/lib/accountNav.ts`.
- No migration, no API-shape change, no client contract change. A caller that used to get
  `403` now gets the same body every member already got.

**Out of scope, and load-bearing:** the assistant charges no AI credits. Metering lives in
`cv_tailor.go`, `cv.go`, `match_analysis_stream.go`, `contributions.go` and `intake.go`, but
`internal/assistant/` has no notion of credits at all — the rollout gate was the only thing
bounding that spend. Opening the assistant hands an unmetered tool-calling loop to every
signed-in user; metering it is a required follow-up, tracked separately, and this proposal
is written on the explicit understanding that the follow-up is owed.
