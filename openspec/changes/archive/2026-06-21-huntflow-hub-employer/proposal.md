## Why

Some Huntflow career sites are not a single employer's board but a **community/agency hub**
that posts vacancies on behalf of many partner companies — the same shape as our `tecla`
marketplace adapter. AlumniHub (`alumnihub-career.huntflow.io`) is one: every vacancy belongs
to a different partner (Fluently, Mirai, Sparkland, "NDA (Hedge Fund)", …), yet our `huntflow`
adapter stamps the configured board company (`AlumniHub`) on all of them. The catalogue
therefore shows the wrong employer and groups unrelated jobs under one fake company.

The real employer is already present in each vacancy's `division` breadcrumb (leaf-first):
`<sub-team> · <Company> · Partners · Vacancies` — the company is the segment immediately before
the hub's `Partners` container folder. We can surface the true employer without any new source
or schema, just by reading what the feed already gives us — but only for boards explicitly
marked as such, since on an ordinary single-company Huntflow site `division` is a department,
not an employer.

## What Changes

- Add an **opt-in per-entry flag** to a board-file entry (`CompanyEntry.Hub bool`,
  `yaml:"hub"`, optional) marking a Huntflow board as a community hub whose real employer lives
  in each vacancy's `division`. Absent/`false` keeps today's behaviour for every other board.
- In the `huntflow` adapter, when the entry is a hub, set each job's `Company` from its
  `division` (the segment immediately before the literal `Partners` folder), falling back to
  the entry's configured company when the division has no such structure (e.g. the hub's own
  internal roles). When the entry is not a hub, behaviour is unchanged (`Company = e.Company`).
- Set `hub: true` on the `alumnihub-career` entry in `sources/huntflow.yml`.

## Capabilities

### New Capabilities
<!-- None. Reuses the source-ingest pipeline, write path, and dedup key unchanged. -->

### Modified Capabilities
- `source-ingest`: a board-file entry MAY be marked as a community hub; for a hub entry the
  `huntflow` adapter resolves each vacancy's employer from its own `division` breadcrumb
  (segment before the `Partners` folder) instead of from the configured company, falling back
  to the configured company when the division carries no employer.

## Impact

- **Code**: `internal/sources/source.go` (one optional field on `CompanyEntry`);
  `internal/sources/huntflow.go` (read `division`, add `companyFromDivision`, branch on the hub
  flag); `internal/sources/huntflow_test.go` (parser + hub/non-hub mapping tests).
- **Config**: `sources/huntflow.yml` — `hub: true` on `alumnihub-career` only. No other board
  file changes; the field is optional and ignored where absent.
- **Validation**: `Config.Validate` unchanged — `company` stays required (it is the hub's name
  and the per-vacancy fallback).
- **DB / dedup / search**: unchanged — `source = "huntflow"`, `external_id = <vacancy id>`. Only
  the displayed `Company` (and its derived `company_slug`) changes for hub boards; existing
  AlumniHub rows re-attribute to the real employer on the next crawl via `UpsertJob`.
- **Out of scope (known seam)**: the `Partners` container marker is hard-coded in the adapter
  for AlumniHub's org tree. If a future hub uses a differently named root folder, lift the
  marker into the config field (string instead of bool) — noted, not built now (YAGNI).
