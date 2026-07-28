## Context

The mail stack deliberately refuses to let the LLM link mail: only a
deterministic match (`TierThread` / `TierName`) auto-links, and a confident model
pick becomes a suggestion the user confirms (`maillink/decide.go`). That
asymmetry is a prompt-injection defence and must not be relaxed.

It has a precondition nobody built: a cheap confirmation path. Measured on a real
account, the suggestion queue holds 74 messages and no interface addresses it,
while 31 messages carrying progress signals belong to applications that were
never recorded at all. Both are interface gaps rather than matcher failures, and
both depress the apparent quality of `emails.job_id` for anything downstream.

The listing already returns everything needed to display link state
(`link_source`, `linked_slug`, `linked_company`, `suggested_slug`,
`suggested_company`); it just cannot be *queried* by it.

## Goals / Non-Goals

**Goals:**

- Make the confirmation queue and the orphaned-mail queue addressable, so
  labelling mail is a bounded page of work instead of a full-mailbox scan.
- Give mail about an unrecorded application a single-action path to becoming a
  tracked, linked application with an honest date.
- Keep every resulting link's provenance unambiguous.

**Non-Goals:**

- Changing how `mailmatch` or `mailclassify` decide anything. This change adds
  consumers of their output, not new inference.
- The `freehire-cli` commands, which live in the sibling repo.
- Silence/ghosting analysis (parked as `application-ghosting-signal`).

## Decisions

### Link state is a derived filter, not a stored column

`emails` already carries `job_id` and `suggested_job_id`; link state is a
function of the two (`job_id IS NOT NULL` → linked; else `suggested_job_id IS NOT
NULL` → suggested; else unlinked). Filtering derives it in the predicate rather
than materializing a column.

*Alternative considered:* a stored `link_state` column, which would index
cleanly. Rejected — it is a second source of truth for something two existing
columns already determine, and every write path would have to maintain it. The
partial `emails_job_id_idx` plus the per-user `emails_user_received_idx` already
serve the access pattern at a personal mailbox's scale.

The three values partition the mailbox, which is what makes the counts
trustworthy: the spec asserts the three totals sum to the unfiltered total. That
assertion is the regression test against a future fourth state (say, a rejected
suggestion recorded distinctly) silently going missing from every listing.

### The listing query and its count change together

The listing and its pagination total are separate statements over the same
predicate. A filter added to one and not the other yields a page of five rows
reporting a total of eighty. This has to be one task, not two, and the scenario
asserting the total is the guard.

### `applied_at` comes from the email, and the error leans one way

An application created from mail is dated by the mail's `received_at`. The true
apply date is unknowable and strictly earlier — the employer replied to something
that already existed — so the mail's timestamp is an upper bound.

The direction of the error is the point. Dating the application *later* than it
really was makes it look *more recently active* than it was, so any elapsed-time
reading of it under-reports silence. That is the safe direction: a missed ghost
is a non-event, while a fabricated one tells a person they were ignored when they
were not. The same principle governs the parked ghosting change, where a pending
suggestion suppresses a silence claim.

*Alternative considered:* letting the caller pass the real apply date. Rejected
for now — it is a second parameter serving a case (the user remembers the exact
date) that the mail already approximates well, and it can be added later without
changing the recorded shape.

### Creating the application reuses the apply path

`jobtracking.MarkApplied` already takes `LockJobForApply` in a transaction so
concurrent applies cannot double-bump `jobs.applied_count`. The new action takes
the same path with a supplied timestamp rather than writing its own insert.

This is what makes the "late recording counts as an application" requirement true
by construction instead of by duplicated logic: the transition rule
(`applied_at` unset → set increments once) lives in one place. Writing a second
insert would fork that rule, and the fork would be discovered as a wrong public
counter.

### A pending suggestion blocks the create-and-link action

Mail carrying a suggestion has a proposed answer already. Allowing a second path
to overwrite it silently would leave the resulting link's provenance ambiguous —
`link_source` would say `manual` while a suggestion the user never saw was
discarded underneath. Instead the action refuses and names the pending
suggestion, so the caller confirms or rejects it first. Rejecting then makes the
action available, so nothing is a dead end.

## Risks / Trade-offs

- **A late recording bumps a public counter.** `applied_count` is public, and
  backfilling applications increments it for real jobs → this is correct, not a
  side effect: the person did apply. The counter measures applications, not
  applications-recorded-promptly. What would be wrong is dating them "now", which
  the design already refuses.

- **Three-way partition is asserted, not enforced by the type system.** A future
  state (an explicitly rejected suggestion, say) could break the partition
  quietly → the summing scenario fails the moment it does, which is why it is a
  scenario rather than a comment.

- **Link-state filtering has no dedicated index.** At personal-mailbox scale
  (hundreds of rows per user) the existing per-user index is sufficient → if a
  user's mailbox ever reaches a size where it is not, the fix is a partial index
  on `suggested_job_id`, mirroring the existing one on `job_id`. Noted, not
  built.

- **The CLI lands separately.** Until the sibling repo ships its commands, the
  new endpoints have no interactive consumer and the labelling work cannot
  actually be done → sequence the CLI work immediately after, and treat this
  change as unfinished for its stated purpose until then.

## Open Questions

- Should the web inbox surface the `suggested` queue as a visible pile, or does
  the existing per-row confirm control suffice? Out of scope here; the answer
  does not change the API.
