## Why

The facts a job search is made of are currently stored as mutable columns, so the
system forgets them. `user_jobs.stage` is overwritten, so the date a transition
happened never existed. `user_jobs.followed_up_at` holds one timestamp, so a second
chase erases the first. And the company response rate we already serve publicly is
recomputed from live mail (`RebuildInsightsCompanyResponse` counts `emails` rows
`WHERE deleted_at IS NULL`), which means a candidate tidying their inbox silently
makes a company look more silent than it was.

The last of these is the urgent one: it is a public claim about a named company whose
value moves for reasons that have nothing to do with the company.

## What Changes

- New `application_events` table: an append-only, content-free log of what happened to
  an application — `applied`, `employer_reply`, `follow_up_sent`, `stage_set` — carrying
  the event's own timestamp, its source, and the company it belongs to.
- Emission from the paths that already make the decision: `internal/maillink` (the
  classification worker), `internal/inbox` (suggestion confirmation, manual link,
  application-from-mail, external triage), `jobtracking.MarkApplied`, `jobtracking.TrackJob`,
  and the follow-up record action.
- Retraction: correcting a wrong email→application link retracts the event it produced.
  Deleting the mail does not — deletion hides content, it does not un-happen the reply.
- `RebuildInsightsCompanyResponse` reads the ledger instead of the `emails` table, so the
  served rate stops depending on inbox hygiene.
- The company payload gains a median time to first reply, under the same sample gate as
  the rate it sits beside.
- `cmd/backfill-application-events` replays the events that have a real date:
  `emails.received_at`, `user_jobs.applied_at`, `user_jobs.followed_up_at`. Stage history
  is **not** backfilled — the current `stage` column carries no date, and inventing one
  would fabricate an observation.

Not in this change: the personal funnel benchmark. It reads stage-to-stage velocity, which
this change deliberately starts empty, so it can only be built once events accrue.

## Capabilities

### New Capabilities
- `application-event-ledger`: the append-only event log — its vocabulary, the paths that
  emit into it, idempotency, retraction, and the backfill's honesty boundary.

### Modified Capabilities
- `company-hiring-signal`: the per-company response rate derives from the ledger rather
  than from live mail rows, and the payload gains a median time to first reply under the
  same sample gate.

## Impact

- **Schema**: new `application_events` table; additive, no change to existing tables.
- **Go**: new `internal/appevent` (vocabulary + emission); write calls in `internal/maillink`,
  `internal/inbox`, `internal/jobtracking`; new `cmd/backfill-application-events`.
- **SQL**: `RebuildInsightsCompanyResponse` rewritten against the ledger;
  `insights_company_response` gains a reply-time column.
- **API**: the company payload's response object gains one field; nothing is removed.
- **Deploy**: migration applies before the reading code ships; the backfill runs after, as
  its own pass.
