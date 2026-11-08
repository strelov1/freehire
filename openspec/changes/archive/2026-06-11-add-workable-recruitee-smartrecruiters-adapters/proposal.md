## Why

freehire only crawls three ATS platforms (greenhouse, lever, ashby). Every company on another platform is invisible — including large pools on Workable, Recruitee, and SmartRecruiters (e.g. Hugging Face on Workable). The OpenJobs dataset alone lists ~172 workable, ~55 recruitee, and ~288 smartrecruiters boards we cannot ingest today. Adding these three adapters unblocks that pool with no change to the pipeline.

## What Changes

- Add three `Source` adapters in `internal/sources`, each registered in `sources.All`:
  - **workable**: `GET apply.workable.com/api/v1/widget/accounts/{board}?details=true` — description is inline HTML; one request per board.
  - **recruitee**: `GET {board}.recruitee.com/api/offers/` — assembles the separate `description` + `requirements` HTML fields; one request per board.
  - **smartrecruiters**: paginated `GET api.smartrecruiters.com/v1/companies/{board}/postings` + a per-posting detail fetch for the description (`jobAd.sections`), because the list endpoint carries no description.
- Each adapter yields `description` as sanitized HTML via the shared `sanitizeHTML`, matching the existing convention.
- **BREAKING (spec-level only):** relax the source-ingest requirement that adapters map a posting "without performing a per-posting detail request" — an adapter MAY fetch per-posting detail when the platform's list endpoint lacks the description. No external API change.
- Follow-up (data, not freehire code): re-harvest OpenJobs for these providers, validate slugs live, append working boards to `sources.yml`, re-ingest.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities

- `source-ingest`: three new registered providers; the "no per-posting detail request" constraint is relaxed so an adapter may fetch detail when the list endpoint lacks the description.

## Impact

- **Code**: `internal/sources/{workable,recruitee,smartrecruiters}.go` (+ tests), one line each in `sources.All`. Possible small extension to the shared HTTP client only if pagination/detail needs it (prefer reusing `GetJSON`).
- **Data**: `sources.yml` grows by the validated boards from the follow-up harvest; re-ingest backfills. No schema/migration change.
- **Performance**: SmartRecruiters ingest is heavier (pagination + N detail requests per board); detail fetches are bounded-concurrency. Workable/Recruitee stay one request per board.
