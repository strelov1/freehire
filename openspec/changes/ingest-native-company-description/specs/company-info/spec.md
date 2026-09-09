## ADDED Requirements

### Requirement: Ingest fills company-info gaps from an adapter-supplied company description

When a board crawl yields a company-level description (see `source-ingest`),
the pipeline SHALL write it into that company's `tagline` (and a
`company_info` summary field) using the same fill-gap, never-overwrite
semantics specified for the curated company-info backfills: a company that
already has a non-empty `tagline` from any source SHALL be left unchanged.

This write SHALL happen as part of the ordinary ingest run, with no separate
backfill invocation required, so that a company newly discovered on a
supporting platform is filled the first time it is crawled.

#### Scenario: A newly-crawled company on a supporting platform gets its tagline on first crawl

- **WHEN** a board is crawled for the first time, its adapter returns a
  company-level description, and the resulting company has no existing
  `tagline`
- **THEN** the company's `tagline` and `company_info` summary field are set
  from that description as part of the same ingest run

#### Scenario: An existing tagline from any source is not overwritten

- **WHEN** a board crawl yields a company-level description for a company
  that already has a non-empty `tagline`
- **THEN** the stored `tagline` is unchanged

#### Scenario: No company description means no write

- **WHEN** a board crawl yields no company-level description (the adapter
  doesn't support it, or the field was blank)
- **THEN** the pipeline makes no `tagline`/`company_info` write for that
  company from this source
