# Search conventions

Meilisearch-backed keyword/faceted search over jobs and companies. The package doc in
`client.go` explains the index topology; this file covers what the code can't tell you.
This package also owns the TEI embedding calls (`embed.go`) that feed the pgvector-backed
`job_semantic_chunks` table — see `internal/embed/AGENTS.md`. There is no live semantic
search index anymore: the
`jobs_semantic` Meilisearch index (and `reindex --semantic`/`--from-pg`/`--posted-within`,
`SearchParams.SemanticRatio`, `Client.SimilarJobs`/`RecommendByVector`/`EmbedText`) were
removed in openspec/changes/drop-hybrid-search-pgvector-similar — `/jobs/:slug/similar`
reads a precomputed pgvector lookup instead (`internal/similarjobs`,
`cmd/similar-backfill`), and `/me/recommendations` was dropped outright.

## Always true

- **A job whose category is unresolved by both the title dictionary and the LLM never
  enters the index.** `search.CategoryUnresolved` (`document.go`) reports true when
  `jobs.category` is empty (`internal/classify` found nothing in the title) AND the raw
  LLM enrichment's own `category` is also empty or the catch-all `"other"` — read from
  the raw JSON, not `jobview`'s folded `Enrichment.Category`, which the dictionary
  column always overwrites (`internal/classify/AGENTS.md`) and so never carries the
  LLM's answer. Both `cmd/reindex`'s `splitJobs` and `cmd/search-drain`'s `IndexBatch`
  apply it — added because this bucket was measured at ~65% of the open catalogue
  (broad multi-industry ATS crawls contribute postings like "Industrial Painter" or
  "Backhoe Loader Operator" that neither dictionary was ever meant to place), diluting
  every keyword and category-filtered search with undifferentiated noise. A job later
  categorized by a dictionary update or a fresh LLM pass is picked up by the next full
  `cmd/reindex` run, not incrementally — `SetJobEnrichment` does not enqueue
  `search_outbox`, so there is no faster path today.
- **A job with no posting body never enters the index.** `search.DescriptionMissing`
  (`document.go`) tests the VISIBLE text — `stripToPlainText` — not the raw column, because a
  source that publishes an empty rich-text field serves markup with no words in it
  (`<p>&nbsp;</p>`) and the ingest sanitizer legitimately keeps those tags. An adapter
  deliberately stores a posting whose detail fetch failed (the listing is authoritative for the
  job existing, and a later crawl can hydrate it), so a body-less row is a recoverable ingest
  state, not an error — but a vacancy page with a title and nothing under it is not a listing
  anyone can act on. `cmd/reindex`'s `splitJobs`, `cmd/search-drain`'s `IndexBatch`, and
  `internal/linkimport` all apply it, alongside `CategoryUnresolved`. Measured at 15,816 live
  rows (0.48% of the open catalogue) when the rule was added (freehire#1866). The exclusion is
  self-healing: a row re-enters the index the moment a crawl fills its description, with no
  backfill or manual step.
- **Meilisearch has ONE serial task queue.** Two rebuilds do not run concurrently — the
  second queues behind the first and looks like a hang while the engine is genuinely busy.
  Before triggering any rebuild, check `ps aux | grep reindex` and
  `GET /tasks?statuses=processing`. Never stack `reindex-companies` with `make reindex`.
- **Killing the reindex client does not cancel enqueued Meili tasks.** To actually stop:
  `POST /tasks/cancel?indexUids=<uid>&statuses=enqueued,processing`. That cancelation itself
  queues, and it is irreversible — don't fire it on an unconfirmed diagnosis.
- **A normally-aborted rebuild no longer orphans the `*_rebuild` index.** `reindexFull`
  (and `reindexCompanies`, likewise) defers `Rebuild.Cleanup` on every non-promoted exit —
  best-effort on a cancellation-immune 30s context — so a failed or cancelled run drops its
  half-built rebuild index itself rather than leaving a full-size index (~55 GB at
  catalogue scale) on disk. An orphan now survives only a hard kill (SIGKILL, power loss)
  or a failed cleanup; reclaim that with `DELETE /indexes/<uid>_rebuild`. (The production
  ENOSPC → orphan → less disk → ENOSPC death spiral came from these orphans.)
  `Rebuild.Prepare` also drops a leftover before starting.
- **Live reads are never affected by a rebuild** — the swap is atomic.
- A full rebuild (`scope=full`) pushes **every** open, non-private, categorized document
  unconditionally to the fresh rebuild index — `content_hash` is never read in
  `cmd/reindex`; the `indexed=X skipped=Y` log line counts rows `ResilientPage` skipped
  for corruption, not hash-skipped ones. (An older version of this doc claimed
  content_hash-incremental full rebuilds; that behavior does not exist in the code —
  verified 2026-08-05.) `content_hash`-gating only exists one layer up, at the
  ingest→`search_outbox` enqueue decision (`cmd/ingest`'s `needsIndex`) — the reason a
  full reindex is the correct, if slow, way to surface any change a write path forgot
  to enqueue, including `is_tech` (deliberately excluded from `jobhash.Of()`, so an
  is_tech-only flip needs a full reindex or some other change to the same row to reach
  the index).
- `SubmitJobs` submits **without awaiting** the Meili task (`internal/linkimport`'s single
  on-demand doc push — `cmd/resolve-url` and the browser extension's "add this page", both
  human-triggered and low-volume, so one unawaited push per action is fine); `IndexJobs`
  awaits (the reindex path AND `cmd/search-drain`'s wave push — a wrong/silently-dropped
  push there would leave the outbox entry deleted with nothing actually indexed). Pick
  deliberately: **never call `SubmitJobs` from a high-frequency caller** — Meilisearch
  re-merges its inverted index/facet structures across the WHOLE live index on every push
  regardless of batch size (measured 2026-08-05 at 90-180s+ per push on the ~2.7M-doc index), so many small
  unawaited pushes queue up and saturate host disk IO. That is exactly what happened when
  `cmd/ingest` called it once per crawl across ~169 independent per-board processes; the
  fix routes that traffic through `search_outbox` + `cmd/search-drain` instead (see
  `internal/searchdrain`), collapsing many small pushes into few, fat, awaited ones.
- `swapIndexes` calls `POST /swap-indexes` over raw HTTP, not the SDK: the pinned
  meilisearch-go always serializes a `rename` field that engine v1.13 rejects.
- Indexed descriptions are capped at `maxIndexedDescriptionRunes`; `maxTotalHits` is the
  count-honesty cap, **not** the pagination guard (that's `maxSearchWindow` in the handler).
- **Both indexes are also what the sitemaps page** (`sitemap.go` → `/api/v1/jobs/sitemap`
  and `/api/v1/companies/sitemap`), which makes the "unresolved category never enters the
  index" rule above a decision about what Google crawls, not only about what search
  returns. They read through `GET /indexes/<uid>/documents` — offset-addressed, unaffected
  by `maxTotalHits` and `maxSearchWindow` (both bound `/search`, not this route), and
  measured flat in the offset: 0 and 1.2M both answer under 0.25s on prod. That replaced a
  Postgres `row_number()` walk which had grown to 64s over 3.4M rows and was timing out
  `/sitemap.xml` outright. Two consequences worth holding: an index outage is now a
  sitemap outage too, and `CompanyDocument.UpdatedAt` exists **only** to carry a
  `<lastmod>` — it is not searchable, filterable, or sortable, and a company indexed
  before it was added simply ships without one until the next `reindex-companies`.

## Adding a filterable attribute

Adding one creates a hard-500 window: the app emits the new filter the moment the image
rolls out, but the attribute only becomes filterable when the rebuild swaps in — ~26 min at
catalogue scale. Meili answers `invalid_search_filter` (400), and the handler maps any Meili
error to 500, so the whole filtered page breaks rather than degrading.

Either run the reindex **before** rolling out the app image, or push the new index settings
to the **live** index first (settings updates are cheap; documents lag, so results are stale
or empty — never a 500).

## Limitations

- A Meili filter error 500s the page instead of degrading. That's the robustness seam.
