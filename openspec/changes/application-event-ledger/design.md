## Context

Three of the facts a job search produces are stored as mutable columns and are therefore
forgotten as they are overwritten:

- `user_jobs.stage` — the transition date never existed.
- `user_jobs.followed_up_at` — one timestamp, so a second chase erases the first, even though
  `internal/userjob/AGENTS.md` treats the second chase as a deliberate decision of the candidate.
- The link between a reply and an application — `emails.job_id` moves when a suggestion is
  confirmed, corrected, or undone.

On top of these sits a public number. `RebuildInsightsCompanyResponse`
(`internal/db/queries/insights.sql:374`) counts an application as answered via
`EXISTS (SELECT 1 FROM emails … AND e.deleted_at IS NULL)`, and `companyResponseRate`
(`internal/handler/company_response.go:38`) serves the ratio above a ten-application gate. The
denominator is already guarded — only applicants with a connected mailbox count, so our blind
spot is never reported as an employer's silence — but the numerator is not: a candidate deleting
old mail makes a named company look more silent on a public page.

The observable substrate is good. `emails` carries `received_at`, a classified
`status_signal` from the `mailclassify` vocabulary, and a link with provenance
(`link_source`, `match_confidence`). What is missing is a record that stops changing once
written.

## Goals / Non-Goals

**Goals:**

- Record what happened to an application once, with the date it happened, in a form that later
  edits to mail cannot rewrite.
- Make the served company response rate stable under inbox hygiene.
- Add time-to-first-reply per company, with censoring stated rather than hidden.
- Preserve the follow-up history that a single column currently destroys.
- Begin accumulating stage-transition history, so a funnel benchmark becomes possible later.

**Non-Goals:**

- The personal funnel benchmark. It needs stage-to-stage velocity, which this change starts
  empty by design; building it now would ship an empty screen that only calendar time can fill.
- Any public surface for stage timings or personal funnel data. This change touches one existing
  public field (the company rate) and adds one beside it.
- Trusting manually-recorded stages in day arithmetic. `stage_set` events are written from day
  one and read by nothing in this change.

## Decisions

### A separate ledger rather than deriving from `emails`

Deriving every aggregate from the mail table was the smaller change and was rejected. Under it
the fact is "a message exists and is linked" — mutable, deletable, and re-pointable — so a
rebuild run twice over the same history can answer differently. Under the ledger the fact is
"a reply arrived on 12 July", written once.

The honest cost: on the day it ships, the ledger's contents are entirely a projection of
`emails` and `user_jobs`, and add no information. The argument for building it now is that the
information it protects is being lost continuously and cannot be recovered later — a table can
be added at any time, last month's transitions cannot.

Alternative considered: a SQL view named `application_events` over `emails`, giving readers one
interface without a physical table. Rejected — it abstracts nothing today, and it cannot hold
the stage transitions and repeated follow-ups that are the reason for the change.

### Mail is the only source trusted for day arithmetic

Only mail-derived events carry an objective timestamp. A manually-set stage records when the
candidate got around to updating their board, so a funnel built on it would measure diligence
and report it as market behaviour. `stage_set` events are recorded but excluded from timings
until there is enough of a sample to say what they are worth. This mirrors career-ops'
`DAY_MATH_SOURCES` split, which counts reconstructed observations but keeps them out of medians.

### Retraction is separate from deletion

Two user actions look alike and mean opposite things. Deleting a message hides content and
leaves the event standing. Re-linking a message asserts the fact belongs to another employer,
and must move the event, because a wrong link left standing poisons a named company's public
rate permanently — the failure mode the mail stack already met when a catalogue company sharing
an ATS brand name auto-collected twenty-three acknowledgements belonging to other employers.

Retraction is a `retracted_at` stamp, not a delete: an event recorded in error is itself a fact,
and the ledger stays append-only.

### Idempotency by constraint rather than by coordination

A partial unique index on `(user_id, kind, source_ref) WHERE source_ref IS NOT NULL` makes
emission and backfill the same operation. The worker and the backfill can meet on the same email
in any order and produce one row. The alternative was a flock between them — the mechanism that
has already deadlocked `reindex` against `reindex-companies` twice.

Manual events carry no `source_ref` and sit outside the index, which is what makes two
consecutive follow-ups two rows.

### The company slug is denormalized onto the event

`cmd/prune` is the only hard-delete path for jobs, and `job_id` clears when it runs. An event
that lost its company would drop out of the company aggregate retroactively — the exact
instability this change exists to remove. The slug recorded at write time keeps the row
self-contained, and the aggregate no longer needs to join `jobs`.

### `occurred_at` and `recorded_at` are separate columns

Connecting a mailbox imports historical ATS mail in one pass. Keyed on write time, the ledger
would report a year of employer replies arriving on the day of connection. Day arithmetic uses
`occurred_at`; `recorded_at` remains available to tell a late arrival from a late event.

### Emission lives in the service layer

`internal/appevent` holds the vocabulary and the write, in the shape `internal/userjob` already
uses for the stage vocabulary. Callers are `internal/maillink`, `internal/inbox`, and
`internal/jobtracking`. The mail stack pins this rule because the in-app assistant calls
`internal/inbox` directly and issues no HTTP request; a rule placed in a Fiber handler is one the
in-process agent never meets, which is how the CV-tailoring contact guard was once lost.

### A linked message is a reply, classified or not

The first draft of this design recorded an event only for mail that was both linked and
classified. An existing rollup test rejected it, and was right to: `external` mail is never
classified server-side — that is what makes the bring-your-own-harness tier free — so every
such user's replies would have gone unrecorded and their employers would have read as more
silent than they were. The signal is detail about the reply; the link is the evidence one
arrived.

### Time to first reply reports its censoring

The median covers answered applications only, and is served with the count of applications still
unanswered. A median over survivors alone, presented bare, tells a candidate that employers
answer in six days while most applications in the sample were never answered at all.

## Risks / Trade-offs

- **The ledger and the mail table disagree** → The ledger is authoritative for aggregates, and
  nothing reads both for the same question. `RebuildInsightsCompanyResponse` moves wholesale
  rather than being taught to consult both.
- **An event outliving its message reads as circumventing deletion** → Events hold no content,
  serve only aggregates, and die with the account through the `users` cascade. The retention
  boundary needs stating in the privacy copy before this ships publicly.
- **The backfill runs long on a large `emails` table** → Keyset pass in the manner of
  `cmd/backfill-derive`, resumable, and idempotent by constraint, so an interrupted run is
  restarted rather than repaired. Prod's I/O headroom is already thin
  (`host2` runs Postgres on stock defaults), so the backfill runs outside the 03–07 UTC window
  where DDL and the nightly dump contend.
- **The migration adds a column to `insights_company_response` while a long read holds a lock**
  → Additive DDL on a rollup table, applied ahead of the reading code, per the standing
  migration discipline.
- **`stage_set` events accumulate with no reader** → Accepted deliberately. They are the input
  the follow-up change needs, and they cost one row per stage change.
- **A candidate's own reply in an ATS thread is recorded as an employer reply** → The Gmail query
  (`internal/gmailsync/senders.go:93`) is an OR over ATS senders and phrases with no `-in:sent`
  clause, so sent mail can enter the table. This predates the change and already affects the
  served rate; it is out of scope here and worth its own investigation.

## Migration Plan

1. Apply the additive migration: `application_events` plus the reply-time column on
   `insights_company_response`. Nothing reads them yet.
2. Ship the emission paths. The ledger begins accumulating live events.
3. Run `cmd/backfill-application-events`, outside the nightly-dump window.
4. Ship the rewritten `RebuildInsightsCompanyResponse` and the payload field.

Rollback: the served payload field is additive and its absence is already a supported state, so
reverting step 4 restores the previous behaviour without touching data. The ledger keeps
accumulating harmlessly if the change is abandoned after step 2.

## Open Questions

- The ten-application gate is inherited from the response rate. Whether the median needs its own,
  higher gate — a median over ten points is thin — is deferred until the sample exists, and the
  code should make the two gates separately adjustable rather than share one constant.
