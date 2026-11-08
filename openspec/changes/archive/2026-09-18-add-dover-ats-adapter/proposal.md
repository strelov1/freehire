## Why

A visitor asked about a live posting hosted on Dover (`app.dover.com/apply/<company-slug>/<job-id>`),
an ATS freehire's catalogue does not crawl at all today: no `atsboard` recognizer, no `internal/ingest/sources`
adapter, no `applyform` handler. Dover exposes a public, unauthenticated JSON API for its careers
pages (company resolve, paginated job listing, per-job detail) with no bot protection on the data
endpoints — only the apply-submission page itself uses Cloudflare Turnstile — so it is a
straightforward addition alongside freehire's other boarded ATS adapters (greenhouse, ashby, lever).

## What Changes

- Add a new `dover` ingest source adapter (`internal/ingest/sources`) that, given a board id (the
  company's Dover slug, e.g. `qompyl`), resolves the client UUID, lists that company's published
  jobs, and hydrates each with its full detail (description, location, workplace type,
  compensation, employment type, visa support) via Dover's public JSON API.
- Add an `atsboard` recognizer so a URL like `https://app.dover.com/apply/<slug>/<job-id>` or
  `https://app.dover.com/jobs/<slug>` resolves to `(dover, <slug>)` for board-contribution intake.
- Add an `applyform` recognizer/capture path for Dover's `application_questions[]`, matching the
  shape the apply-form capture queue already stores for greenhouse/ashby/workable/lever.
- Register `dover` in the provider registry so it appears in the source facet and generated
  `SOURCE_VALUES`.

## Capabilities

### New Capabilities

- `dover-source`: crawling Dover-hosted company career pages into the job catalogue via Dover's
  public JSON API (board resolution, paginated listing, per-job hydration, posting normalization).

### Modified Capabilities

(none — `apply-form-capture` and `board-harvest`'s existing requirements already cover "one more
recognized platform"; only their recognizer registries gain an entry, no requirement changes)

## Impact

- `internal/ingest/sources/dover.go` (new adapter) + registration in the provider registry.
- `internal/ingest/atsboard/board.go` (new recognizer case).
- `internal/ingest/applyform/` (new recognizer for Dover application questions).
- Generated source-facet values (`contracts.ts` / `SOURCE_VALUES`) regenerated, not hand-edited.
- No schema migration expected — Dover fits the existing `jobs`/`boards`/`apply_forms` shapes.
