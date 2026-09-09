## ADDED Requirements

### Requirement: Ingest fills a company-info gap from an adapter-supplied company description

When a board crawl yields a company-level description (see `source-ingest`),
the pipeline SHALL write it into that company's `company_info` summary field
using the same fill-gap, never-overwrite semantics specified for the curated
company-info backfills: an existing key in `company_info` SHALL be left
unchanged. This source SHALL NOT write `tagline` — that field stays reserved
for the short, curated-style values the other company-info sources produce,
never this source's raw employer-authored prose.

This write SHALL happen as part of the ordinary ingest run, with no separate
backfill invocation required, so that a company newly discovered on a
supporting platform is filled the first time it is crawled.

#### Scenario: A newly-crawled company on a supporting platform gets its company-info summary on first crawl

- **WHEN** a board is crawled for the first time and its adapter returns a
  company-level description
- **THEN** the company's `company_info` summary field is set from that
  description as part of the same ingest run, and its `tagline` is left
  unset

#### Scenario: An existing company_info key from any source is not overwritten

- **WHEN** a board crawl yields a company-level description for a company
  whose `company_info` already carries the summary key (or any other key)
  from another source
- **THEN** the stored value is unchanged

#### Scenario: No company description means no write

- **WHEN** a board crawl yields no company-level description (the adapter
  doesn't support it, or the field was blank)
- **THEN** the pipeline makes no `company_info` write for that company from
  this source
