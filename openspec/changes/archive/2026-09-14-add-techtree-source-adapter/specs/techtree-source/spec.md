## Purpose

Crawl TechTree (`jobs.techtree.dev`), a small multi-tenant AI-recruiting-agency job board,
into the catalogue by enumerating its own sitemap and reading each posting's own page —
rather than guessing at a listing or per-board query surface the site does not expose.

## ADDED Requirements

### Requirement: Sitemap-enumerated crawl

The system SHALL provide a `techtree` source adapter that discovers postings by reading
`https://jobs.techtree.dev/sitemap.xml` and following each `/job/<uuid>` URL it lists,
because the site exposes no other discovered listing or search endpoint.

#### Scenario: The sitemap yields the current posting set

- **WHEN** the adapter crawls
- **THEN** it fetches the sitemap and returns one candidate `Job` per `/job/<uuid>` entry
  found in it, ignoring non-job entries (e.g. `/`, `/talent-scout`, `/terms-of-service`)

#### Scenario: A malformed or unreadable sitemap fails the crawl loudly

- **WHEN** the sitemap cannot be fetched or parsed as XML
- **THEN** the crawl fails rather than silently reporting zero postings, so a broken feed is
  visible the same way an unparseable page fails loudly for `gulftalent`'s sitemap-index
  requirement

### Requirement: Self-contained aggregator company identity

The adapter MUST read each posting's employer from that posting's own page, because
TechTree is a multi-tenant aggregator where each posting names its own hiring company (e.g.
one sampled posting named "Telepatia") and the job URL itself carries no tenant slug. The
adapter is registered as an aggregator-marker source so its jobs are stored under their own
company identity, not a single TechTree-wide one.

#### Scenario: Company comes from the posting's own page

- **WHEN** a posting page states its hiring company
- **THEN** that company is the job's company

#### Scenario: Posting without a resolvable employer is dropped

- **WHEN** a posting page has no resolvable hiring-company name
- **THEN** the adapter omits that posting rather than persisting a company-less job

### Requirement: Stable dedup identity

The adapter SHALL derive each job's `ExternalID` from the job UUID in the posting's own URL
(`/job/<uuid>`), so re-crawling the same posting dedups to the same catalogue row and the
`?tp=` query parameter observed on every posting URL is never treated as part of the
posting's identity.

#### Scenario: Re-crawl dedups regardless of the tracking parameter

- **WHEN** the same posting is seen again, whether the URL carries no `tp` parameter, a
  different `tp` value, or the same one
- **THEN** it maps to the same `ExternalID` and updates the existing row rather than
  creating a duplicate

### Requirement: The full posting body, not the page's short summary

The adapter SHALL populate a job's description from the posting page's full rich-text body,
because the page's own structured summary (the same short text repeated in its meta
description and in its embedded `JobPosting.description` field) omits the substance a
candidate needs (e.g. "What you'll do", "What you bring") and would silently under-inform
every posting if used as the whole description.

#### Scenario: The description carries the full body, not the summary

- **WHEN** a posting page states both a short summary and a longer rich-text body
- **THEN** the job's description is built from the rich-text body, not the short summary

#### Scenario: A posting missing its rich-text body is dropped

- **WHEN** a posting page carries no locatable rich-text body
- **THEN** the adapter omits that posting rather than persisting one with only the short
  summary as its description

### Requirement: A crawl that reads nothing of what it listed must fail

The adapter MUST NOT report a successful, empty-yield run when the sitemap listed postings
but none of them could be read — the same posture `internal/ingest/sources/AGENTS.md`
documents for `echojobs`' 19-day silent outage: a source with nothing to offer and a source
that is blocked must not look identical to the pipeline.

#### Scenario: Every listed posting fails to read

- **WHEN** the sitemap lists one or more postings and every one of them fails to fetch or
  parse
- **THEN** the crawl run reports failure rather than exiting cleanly with zero postings
  ingested
