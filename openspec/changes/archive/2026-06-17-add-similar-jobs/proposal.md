## Why

A job seeker who lands on one vacancy has nowhere to go next within the same
topic — the detail page is a dead end. The semantic embedder that already powers
hybrid search (the `jobs_semantic` index) can surface vacancies close in meaning
to the one being viewed, turning every job page into an entry point for
discovery without any new indexing infrastructure.

## What Changes

- Add a public endpoint `GET /api/v1/jobs/:slug/similar` that returns jobs
  semantically nearest to the one identified by `:slug`, using Meilisearch's
  similar-documents query against the existing `jobs_semantic` index and its
  already-configured embedder.
- The result reuses the standard list envelope and the public job wire shape
  (`{"data": [...]}`), identifying jobs by `public_slug` and never exposing the
  internal numeric `id` — consistent with the other public job reads. The source
  job itself is never included in its own similar list.
- Add a "Similar jobs" section to the job detail page in the SPA, rendered from
  the new endpoint and degrading silently to nothing when there are no neighbours
  or the call fails (it must not break the page).
- No change to the embedder, the document template, the index settings, or any
  reindex path: closed jobs are already absent from `jobs_semantic` (the reindex
  command indexes open jobs only), so similar results are open jobs without any
  added filter, settings change, or re-embedding.

## Capabilities

### New Capabilities
- `similar-jobs`: surface vacancies semantically close to a given job — the
  similar-documents query against the semantic index, the public per-job
  `/similar` endpoint, and the job-detail "Similar jobs" section.

### Modified Capabilities
<!-- None: the embedder, index settings, and reindex behavior in job-search are
     unchanged; this capability consumes the existing semantic index as-is. -->

## Impact

- **API**: new route `GET /api/v1/jobs/:slug/similar` (public, unauthenticated).
- **Code**: `internal/search` (new `SimilarJobs` client method over the SDK's
  `SearchSimilarDocuments`), `internal/handler` (new handler + route wiring),
  `web/src/routes/jobs/[slug]` (new UI section + data load).
- **Dependencies**: none new — uses the pinned `meilisearch-go` and the existing
  `jobs_semantic` index/embedder.
- **No** DB migration, schema change, index settings change, or reindex required.
