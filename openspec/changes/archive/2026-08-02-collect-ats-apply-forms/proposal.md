## Why

We know what a job says, but nothing about what applying to it actually costs. A
posting that takes one click and a posting that demands fourteen screening
questions with three essays look identical in the catalogue, and a candidate only
discovers the difference after committing to the form.

A feasibility spike (2026-08-02, verdict PARTIAL) established that three of the
ATS platforms we already crawl hand out their application form over plain HTTP,
anonymously, in machine-readable shape — every field carrying its submit-time
identifier, type, required flag and enumerated options. Recruitee ships it in the
very response ingest already downloads. That data unlocks two things we cannot
build today: telling a candidate what awaits them before they invest the time,
and filling the form for them without first prising the options out of a live DOM
widget one click at a time.

This change captures and stores that data. It deliberately ships no reader — the
consumers land in follow-up changes, on a store whose shape is settled.

## What Changes

- A new `apply_forms` store holds one captured application form per job: the
  ATS's own field identifiers, types, required flags, option lists and question
  text, kept verbatim as the platform returned them, plus the capture stamp.
- The Recruitee adapter stops discarding the form data that already arrives with
  every posting (`open_questions`, `dynamic_fields`, the `options_*` standard
  field flags) and yields it alongside the job. No additional request.
- Greenhouse and Ashby forms are fetched by a queue-driven worker after the
  pipeline has written the job, because only then is it known which postings are
  new. Greenhouse needs `GET /boards/{board}/jobs/{id}?questions=true`; Ashby
  needs one GraphQL call. Both are per-posting; neither is available on the list
  endpoint the adapters use.
- A new `apply_form_outbox` queue with the retry and claim semantics already
  established by `enrichment_outbox` and `semantic_outbox`, enqueued by
  `UpsertJob` for the two providers that need a detail request.
- A new run-once-and-exit worker `cmd/capture-apply-form` drains the queue.

Explicitly out of scope, and why:

- **No reader.** No API field, no wire projection, no UI. The derived summary a
  job page would show (question count, essay presence, salary/visa asked, time
  estimate) is a separate change against a settled store; a seam is noted in
  design, not built.
- **No submission.** The spike found a captcha on every platform but Recruitee.
  Submitting from the server is not a thing this data enables.
- **Lever is deferred.** Its form is only readable by fetching the full 727 KB
  apply page per posting — 88% of the traffic this whole effort would cost, for
  16% of the coverage. It wants a lazy, on-demand fetch, which is a third
  acquisition mode and its own change.
- **Workday and SmartRecruiters are excluded, not deferred.** Workday's form
  needs an authenticated candidate session; SmartRecruiters' apply app sits
  behind DataDome, which turned away both curl and headless Chrome.
- **Workable is unresolved.** The spike could not reach it — Cloudflare rate-
  limited the probing IP while production ingest keeps working from its own.
  Absence of evidence; it is neither included nor ruled out.

## Capabilities

### New Capabilities
- `apply-form-capture`: capturing a job's application form from the ATS that
  published it, storing it verbatim, and keeping it fresh — the store, the
  per-provider acquisition, the queue, and the worker that drains it.

### Modified Capabilities
- `source-ingest`: an adapter MAY now yield an application form alongside the
  normalized job when its list endpoint already carries one, and the ingest write
  path persists it. Today the adapter contract stops at the job shape.

## Impact

- `internal/sources/` — the Recruitee adapter and the shape adapters yield.
- `internal/pipeline/` — `UpsertJob` persists an attached form and enqueues the
  outbox for Greenhouse and Ashby.
- `internal/db/` — new tables, new queries, regenerated code (`make sqlc`).
- `migrations/` — one new migration (next free number is 0071; verify against
  production before merge, per the migration-number hazard).
- `cmd/capture-apply-form/` — new worker; needs `DATABASE_URL`, exits non-zero on
  failure, follows `worker.Bootstrap`.
- Outbound traffic: roughly 22 KB per Greenhouse posting and 15 KB per Ashby
  posting, once per posting rather than once per ingest run. Recruitee adds none.
- No API surface, no web surface, no breaking change.
