# Search conventions

Meilisearch-backed keyword/faceted search over jobs and companies. The package doc in
`client.go` explains the index topology; this file covers what the code can't tell you.
This package also owns the TEI embedding calls (`embed.go`) that feed the pgvector-backed
`job_semantic_chunks` table — see `internal/ai/embed/AGENTS.md`. Those vectors never reach
the jobs index: its only embedder is `skills`. The `desc` embedder that carried them (for
agents' opt-in semantic search and hybrid similar jobs) was removed to speed up index
writes; `ensure()`'s embedder reset drops it from a live index that still declares it.

Similar jobs use the same live index: a keyword search on the source's title.
Country, city (when known) and specialization are mandatory constraints; when only a
country is known it remains the location constraint, and region is used only without
country/city. Unknown location or specialization yields no related postings. Results
are re-read from Postgres to exclude currently closed, private, duplicate and unseen jobs.
There is no precomputed similarity list or similarity backfill worker; its columns and
HNSW index were dropped by `0228_drop_precomputed_similar_jobs.sql` and
`0229_drop_similarity_hnsw_index.sql` (#4088).

## Always true

- **Closed and duplicate index documents carry internal visibility markers.** Every
  jobs query, including keyword/vector/federated search, facet/distinct counts and job
  sitemaps, keeps `NOT is_closed = true AND NOT is_duplicate = true`. Legacy documents
  without markers remain eligible. Full replacement from a canonical open row clears
  both markers. Status updates use the experimental edit-documents-by-function API:
  enable it and await both live filterable-attribute settings before deploying readers
  or the drain. Legacy documents can still carry retired `desc` vectors after
  their embedder was removed. Marker updates clear and remove that retired field before
  assignment to stay under Rhai's aggregate array limit, preserving active skill vectors. Updates touch existing documents only, preserving text/vectors without
  creating skeletons. Private/missing rows still require physical removal; a full rebuild
  omits closed/duplicate rows to reclaim retained storage. Before rolling back readers,
  physically remove marked documents and await completion or the old filter exposes them.
- **A job whose category is unresolved by both the title dictionary and the LLM never
  enters the index.** `search.CategoryUnresolved` (`document.go`) reports true when
  `jobs.category` is empty (`internal/dict/classify` found nothing in the title) AND the raw
  LLM enrichment's own `category` is also empty or the catch-all `"other"` — read from
  the raw JSON, not `jobview`'s folded `Enrichment.Category`, which the dictionary
  column always overwrites (`internal/dict/classify/AGENTS.md`) and so never carries the
  LLM's answer. Both `cmd/reindex`'s `splitJobs` and `cmd/search-drain`'s `IndexBatch`
  apply it — added because this bucket was measured at ~65% of the open catalogue
  (broad multi-industry ATS crawls contribute postings like "Industrial Painter" or
  "Backhoe Loader Operator" that neither dictionary was ever meant to place), diluting
  every keyword and category-filtered search with undifferentiated noise. A job later
  categorized by a dictionary update or a fresh LLM pass is picked up by the next full
  `cmd/reindex` run, not incrementally — `SetJobEnrichment` does not enqueue
  `search_outbox`, so there is no faster path today. Two placements other than a category
  keep a job in: `is_tech` confidently true, and a title `internal/dict/roletype` marks
  `people_manager` — the `role_type` facet reaches a General Manager no category files,
  and that axis is what the retired `management` category was a proxy for. The gate reads
  the same `roletype.Derive` `FromJob` writes onto the document, and
  `classify.ConfirmedNonTech` applies the same veto at ingest, so a posting is kept iff
  some facet reaches it. The company count in `RefreshCompanyFacets` mirrors every
  branch but that one (the dictionary is Go), so it under-counts such postings.
- **`category=management` is translated, not dropped.** The category is retired
  (`vocab.RetiredCategoryManagement`), but saved searches, alerts and shared links carry
  it, often ORed with other categories. `retiredCategory` (`query_filter.go`) answers the
  value with `role_type = "people_manager"` inside the category group, so
  `category=sales&category=management` still means "sales OR people managers", and
  `category_exclude=management` excludes the people managers. Nothing widens, so
  `meta.ignored_params` has nothing to report, and `IgnoredValues` counts it as known.
- **An unknown value of a closed facet is reported, not dropped** (`query_values.go`,
  freehire#3280). `work_mode=foo` still reaches the filter and narrows to nothing, so
  `total: 0` would read as an empty market; `IgnoredValues` names it in
  `meta.ignored_values` (`{"work_mode": ["foo"]}`, `_exclude` keyed as sent). Only the
  facets in `closedFacets` are checked, each against its `vocab` list itself — an open
  facet (`skills`, `countries`, `cities`, …) has no list to check, and a copy here would
  drift. The comparison is trimmed and case-insensitive so a value the filter could still
  match is never named. A facet added to `closedFacets` must also be in `StringFacets`; a
  test holds that.
- **A job with no posting body never enters the index.** `search.DescriptionMissing`
  (`document.go`) tests the VISIBLE text — `stripToPlainText` — not the raw column, because a
  source that publishes an empty rich-text field serves markup with no words in it
  (`<p>&nbsp;</p>`) and the ingest sanitizer legitimately keeps those tags. An adapter
  deliberately stores a posting whose detail fetch failed (the listing is authoritative for the
  job existing, and a later crawl can hydrate it), so a body-less row is a recoverable ingest
  state, not an error — but a vacancy page with a title and nothing under it is not a listing
  anyone can act on. `cmd/reindex`'s `splitJobs`, `cmd/search-drain`'s `IndexBatch`, and
  `internal/ingest/linkimport` all apply it, alongside `CategoryUnresolved`. Measured at 15,816 live
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
- `SubmitJobs` submits **without awaiting** the Meili task (`internal/ingest/linkimport`'s single
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
  `internal/search/searchdrain`), collapsing many small pushes into few, fat, awaited ones.
- `swapIndexes` calls `POST /swap-indexes` over raw HTTP, not the SDK: the pinned
  meilisearch-go always serializes a `rename` field that engine v1.13 rejects.
- Indexed descriptions are capped at `maxIndexedDescriptionRunes`; `maxTotalHits` is the
  count-honesty cap, **not** the pagination guard (that's `maxPageWindow` in the handler,
  which bounds the Postgres-backed lists too since the 2026-09-14 deep-offset outage).
- **Both indexes are also what the sitemaps page** (`sitemap.go` → `/api/v1/jobs/sitemap`
  and `/api/v1/companies/sitemap`), which makes the "unresolved category never enters the
  index" rule above a decision about what Google crawls, not only about what search
  returns. They read through `GET /indexes/<uid>/documents` — offset-addressed, unaffected
  by `maxTotalHits` and `maxPageWindow` (both bound `/search`, not this route), and
  measured flat in the offset: 0 and 1.2M both answer under 0.25s on prod. That replaced a
  Postgres `row_number()` walk which had grown to 64s over 3.4M rows and was timing out
  `/sitemap.xml` outright. **`ListFreshSitemapPage` is the exception and goes through
  `/search`** (`/api/v1/jobs/sitemap/fresh`), because `/documents` accepts no `sort` and
  `web-ssr-seo` requires a newest-first job sub-sitemap — which the paged reader silently
  never provided. So that one IS subject to `maxTotalHits` (harmless at 10,000,000) and is
  NOT flat in the offset: measured warm at `limit=10000` on prod 2026-09-22, 716ms at
  offset 0 rising to 1,776ms at 30,000 in the engine, and 1.3s rising to 2.6-4.9s end
  to end through the sub-sitemap — with one cold 8.5s reading on the deepest chunk at
  the first request after a release. The engine figure is the one to reason about when
  changing the window; the end-to-end figure is the one the 10s SSR fetch timeout
  actually applies to, and the two differ by more than 2x. Its window is bounded at `freshSitemapMaxOffset`
  in the handler for exactly that reason, and the bound covers offset PLUS limit, since on
  a sorted read the caller controls the depth through both. Two consequences worth holding: an index outage is now a
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

The hazard is one-directional, which is worth knowing when a change goes the other way:
REMOVING a filterable attribute needs no dance at all. `company_slug_folded` was declared here
for the ingest-time coverage gate and was dropped when that gate moved to Postgres — the binary
stops asking for it first, and the live index keeps a harmless declaration until the next
rebuild. Only a binary that asks for an attribute the live index has not declared breaks.

## Adding a sortable attribute

Same window, same rule, one worse detail. Meili answers `invalid_search_sort`, which the
handler maps to 500 the same way — so a binary whose `searchSortable` accepts a value the
live index has not declared does not fall back to the default ordering, it breaks the page.
Settings first, binary second.

**A hand patch must send the COMPLETE `sortableAttributes` list.** Meili replaces that
setting wholesale rather than merging it, so a patch naming only the new attribute silently
drops `posted_at` — and `posted_at` is what the feed's DEFAULT ordering uses, so the blast
radius is every caller, not just the one who picked the new sort. `EnsureIndex` runs only
in tests, and settings otherwise reach production when `cmd/reindex` swaps a freshly built
index in. Read the setting back afterwards; a 200 on the patch only means the task was
accepted.

**`cmd/search-settings-drift` is the read side of this hazard.** It cannot fix the
ordering — settings-before-binary is still a human decision, made when a deploy and a
reindex are sequenced — but it means a gap is found on its own five-minute schedule rather
than by a caller hitting the 500 first: `Client.SettingsDrift` fetches the live jobs and
companies indexes' settings and diffs them against `facetSettings()`/`companySettings()`,
publishing what it finds (`freehire_search_settings_drift_count`) via the node_exporter
textfile collector. It only reports the hazardous direction — a live index still declaring
something this binary no longer asks for is never drift, per the note above.

`view_count` (the "Most viewed" ordering) is the easy case of this and worth contrasting
with the match embedder below: the counter rides the embedded job projection, so it is
already on every document. Declaring it sortable is the whole change, and the ordering is
correct the moment the setting applies — no rebuild to wait for, no dark flag. What the
counter does NOT get is outbox plumbing: `cmd/rollup-views` moves it daily without touching
indexed content, so the incremental push never fires, but the scheduled rebuild reads it
from Postgres more often than the rollup writes it. The indexed figure is never staler than
the source figure.

## Skill vectors and the match sort

The facet index carries ONE embedder, named `skills` and sourced `userProvided`. No
model is ever called: the vectors are arithmetic over `internal/dict/skillvec`'s
permanent position registry (unweighted — see that package on why rarity weighting was
removed), written into each document's `_vectors` by the three
indexers (`cmd/reindex`, `cmd/search-drain`, `internal/ingest/linkimport`).

**Two ordering hazards, and both look like an outage rather than a missing feature:**

1. **The embedder must exist in the LIVE index before a binary queries it.** A vector
   search against an index without it is a 400, which this package's error mapping
   turns into a 500 — the same shape as the filterable-attribute window above, and it
   breaks `/jobs/search` for everyone who picked the sort. Patch settings first, roll
   the binary second.
2. **The vectors only exist after a full rebuild.** Declaring the embedder does not
   retro-fill 1.36M documents; until the rebuild lands, the match sort returns only
   what has been re-indexed since. It is a thin feed, not an error, so nothing alerts.

That second hazard used to be handled by shipping the control dark behind a
`PUBLIC_MATCH_SORT` flag. The rebuild has long since landed and the flag is gone; the
control is offered to every visitor, and a caller with no skills to rank against is told
so by the feed rather than having the ordering hidden from them. What survives is the
ORDER the hazard implies, and it still binds any future change to the vectors: deploy →
settings → rebuild → verify by hand with `?sort=match`. Rolling out a binary that queries
an embedder the live settings do not yet declare is still the first hazard above.

`skillvec.Dimensions` is baked into the live settings. **Changing it requires a full
rebuild**, and until that rebuild finishes the index rejects every document carrying
the new width.

**The embedder reset runs BEFORE the settings update, never after.** Meilisearch merges
settings, and it merges `embedders` BY KEY — an update naming one embedder leaves any
other in place — so the reset still has to happen or an index a prior version gave a
model-backed embedder would keep embedding forever. But doing it afterwards deletes the
embedder the settings just declared. That shipped once: `EnsureIndex` and
`Rebuild.Prepare` each declared the skill embedder and immediately reset it, so a
rebuild would have produced an index with no embedder while every unit test passed and
nothing logged. The order now lives in `ensure()` alone, and
`embedder_integration_test.go` asserts the embedder survives both paths against a real
engine — which is the only place that claim can be tested, since a unit test of
`facetSettings()` cannot see what the index ends up with.

A rebuild with vectors is materially slower and larger than one without — the cost is
HNSW graph construction, so it lands on full rebuilds, not on incremental
`search_outbox` pushes. Schedule it deliberately; `freehire-reindexw` has outgrown its
slot before, and the disk floor already refuses rebuilds when free space is tight.

**Before blaming the vectors for a slow rebuild, check what the producer reads.** On
2026-10-22 a rebuild took 7.7h, and the engine was not the bottleneck: `Promote` waited
only minutes for the queued batches at the end, and the per-minute document rate rose
from ~4k to ~25k as the keyset moved from old ids (~21% open and canonical) to new ones
(~64%). The scan was paging the WHOLE jobs table — 18.8M rows, every closed one
de-TOASTed and thrown away, a population that only grows now that nothing deletes
closed rows — to index 3.78M documents. It now reads `ListIndexableJobsByIDAfter`
(open, canonical, public: ~6.2M rows through `jobs_sitemap_idx`).

## Query operators

`ParseQuery` (`query_syntax.go`) reads `OR` and a leading `title:` in `q`. A
query with neither is one alternative whose text is `q` byte-for-byte, so it
builds exactly the single-index request it built before operators existed.
Keep it that way: every search on the site goes through it.

Two or more alternatives go out as ONE federated `/multi-search`
(`federated.go`). Each query carries the same filter, sort and vector, and the
federation pages, deduplicates and merges facets. Two traps:

- Per-query `limit`, `offset` and `facets` are a 400 inside a federation.
- `mergeFacets` defaults to 100 values per facet, so it must pass
  `maxValuesPerFacet`.

The merged sort is global only because the ranking rules put `sort` right after
`words`. Each hit fully matches its own alternative, so the sort decides.
Moving `sort` later breaks that, and `TestSearchOperatorsOnRealEngine` catches it.

## Limitations

- **A filter Meilisearch rejects degrades rather than failing the request** (`internal/api/handler/search.go`'s `runJobSearch`, both job-search endpoints). This bullet used to claim the failure was a flat 500 — it was not, even before this fixed anything: `queryErr` already classified it as `ErrBadQuery`, and `errors.go` already mapped that to 400. The stale wording aside, the substance held: a 400 still failed the whole request rather than getting the "drop it, report it" treatment `search.UnknownParams` already gives an unrecognized param. Now: when the primary query fails as `ErrBadQuery` and a dynamic filter was actually part of the request, the handler retries once with the filter dropped and reports every active filter param in `meta.ignored_params` — `search.ActiveFilterParams` is the mirror of `UnknownParams` this needed. The realistic trigger stays what it always was: the "Adding a filterable attribute" deploy window above, not a malformed client value (every facet/scalar value is escaped before it reaches Meilisearch). See `openspec/changes/search-filter-error-degrade`.
- **The analogous sort-attribute hazard is NOT closed by the above.** "Adding a sortable attribute" (above) has the same root cause and the same ~26 min window, but a bad `sort` value fails the query the same way a bad filter used to — dropping a sort falls back to default ordering, a differently-shaped fix nothing here attempts yet. `searchSortable`'s `view_count` entry (`internal/api/handler/search.go`) still carries the original warning.
