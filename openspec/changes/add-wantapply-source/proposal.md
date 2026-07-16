## Why

Wantapply (`wantapply.com`, mirror `wantapply.cy`) is a curated IT job aggregator (~3000+ live vacancies, crypto/fintech/gamedev/CIS-relocation). Today only a small curated subset reaches our catalogue via the `wantapply_marketing` Telegram channel (~86 live jobs), and those telegram rows carry a `t.me/<post>` URL that gives no close signal, so they never expire. The public site exposes every vacancy as a clean `JobPosting` JSON-LD, and its `.cy` mirror is served without the WAF that blocks the `.com` host. A first-party site adapter captures the full catalogue with structured facets and a real close signal.

## What Changes

- Add a new `wantapply` source adapter (boardless aggregator + `HydratingSource`) that crawls `https://wantapply.cy/sitemap.xml`, fetches each new vacancy's detail page, and extracts its `JobPosting` JSON-LD into a normalized `sources.Job`.
- Register `wantapply` in `sources.All`; add a single boardless entry board file `sources/wantapply.yml`.
- The adapter is a board source, so the post-run unseen-sweep closes vacancies that drop out of the sitemap — the close signal telegram lacks.
- Expand the Wantapply Telegram footprint: add four channels (`wantapply_managers`, `wantapply_design`, `wantapply_analytics`, `wantapply_qa_jobs`) to `sources/telegram.yml`, keeping the existing `wantapply_marketing`. The telegram channels are retained alongside the site adapter; the small overlap is an accepted duplicate (cross-source dedup only suppresses aggregator copies against a first-party ATS, and neither telegram nor wantapply is one).

## Capabilities

### New Capabilities
- `wantapply-source`: crawl the Wantapply site (`.cy` mirror) sitemap, filter vacancy slugs, hydrate only new postings, and map each `JobPosting` JSON-LD into the normalized job shape; participate in the board unseen-sweep for closes.

### Modified Capabilities
<!-- Adding Telegram channel entries and a board-file entry is board onboarding (YAML config), not a spec-level requirement change to telegram-ingest or source-ingest. No delta specs. -->

## Impact

- **New code:** `internal/sources/wantapply.go` + `internal/sources/wantapply_test.go`, one line in `sources.All`, new `sources/wantapply.yml`.
- **Config:** four new channel entries in `sources/telegram.yml`.
- **Existing helpers reused:** `internal/sources/jsonld.go` (`LDJobPosting`), the shared HTTP client, the `HydratingSource`/`SeenRefresh` and boardless/aggregator markers in `source.go`.
- **Ops:** a new per-board ingest cron timer for `sources/wantapply.yml`; prod `sources` tree is synced from git.
- **No breaking changes**; no cross-source dedup or pipeline changes.
