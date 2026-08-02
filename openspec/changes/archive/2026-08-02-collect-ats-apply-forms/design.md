## Context

The crawl already visits every board on these three platforms; what it throws
away is the application form. A spike (2026-08-02) confirmed all three hand it
over anonymously, and measured what each one costs:

| Provider | Where the form lives | Cost per posting |
|---|---|---|
| Recruitee | already in the board listing ingest downloads | none |
| Greenhouse | `GET boards-api…/boards/{board}/jobs/{id}?questions=true` | 22 KB, 0.42 s |
| Ashby | one GraphQL call to `jobs.ashbyhq.com/api/non-user-graphql` | 15 KB, 0.35 s |

The spike also killed the obvious shortcut: Greenhouse ignores `questions=true`
on its list endpoint, and Ashby's `posting-api` board listing carries no form
either. Both need a request per posting, which is why acquisition splits in two.

Two existing constraints shape everything below. Adapters are read-only,
independent per board, and one slow board must not stall a crawl — the pipeline's
AGENTS.md is explicit that a fetch failure counts against board health. And the
repository already solved "work that must happen once per job, after the write,
without slowing the write": `enrichment_outbox` and `semantic_outbox`, drained by
`cmd/enrich` and `cmd/embed`.

## Goals / Non-Goals

**Goals:**

- One stored application form per job, verbatim, for the three providers that
  expose one.
- A posting's form is fetched once, not once per ingest run.
- Ingest gets no slower and no more failure-prone than it is today.
- The store's shape is settled well enough that a reader — a job-page summary,
  autofill — can be built against it without a migration.

**Non-Goals:**

- Reading the data back. No API field, no wire projection, no UI.
- Deriving anything from the form (question counts, essay detection, time
  estimates). That is the reader's job; see the seam below.
- Submitting an application.
- Lever, Workday, SmartRecruiters, Workable — see the proposal for why each.

## Decisions

### Two acquisition paths, split by cost, not by taste

Recruitee's form is already in memory when the adapter runs. Routing it through a
queue would mean writing a row, claiming it, and re-fetching data we had — an
outbox for the sake of symmetry. So Recruitee's adapter yields the form on the
`sources.Job` it already returns, and the ingest write path persists it in the
same transaction.

Greenhouse and Ashby cannot do that: the adapter would have to issue one request
per posting mid-crawl, turning a single board fetch into hundreds and making the
crawl's duration a function of board size. Worse, the adapter has no idea which
postings are new — that answer only exists after `UpsertJob`'s `ON CONFLICT`
resolves. So they enqueue and a worker drains.

*Alternative rejected:* one uniform path (queue everything, including Recruitee).
Uniformity is real, but it buys a queue round-trip and a second network fetch for
data already held, on 35k postings. The asymmetry is in the platforms; hiding it
costs more than naming it.

*Alternative rejected:* fetch Greenhouse/Ashby forms inside the adapter with a
bounded worker pool, the way the SmartRecruiters adapter already fetches detail
pages. That precedent exists because SmartRecruiters' list omits the
*description* — without it there is no job at all. A form is not load-bearing for
the job, so paying for it on the crawl's critical path inverts the priority.

### The store keeps the platform's own vocabulary

A stored form holds the ATS's field identifiers, option values and question text
exactly as returned. No mapping into a freehire vocabulary, no canonical field
names — the opposite of what `internal/skilltag` or `internal/classify` do,
deliberately. Those dictionaries exist to make facets comparable across
platforms; a form field's identifier is not for comparing, it is for handing back
to the platform that issued it. `question_67165648` means nothing except to
Greenhouse, and any normalization of it is loss.

That argues for the payload landing as JSONB with a thin typed envelope: the
provider, the capture time, and the form itself. Per-field columns would be a
schema that three platforms disagree about.

### Enqueue is gated on "has no form", not on freshness

`EnqueueJobEnrichment` gates on version and category; the natural analogue here
would gate on a content hash so an edited posting re-fetches its form. It should
not. A posting's description changes often; its form almost never — it is
configured once on the requisition. Gating on content change would re-fetch
135k Greenhouse forms every time a company reworded a job ad.

So: enqueue when the job has no current form. Re-capture is a deliberate,
operator-driven act (drop a provider's captures, let the queue refill), which the
provenance stamp on each row exists to make possible.

*Consequence, accepted:* a form edited after capture goes stale silently. For the
first consumer — telling a candidate roughly what awaits them — a form that is
weeks out of date is still overwhelmingly right. For autofill it matters more,
and autofill will want a freshness policy; that is the change that should pay for
it.

### Reuse the outbox shape verbatim

`apply_form_outbox` copies `semantic_outbox`'s columns and claim semantics:
`claimed_at` as a lease so a dead worker's rows are reclaimed without a reaper,
`SKIP LOCKED` so concurrent workers take disjoint rows, `attempts` and
`failed_at` for bounded retry. This is not a new pattern to design; it is one to
copy, and copying it means `cmd/capture-apply-form` reads like `cmd/embed`.

Ordering differs: `semantic_outbox` claims freshest-first because a stale vector
on a hot job hurts most. Here, order barely matters — but freshest-first is still
right, since a newly posted job is the one someone is about to apply to.

### The reader's seam, noted and not built

A job-page summary wants derived facts: how many questions, are any of them
essays, is salary asked, is visa asked, roughly how long this takes. All of it is
computable from the stored form. It is not computed here, because the shape of
that summary is a product decision this change does not make, and a derived
column added now would be a guess persisted.

The seam is the store itself: a reader package reads a job's form and derives
what it needs. If derivation later proves too slow to do per request, it becomes
a materialized column then — with a real consumer to shape it.

## Risks / Trade-offs

- **A platform changes its form payload shape and captures silently degrade** →
  The stored provenance (provider + capture time) makes the blast radius
  queryable and lets a provider's captures be dropped and re-queued as a group.
  Parsing failures are recorded on the outbox row like any other failure rather
  than being swallowed.

- **The first drain is a 185k-posting backlog against two platforms** → The
  worker takes two bounds, and they are separate on purpose: concurrency decides
  how fast a run goes, a per-run budget decides how long it goes for. Only the
  second keeps the backlog spread across scheduled runs — a drain loop that
  simply ran until the queue emptied would work for hours, and since nothing in
  this fleet holds a lock, a systemd `Type=oneshot` unit would then refuse every
  scheduled firing behind it while ingest kept enqueueing. With the budget, the
  cron cadence sets throughput. Worth watching on the first production run; the
  per-posting sizes (22 KB, 15 KB) are small, but the request count is not.

- **`cmd/capture-apply-form` has no lock, like every other worker here** → Per
  the repository's standing hazard, nothing takes a flock; only systemd
  `Type=oneshot` prevents a timer-driven worker from stacking on itself, and a
  manual run is unprotected. The outbox lease plus `SKIP LOCKED` makes concurrent
  drains safe by construction, which is the right answer anyway.

- **Recruitee's form travels on `sources.Job`, widening a struct every adapter
  shares** → Kept optional and nil-by-default, so no other adapter is touched and
  no other test changes. The spec pins this: an adapter yielding no form behaves
  exactly as today.

- **Storing employer-authored question text verbatim** → This change stores; it
  does not display. The republication question belongs to the reader change, and
  the answer already leans toward showing a derived summary rather than the text.

## Migration Plan

One migration (next free number appears to be **0071** — verify against
production before merge; migration numbers have collided here before by being
applied from an unmerged branch). It creates the form store and the outbox, both
additive, both empty. No column changes to `jobs`, so no lock on the catalogue's
hot table — worth keeping that way given the snapshot/DDL hazard.

Deploy order is the standing one: migrate before shipping code that reads the new
schema. The worker is inert until scheduled, and the queue simply accumulates
until it runs, so there is no window where ingest depends on the worker existing.

Rollback: stop scheduling the worker. Ingest keeps working — a failed enqueue
must not fail a crawl — and the tables can be dropped without touching a job row.

## Open Questions

- Which cadence the worker gets, and whether the first backlog drain should be
  run by hand under supervision before it goes on a timer.
- Whether Workable belongs in the next slice. The spike could not reach it from
  the probing IP while production ingest reaches it fine, so its exclusion is an
  absence of evidence and re-testing it from production is cheap.
