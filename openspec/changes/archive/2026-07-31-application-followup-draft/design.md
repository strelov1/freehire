## Context

The silence signal is finished work: `internal/userjob/silence.go` holds a stage-aware threshold
ladder whose numbers carry their own provenance, `ListTrackedJobs` derives
`last_activity_at = GREATEST(applied_at, newest linked inbound mail)`, and `BoardCard.svelte` already
paints "No reply for N days". `internal/ghost/evidence.go` consumes the same verdict.

What is absent is anything to do about it. This change adds the action and touches none of the
measurement.

Two facts shape the design more than any preference:

- **`last_activity_at` measures the other side.** It is the later of the apply date and the newest
  *inbound* message. A follow-up the candidate sends is not inbound, so feeding it into that
  derivation would make the board report a reply that never came.
- **The commonest silent application has no recipient.** An address exists only via
  `emails.from_addr` on linked mail. The `applied` stage — 21 days, 16% of the observed sample — is
  precisely the case where nobody ever wrote back, so there is nothing to prefill.

## Goals / Non-Goals

**Goals:**

- Turn a silence badge into a next step, at the place the badge already is.
- Assemble the draft deterministically: no credits, no metering, no fabricated claims.
- Record that a chase happened without corrupting what silence means.

**Non-Goals:**

- Sending. `emailnotify.Client.Send` exists and the hosted mailbox would give a plausible From, but
  putting our infrastructure between a candidate and a recruiter is an identity and deliverability
  decision with its own failure modes — and it could only serve the minority of silent applications
  where an address is known.
- An LLM draft. A template plus one line of the candidate's own evidence cannot invent a claim and
  costs nothing. If real use shows the drafts read as boilerplate, an LLM pass has a baseline to beat.
- A digest across applications. Same data, different surface; this change stays next to the badge.

## Decisions

### The assembler is pure, and lives in its own package

A new `internal/followup` with a single entry point taking a small input struct (role, company, days
silent, stage, optional strength, optional recipient) and returning subject + body. Pure and I/O-free,
in the shape `internal/atscheck` and `internal/verdict` already use, so the wording is table-tested
rather than eyeballed.

**Alternative rejected:** assembling inside the handler. The text is the part worth testing hardest —
tone, the elapsed-time phrasing, what happens when a field is missing — and a handler drags a DB and
an HTTP context into every one of those cases.

### The one non-generic line comes from the cached fit analysis

`matchanalysis.Analysis.Strengths` already names why this candidate fits this vacancy, derived from
their own CV and sanitized on the way in. The draft restates the first one.

**Alternative rejected:** deriving it from `RequirementMatch` by picking the strongest
`evidence_strength`. More precise in theory, but a requirement is the *vacancy's* phrasing ("5+ years
Go"), while a strength is already written as a claim about the candidate — which is what a chase
needs. **Alternative rejected:** letting the candidate type it. That turns a one-click action into a
writing task, which is the thing they were already avoiding.

**Absence is honoured, not filled.** No cached analysis, or none with strengths, means the line is
omitted. The provenance rule this project runs on — a claim about the candidate must come from the
candidate — is enforced here by having no other source available.

### `followed_up_at` is recorded, and deliberately not wired into the clock

A new `user_jobs.followed_up_at`. It is written by an explicit record action and read by the board;
it is NOT added to the `GREATEST(...)` that derives `last_activity_at`, and the ghost signal that
consumes silence therefore sees no change.

The card gains a third reading — silent *and chased* — instead of flipping back to green. Resetting
would have been the cheaper code and the wrong meaning: the employer still has not replied, and a
candidate deciding whether to write off an application needs to know they already tried once.

### Both endpoints sit on the tracking surface and admit an API key

`GET /me/tracking/:slug/followup` assembles the draft; `POST` records the chase. The neighbouring
tracking reads and the apply-tracking write already admit a key (the CLI records applies), so a key
here is consistent — a follow-up is the same kind of act as marking an application sent.

The draft endpoint refuses anything that is not `silent`, reusing `userjob.SilenceStateFor` rather
than re-deriving the rule, so the offer and the badge can never disagree about which applications
qualify.

## Risks / Trade-offs

- **A template reads as boilerplate** → Mitigated by the strength line and a concrete ask, and
  bounded by being free: if adoption shows it is ignored, the LLM upgrade starts from a measurable
  baseline rather than a guess.
- **The candidate sends the draft and we never learn** → We record only what they tell us via the
  record action. A follow-up sent without pressing it leaves the card saying "not chased". Accepted:
  the alternative is sending on their behalf, which is the non-goal above.
- **`followed_up_at` invites a future reader to add it to the silence clock** → The requirement
  states it must not, the derivation query carries a comment, and a test asserts the days-silent is
  unchanged by a recorded follow-up.
- **A stage that never accrues silence could still be chased** → The draft endpoint gates on the
  silence state, so a terminal or active application is refused rather than drafting something the
  board would not have suggested.

## Migration Plan

`0059_user_jobs_followed_up_at.sql` adds one nullable timestamptz. No backfill: a null means "never
chased", which is true of every existing row. `make sqlc` after the query changes; `release.sh`
applies the migration before the colour flips. Rollback is a revert — the column is inert to code
that does not read it.
