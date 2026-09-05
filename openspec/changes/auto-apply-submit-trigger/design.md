## Context

See proposal.md for motivation. Relevant existing pieces this design builds directly on
top of, unchanged:

- `auto_apply_queue` (migration 0116): `UNIQUE (user_id, job_id)` already exists — the
  dedup key this change relies on rather than adding its own.
- `POST /me/auto-apply/:queueId/{tailor,review}` (`auto-apply-tailored-resume`) and
  `cmd/auto-apply-orchestrate` (`auto-apply-inngest-orchestration`, this session) already
  carry a queue entry from tailoring through review once `auto-apply/submit` is published —
  this change's only job is to publish that event for the first time from something a
  candidate can actually reach.
- `internal/api/handler/auto_apply_review_publish.go`'s `autoApplyEventPublisher`
  interface and its real implementation, `inngestEventPublisher` — already publishes
  `auto-apply/review.decided` over a plain `http.Client` (not `safehttp` — see that
  change's own design.md for why) to the self-hosted Inngest event API.
  `PostAutoApplyReview`'s own event-publish call is the pattern this change's new
  publish call mirrors: best-effort, logged on failure, never returned to the caller.
- `plan.TierOf(proUntil, now) Tier` — the one-column PRO/free read this change uses
  directly; no new plan-limit machinery.
- `cv.Store.BaseCV(ctx, userID) (Record, bool, error)` — the existing "does this candidate
  have a base CV" read, reused as-is for the preflight check.
- `jobview.Job` already carries one caller-scoped field (`MyVote`, populated only for an
  authenticated caller) — the precedent this change's own new field follows.

## Goals / Non-Goals

**Goals:**
- Let an eligible candidate start exactly one auto-apply attempt per job from the job page.
- Reuse the existing queue table, its unique constraint, and the existing event-publish
  shape — no new schema, no new publish mechanism.

**Non-Goals:**
- Any change to `cmd/auto-apply-orchestrate`, the tailor/review endpoints, or
  `internal/application/autoapply` — all already handle everything from a fresh queue
  entry onward.
- Widening past Greenhouse. `auto-apply-tailored-resume` already scoped the file-upload
  resolution step to Greenhouse only; enqueuing for a source that step can never resolve
  would create attempts that can tailor and get reviewed but can never actually submit.
- An "undo"/cancel action once queued, or a way back in after a decline. Matches
  `auto-apply-tailored-resume`'s own "a decline is terminal, `Park` has no automatic path
  back" trade-off, restated here because this change is the thing that could have added a
  retry path and deliberately does not.
- A UI (or requirement) for a candidate to disable/withdraw PRO eligibility — reads
  `users.pro_until` as it already exists; no billing-flow changes.

## Decisions

**`POST /api/v1/jobs/:slug/auto-apply`, addressed by the job's public slug, not by a
queue id.** Unlike the tailor/review endpoints (which already have a queue entry to name),
this is the call that creates one — mirrors the existing sibling route shape under
`/jobs/:slug/...` (e.g. the apply-form read), and slug is what the job page already has in
hand.

**`mw.cookie`, not `mw.key`.** `auto-apply-tailored-resume`'s own design.md drew this exact
line for `/autopilot`: "an unattended run rewrites a CV, and the browser is the only place
the candidate can watch it happen and undo it." This endpoint is the same shape at even
higher stakes — a fresh entry does not just rewrite a CV unattended, it starts a sequence
that can end in a REAL submitted job application. A leaked API key must not be able to
start that chain.

**PRO-only is a plain tier check, not a new metered feature in `internal/ai/plan`, and is
named here as a deliberate exception.** This repository's own stated convention (see
`AGENTS.md`'s Conventions) is that a plan differs in how MUCH of a feature it allows per
day, never in WHETHER the feature exists at all — every existing plan-gated surface
(tailor, match) is a daily allowance, not an on/off switch. Auto-apply's whole value is
being unattended: a free-tier daily allowance of "a small number of auto-apply starts"
would still let a candidate queue attempts, just slowly, which does not match the intent
behind gating it at all. Presented as one of three options (PRO-only / metered daily
allowance / open to everyone) during this change's own brainstorming and picked
explicitly by the user as a deliberate, acknowledged exception rather than a precedent for
future features — a plain `plan.TierOf(user.ProUntil, now) == plan.TierPro` check, refused
as 402 the same shape `refuseNewTailoring` already answers with, before any queue row is
touched. Alternative considered and rejected: a new metered `auto-apply-enqueue` feature in
`internal/ai/plan` with a tiny free-tier daily allowance — matches the existing
convention's letter, not its spirit here, and adds a whole allowance/shadow-mode surface
for a gate that is meant to be binary.

**Enqueue itself is a plain INSERT, not routed through
`internal/application/autoapply.Store`.** That package's own AGENTS.md is explicit that
what populates `auto_apply_queue` is out of its scope — it only ever claims what is
already there. Adding a write path into it here would be the exact layering violation its
own docs warn against; the new sqlc query lives directly in
`internal/platform/db/queries/auto_apply_queue.sql` beside the existing ones, and the
handler calls it directly, matching how `PostAutoApplyTailor`/`PostAutoApplyReview`
already call `db.Queries` methods without an intermediating package.

**Dedup is `INSERT ... ON CONFLICT (user_id, job_id) DO NOTHING RETURNING id`, with a
second read on a miss** — the same unique-violation-then-refetch shape `cv.Store.Tailor`
already uses for its own per-(user, job) uniqueness, chosen for the same reason: a
`SELECT` then `INSERT` has a race window a concurrent double-click can hit, and the
database's own constraint is the only thing that closes it. `DO NOTHING` rather than
letting a real unique-violation error surface, because "already queued" is an expected,
common outcome here (a page reload, a second tab), not a fault to log.

**The event publish happens ONLY on the row this request itself created** (checked via
the insert's own returned id, not "an entry exists now" more broadly) — mirrors why
`PostAutoApplyReview` only publishes on the write IT performed: publishing on every
request, including an idempotent replay that touched no row, would let the orchestrator's
executor see the same `auto-apply/submit` more than once for one entry. The orchestrator's
own function is idempotent per its trigger event's data by Inngest's own dedup, but relying
on that rather than simply not re-publishing would be depending on a safety net this
change does not need to lean on.

**The job detail response's new field is read via one extra query keyed on
`(caller_id, job_id)`, not folded into the main job query.** Matches how `MyVote` is
already read as its own caller-scoped lookup rather than a join baked into the primary job
query for every anonymous reader too. Returns one of three states (`none`, `queued`,
`declined`) computed from the same three columns (`tailored_cv_id`/`review_decision`
absent-or-set) the tailor/review endpoints already read — no fourth status column, no
enum duplicated between Go and the database.

**Alternatives considered:**
- *A general "enable auto-apply" account-level toggle instead of a per-job button.*
  Rejected: the product direction decided before this change (recorded in
  `auto-apply-tailored-resume`'s own deferred task 2.1) is a per-posting button on the JD
  page, not a standing rule — a candidate chooses per vacancy, not once for everything
  matching some future criteria.
- *Route the enqueue write through `internal/application/autoapply.Store` for
  consistency with how that package reads the same table.* Rejected per the layering
  decision above — that package's own scope explicitly excludes what populates the queue.

## Risks / Trade-offs

- **PRO-only is a feature-existence gate, the one thing this repo's plan-limit convention
  says never to do.** → Mitigated by making the exception explicit and documented here
  rather than precedent-setting silently; presented as one of three explicit options during
  this change's own brainstorming, and the user picked PRO-only deliberately.
- **A double-click or a slow network retry could, in principle, race two inserts.** →
  Mitigated by the database's own `UNIQUE (user_id, job_id)` plus `ON CONFLICT DO NOTHING`:
  the second request's insert affects zero rows and the handler reads the (now-existing)
  row instead, exactly like `cv.Store.Tailor`'s own concurrent-tailor race today.
- **Publishing `auto-apply/submit` is best-effort, same as `PostAutoApplyReview`'s own
  publish; a publish failure leaves a queue entry that nothing will ever pick up.** →
  Same accepted trade-off `auto-apply-inngest-orchestration` already made for the mirror
  case (a failed `review.decided` publish): the entry sits exactly where an unresolved
  queue entry already sits today, for a human to notice — not a new failure mode, the
  existing one applied to one more publish call.

## Migration Plan

Purely additive: no schema migration, one new sqlc query, one new route, one new field on
an existing wire shape, one new frontend button. Deploy order: none in particular — the
new endpoint and the orchestrator it hands off to (already deployed per
`auto-apply-inngest-orchestration`) have no ordering dependency on each other at deploy
time, only at request time (an enqueue before the orchestrator is deployed would publish
an event nothing consumes yet, which is simply a queue entry that waits, the same
degraded-but-safe outcome a publish failure already has). Rollback is reverting the
frontend button and the route; no data to unwind — a queue entry created before rollback
stays exactly as valid as one created any other way.
