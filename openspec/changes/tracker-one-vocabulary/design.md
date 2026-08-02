## Context

The tracker stores one state per application (`applications.stage`, 8 values) and renders it
through three unrelated vocabularies defined in five places: the stages themselves
(`internal/userjob/stages.go`), the board's four columns (`web/src/lib/board.ts`), the seven
pipeline buckets (`internal/userjob/buckets.go` + `web/src/lib/pipeline.ts`), a fourth copy of
the bucket vocabulary inside `HomeFunnel.svelte`, and the labels (`web/src/lib/stages.ts`).

`internal/userjob` is already the natural owner. It holds three tables keyed on the stage
vocabulary — `activeRank` and `terminalStages` (moved there by the S16 finding of the
2026-08-01 architecture review, precisely because a tracking rule was living in the mail
package) and `silenceThresholds`. `TestEveryStageIsRankedOrTerminal` already binds two of them
to the vocabulary. This change adds the third table in the same shape.

The mail side is a separate vocabulary by design: `internal/appevent` documents why the event
kinds and the classifier's signals are kept apart, and `mailclassify` maps signals to stages
through a private table. That mapping is correct and stays; only its visibility changes.

## Goals / Non-Goals

**Goals:**

- One definition of the stage labels and the stage→group membership, in Go, generated into the
  frontend contracts.
- `/me/tracking/pipeline` returns per-stage counts; the bucket vocabulary ceases to exist.
- The mail signal→stage relationship is visible where mail is read, and a disagreement between
  a classified message and the stage is offered for one-click resolution.

**Non-Goals:**

- Changing the stage vocabulary or migrating data. The 8 values stay.
- Changing when mail advances a stage. `AdvanceStage`, `Forward`, `IsTerminal` and the
  confidence thresholds are untouched.
- Tracker load performance — a separate change, gated on a measurement that has not been taken.
- The drawer's `Viewed → Saved → Applied` strip, which renders an engagement funnel in a shape
  that reads as a timeline (newest on the left). Real chronology already exists on the Calendar
  tab. Recorded as known debt.

## Decisions

### Labels and groups live in Go, not in TypeScript

The vocabulary has a second reader that is not a browser: the in-app assistant calls
`internal/jobtracking` directly with the session owner's id and never passes through Fiber.
This is the rule `internal/inbox` states for mail — a rule enforced in a handler is a rule the
in-process reader never meets. A label defined in `stages.ts` does not exist for that reader.

*Alternative rejected:* keep labels in TypeScript and have the server return raw stages only.
It reads simpler, and it puts the product's words where half its readers cannot see them.

### The endpoint returns stages, not groups

Grouping is static. Returning both the counts and their grouping would place the mapping in the
response and in the generated contracts — two places again, differing only in latency. The
endpoint returns the raw per-stage counts; the grouping arrives once, generated.

*Alternative rejected:* return `groups` alongside `stages` for the convenience of external
callers. The grep found no external caller, and the convenience costs the invariant.

### `buckets` is removed rather than deprecated

A deprecated field is the third vocabulary surviving in the code, the docs and the OpenSpec
requirement — exactly the thing this change exists to delete. No consumer was found outside
this repository (CLI, MCP skills, ChatGPT Actions surface all checked).

### The suggestion is silenced by the ledger, not by a new flag

`application_events` already records every `stage_set` with its timestamp. A `stage_set` newer
than the message that prompted an offer means the candidate has answered the question,
whichever stage they chose. A `dismissed` column would be a second store of the same fact, and
the two would eventually disagree.

*Alternative rejected:* a dismissal flag on the email row. It needs a migration, a write path,
and a rule for what a *later* disagreeing message should do to it.

### No confidence threshold on the suggestion

`emails.match_confidence` is the matcher's confidence in the **link**, not the classifier's
confidence in the **signal** — `migrations/0020` says so, and `mail_classification.sql` keeps
it pinned to the link. The classification confidence exists only in memory while
`cmd/classify-mail` runs and is never persisted, so a `>= 0.8` rule would require a new column
written by the classifier.

That column is not worth adding blind. The signal is already trusted enough to be rendered as a
label on the message, and every suggestion is confirmed by a human before it changes anything.
If suggestions prove noisy, persisting the confidence is a separable change with a measurement
behind it.

## Risks / Trade-offs

- **Breaking `/me/tracking/pipeline` for an unknown external caller** → The grep covers this
  repository only. The endpoint is documented, so an unlisted integration is possible; the
  change is announced in the changelog and the shape is spec'd, not silently altered.
- **Two levels of concept survive** (stage and group) → Deliberate: collapsing to five stages
  would break the stored vocabulary and the public contract. The mitigation is that the coarse
  and the precise names are always shown together — a `Closed` column whose cards each read
  `Rejected`.
- **The suggestion may be noisy without a confidence gate** → It is confirmed by a human, is
  limited to the newest message, and is silenced permanently by any stage change afterwards. If
  measurement shows noise, the gate is a small follow-up.
- **A generated group table that drifts from the stage values** → Caught twice: a Go test in
  both directions, and a `satisfies`-style type check in the required `pnpm run check` gate.
- **`HomeFunnel.svelte` renders hardcoded marketing data** → It keeps its own numbers; only its
  vocabulary becomes generated, so the demo cannot name a bucket the product no longer has.

## Migration Plan

No schema migration — no column is added, altered or dropped. The only new SQL is a read
(`LastStageSetAt`) over the existing ledger.

Deploy order is unconstrained: the SPA is served from the same build as the API, so the
contract change and its only consumer ship together. Rollback is a redeploy of the previous
build.

## Open Questions

None blocking. Deferred by decision, not by uncertainty: whether to persist the classifier's
confidence (see above), and the drawer's activity strip (non-goal).
