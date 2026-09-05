## Why

The enrichment LLM has extracted each posting's own stated requirements since 2026-08-18 — `{text, priority}` pairs, sanitised, served over the API, and shipped into every job page's hydration payload. **Nothing renders them.** Measured on prod 2026-09-04: 133,404 open postings carry a non-empty list, averaging 9.3 items, and the bytes reach the browser only to be discarded.

Surfacing them is nearly free. Widening them is the second half: 133k is 2.9% of the 4.6M open catalogue, because LLM enrichment is expensive and slow. A deterministic parse of the description's own "Requirements" list reaches 12.8% of open postings (measured over 164 live postings) at zero model cost, and the two sources union rather than compete.

## What Changes

- **Render the list on the job page.** A "What they ask for" section in the right column of `JobView.svelte`, between the description and Skills, grouped `Required` then `Preferred`, every item verbatim. Absent when the list is empty.
- **Derive the same shape without an LLM.** A new deterministic extractor reads the posting's description HTML: it finds a heading in a controlled vocabulary (`Requirements`, `Qualifications`, `What you'll need`, `Nice to have`, …) and takes the `<li>` items of the list that follows it. Priority comes from the heading. No heading means no items — there is no fallback that guesses which list is the requirements list.
- **Store the derivation** in a new `jobs.requirements_derived` column (migration 0139), written by `UpsertJob` at ingest.
- **Merge it into the served field.** `SetJobEnrichment` gains a third overlay, chained after the two salary overlays it already carries: when the LLM payload states no requirements, the derived list fills them. `enrichment.requirements` stays the single field every consumer reads, and a later enrichment run cannot erase the derived list.
- **Backfill the existing catalogue** with a dedicated one-off `cmd/backfill-requirements` over open postings — keyset-paced, chunked, idempotent — following `cmd/backfill-clearance`'s shape rather than folding into the ~15h `cmd/backfill-derive` pass.

No breaking changes. The wire shape does not move; a field that was always present and usually empty simply becomes populated more often.

## Capabilities

### New Capabilities
- `job-requirements-display`: the job detail page renders the posting's stated requirements, grouped by priority, verbatim, and renders nothing when there are none.
- `posting-requirements-derivation`: a deterministic, dictionary-gated extraction of a posting's requirements from its description markup, stored per job and refreshable by a one-off backfill.

### Modified Capabilities
- `job-enrichment`: the served `enrichment.requirements` is filled from the deterministic derivation when the LLM payload states none, so the field's coverage no longer depends on whether the model has run.

## Impact

- **Schema:** migration 0139 adds `jobs.requirements_derived jsonb NOT NULL DEFAULT '[]'::jsonb`.
- **SQL:** `SetJobEnrichment` and `UpsertJob`'s conflict branch gain one overlay each; a new chunked update query for the backfill. `make sqlc` regeneration required.
- **Go:** a new extractor package under `internal/job`; `internal/ingest/pipeline` passes the derived list through `UpsertJob`; new `cmd/backfill-requirements`.
- **Web:** one new section in `web/src/lib/components/JobView.svelte`. The `Requirement[]` type is already generated in `web/src/lib/generated/contracts.ts`.
- **Not affected:** no Meilisearch reindex (the job detail page reads a Postgres row and the field is never a filter), and no enrichment version bump (the derivation is orthogonal to the model payload and must not re-queue 1.3M rows).
- **Downstream:** nothing else reads `enrichment.requirements` yet. `internal/candidate/matchanalysis` builds its own requirement list from a first LLM stage (`analyzer.go`'s `s1.Requirements`) and `internal/candidate/coverletter` takes that list, so neither inherits anything from this change. An earlier draft of this proposal claimed they did, and used it to argue for storing the derivation rather than parsing on the request path; that argument was wrong. The storage decision stands on its own smaller ground — the backfill needs somewhere to put its answer, and a per-request parse of every job body is work repeated on every read.
