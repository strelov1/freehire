## Context

Three signals already exist and none of them talk to each other (`internal/jobreality`,
`internal/userjob`'s silence ladder, `job_reports`). The design question is not "what is a ghost
job" — the literature agrees on the observable symptoms — but **where each kind of evidence
lives**, because the three kinds have genuinely different natures:

| Evidence | Nature | Consequence |
|---|---|---|
| Posting shape | a function of time over already-stored fields | must be computed, never stored — it goes stale sitting still |
| Application outcome | derived state of `user_jobs` + linked mail | must be queried live, never copied — a copy desynchronises when mail arrives |
| A person's statement | an event | must be stored — nothing else records it |

`jobreality` solved the first case by computing at index time and storing nothing. That solution
does not generalise to the other two, which is the core of what follows.

## Goals / Non-Goals

**Goals.** Converge the three channels into one hedged, evidence-backed verdict. Make the
anonymity and convergence guarantees structural rather than conventional. Ship a feature that is
silent until a calibration gate is deliberately opened.

**Non-Goals.** Asserting employer intent. Filtering or ranking on the verdict. Achieving coverage
at launch — the outcome tier is expected to be nearly empty, and saying so is part of the design.

## Decisions

### Read-time, not index-time

`cmd/reindex` is `content_hash`-incremental even at `scope=full`: it scans every row but pushes
only documents whose hash moved, and there is no `--force`. A field absent from `jobhash.Of()`
therefore never reaches the index on its own — this is the documented `is_tech` trap.

Ghost evidence moves with no ingest whatsoever: a reply arrives, a 22nd day of silence elapses,
someone files a report. `content_hash` does not budge, so a Meilisearch facet would need its own
per-change document-delivery path, plus the hard-500 window that adding a filterable attribute
opens until a ~26-minute rebuild swaps in.

**Decision:** v1 computes the verdict in `jobview` on read. The evidence tables are hundreds of
rows; a bulk lookup for a 20-card page is trivial, in the shape `RoleClusterCountsAll` already
uses (a sparse map keyed by job id, holding only jobs with evidence).

**Rejected:** a stored `jobs.ghost_level` column written by a nightly worker. It is the exact trap
`jobreality` was designed to avoid — a time-dependent value in a stored column lies between runs,
and a threshold change costs a full pass over 3.5M rows instead of one recomputation.

**Seam:** promoting the verdict to a facet later is an index-settings change plus a delivery
mechanism, not a redesign. The criterion codes and level vocabulary are chosen to be facet-ready.

### Outcome counts are queried, reports are stored

Application silence is already derivable from `user_jobs` + linked mail. Emitting it as an event
row would create a second copy that must be invalidated when mail later arrives — a
synchronisation bug waiting to happen. It is queried live.

A person's statement has no other home, so `ghost_reports` exists: `UNIQUE (user_id, job_id)`,
`applied_on date` as claimed, `retracted_at` for withdrawal. Retraction is how the signal
self-heals when an employer answers on day 40.

**Rejected:** recording the report as a backdated `user_jobs.applied_at`. `user_jobs` is tracking
truth and is already disciplined about honest dating ("an application recorded from mail is dated
by the mail, never `now()`"). Writing an unverifiable claim into it corrupts another feature's
semantics to save one table.

### The mail-connectivity gate

`jobtracking.Silence` takes the later of `applied_at` and the newest linked message, falling back
to `applied_at` when there is no linked mail. For a user with no connected mailbox there is
*never* linked mail, so the fallback always applies and every application of theirs reads silent
after 21 days — including ones the employer answered in a mailbox we cannot see.

On the owner's own board this is harmless: they know whether they were answered, and the marker is
a nudge. As input to a public claim it manufactures exactly the failure `docs/agents/mail-stack.md`
names the worst one — telling someone they were ignored when they were not.

**Decision:** an application contributes outcome evidence only if its owner has a connected
mailbox (`gmail_connections.status = 'connected'` or a row in `mailboxes`). This shrinks an
already tiny sample; the project's direction of error is settled and this follows it.

### The ATS cross-check

**Join key.** Not the full `RoleFingerprint` — it hashes the description, and aggregators truncate
or rewrite descriptions, so the same role never matches across sources. The key is the part that
survives rewriting: `company_slug` + `stripTrailingClause(normalizeRoleText(title))`, both already
implemented in `internal/jobhash`.

**Coverage gate.** The criterion fires only when the company has at least one open job from a
source of kind `ats` or `company` (`sources.ProviderKind`). Absence is evidence only where we
looked; without the gate it reports our own blind spots as the employer's fault.

**Staleness.** `ats_absent_at` older than 14 days is ignored. The worker re-stamps every run, so
an expired stamp means the worker stopped — and a stopped worker then falls silent instead of
accusing the catalogue indefinitely from a frozen snapshot.

**One column, not two.** "Checked and present" and "never checked" are the same thing to the
verdict — no evidence — so they share the NULL. A second column would carry a distinction nothing
reads.

**Known limit.** `company_slug` collisions run about 2%. A collision can match a title at the
wrong company and suppress a true positive. That direction — under-detection — is the accepted one.

### Both gates are structural, not conventional

Two guarantees are enforced by shape rather than by discipline:

- **Anonymity.** The outcome counts are absent from the payload below two distinct contributors.
  There is nothing for a future handler to forget to redact, because there is nothing to redact.
  The threshold is forced twice over: a count of one deanonymises the single applicant to the
  employer, and a single account should not be able to mark an honest posting.
- **The feature flag.** `possible` needs two criteria. At deploy, `ats_absent` is unpopulated and
  no report exists, so at most `evergreen_posting` can fire and the verdict cannot leave `none`.
  The convergence threshold *is* the flag — one that cannot be left on by accident.

### One chip, not two

`evergreen_posting` is precisely `reality.class = likely-evergreen`. Rendering both badges shows
one fact twice, the second time louder. The ghost chip supersedes the reality chip and carries
the evergreen fact inside its checklist; when ghost is `none`, `RealityBadge` renders unchanged.

## Risks / Trade-offs

**Staffing agencies reaching `possible`** — the failure that killed the previous spike. Two
criteria can both fire on an agency honestly: its pipeline really is evergreen, and a client role
it advertises really is absent from its own board. Convergence alone does not save us here.
Mitigation is procedural and has veto power: `cmd/ghost-crosscheck` is dry-run by default, and the
cron is enabled only after a prod report of who reaches `possible`, broken down by source and
company. Staffing-dominated → stop, and specify a dict-only exclusion first. The previous spike
died at calibration because calibration was a surprise; here it is a planned gate.

**Age is measured from first crawl.** Inherited from `jobreality` and not deepened, but any reader
of this feature must know that "open 240 days" means "we have seen it for 240 days".

**Telegram jobs cannot exceed `none`.** They have no company board, so `ats_absent` is
unreachable and only one criterion is available. Consistent with telegram already being excluded
from liveness probing.

**No dispute path in v1.** A moderator can only retract an individual `ghost_reports` row by hand.
Acceptable because the blast radius is bounded by what the mark does *not* do: it does not affect
ranking, does not hide the posting, does not close it, and disappears when the job closes. Compare
`job_reports`, whose resolve lever *does* close a job — which is why a human sits in that loop.

## Migration Plan

1. Migration `0051` (`ghost_reports`, `jobs.ats_absent_at`) applied to prod **before** the image.
   An unapplied migration 500s every job read (42703), not only this feature.
2. Deploy. The feature is silent by construction (see the structural flag above).
3. `cmd/ghost-crosscheck` run by hand, dry-run, producing the calibration report.
4. Calibration gate. Only on passing it does the cron start, and only then can a mark appear.

Rollback is dropping the cron; the verdict returns to `none` as stamps expire after 14 days.

## Open Questions

None blocking. The company response-rate tier will read "not enough data" for essentially every
company at launch; that is the expected state and the moat the previous spike said to wait for,
not an unfinished implementation.
