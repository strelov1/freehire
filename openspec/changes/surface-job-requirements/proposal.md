## Why

The enrichment LLM has extracted each posting's own stated requirements since 2026-08-18 — `{text, priority}` pairs, sanitised, served over the API, and shipped into every job page's hydration payload. **Nothing renders them.** Measured on prod 2026-09-04: 133,404 open postings carry a non-empty list, averaging 9.3 items, and the bytes reach the browser only to be discarded.

Surfacing them is nearly free. Widening them is the second half: 133k is 2.9% of the 4.6M open catalogue, because LLM enrichment is expensive and slow. A deterministic parse of the description's own "Requirements" list reaches a further **28.0%** of open postings at zero model cost, and the two sources union rather than compete.

*(Planning estimated 12.8% from 164 postings in the API's default order. That was a biased sample — newest first skews to flat-text aggregators — and the shipped result is measured with `TABLESAMPLE` over 350k open rows after the backfill. See design.md for why id order and recency both proxy for the source.)*

## What Changes

- ~~**Render the list on the job page.**~~ **Withdrawn.** A "What they ask for" section shipped, ran on prod for a day, and was removed: the extractor fires exactly when the description already displays that list, so the section was verbatim duplication on every posting it appeared for. See tasks.md §10. The list is still SERVED under `enrichment.requirements`; nothing renders it.
- **Derive the same shape without an LLM.** A new deterministic extractor reads the posting's description HTML: it finds a heading in a controlled vocabulary (`Requirements`, `Qualifications`, `What you'll need`, `Nice to have`, …) and takes the `<li>` items of the list that follows it. Priority comes from the heading. No heading means no items — there is no fallback that guesses which list is the requirements list.
- **Store the derivation** in a new `jobs.requirements_derived` column (migration 0139), written by `UpsertJob` at ingest.
- **Merge it into the served field.** `SetJobEnrichment` gains a third overlay, chained after the two salary overlays it already carries: when the LLM payload states no requirements, the derived list fills them. `enrichment.requirements` stays the single field every consumer reads, and a later enrichment run cannot erase the derived list.
- **Backfill the existing catalogue** with a dedicated one-off `cmd/backfill-requirements` over open postings — keyset-paced, chunked, idempotent — following `cmd/backfill-clearance`'s shape rather than folding into the ~15h `cmd/backfill-derive` pass.

No breaking changes. The wire shape does not move; a field that was always present and usually empty simply becomes populated more often.

## Capabilities

### New Capabilities
- `posting-requirements-derivation`: a deterministic, dictionary-gated extraction of a posting's requirements from its description markup, stored per job and refreshable by a one-off backfill.

### Modified Capabilities
- `job-enrichment`: the served `enrichment.requirements` is filled from the deterministic derivation when the LLM payload states none, so the field's coverage no longer depends on whether the model has run.

## Impact

- **Schema:** migration 0139 adds `jobs.requirements_derived jsonb NOT NULL DEFAULT '[]'::jsonb`.
- **SQL:** `SetJobEnrichment` and `UpsertJob`'s conflict branch gain one overlay each; a new chunked update query for the backfill. `make sqlc` regeneration required.
- **Go:** a new extractor package under `internal/job`; `internal/ingest/pipeline` passes the derived list through `UpsertJob`; new `cmd/backfill-requirements`.
- **Web:** none. A section was added and then removed (tasks.md §10); the helpers it needed went with it.
- **Not affected:** no Meilisearch reindex (the job detail page reads a Postgres row and the field is never a filter), and no enrichment version bump (the derivation is orthogonal to the model payload and must not re-queue 1.3M rows).
- **Downstream:** nothing else reads `enrichment.requirements` yet. `internal/candidate/matchanalysis` builds its own requirement list from a first LLM stage (`analyzer.go`'s `s1.Requirements`) and `internal/candidate/coverletter` takes that list, so neither inherits anything from this change. An earlier draft of this proposal claimed they did, and used it to argue for storing the derivation rather than parsing on the request path; that argument was wrong. The storage decision stands on its own smaller ground — the backfill needs somewhere to put its answer, and a per-request parse of every job body is work repeated on every read.
