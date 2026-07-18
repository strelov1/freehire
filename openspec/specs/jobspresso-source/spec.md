# jobspresso-source Specification

## Purpose
TBD - created by archiving change add-jobspresso-source. Update Purpose after archive.
## Requirements
### Requirement: Jobspresso maps its RSS feed to jobs

The system SHALL provide a `jobspresso` source adapter that fetches the WordPress Job Manager
RSS feed at `https://jobspresso.co/feed/?post_type=job_listing` and maps each `<item>` to a
normalized `Job`. The `Job` SHALL carry the numeric `p=` post id from the item `<guid>` as its
`ExternalID` — falling back to the last path segment (slug) of the item `<link>` when no `p=` is
present — the item `<link>` as its canonical URL, the item `<title>` as its title, the company
and location split out of `<dc:creator>`, the sanitized inline HTML body from
`<content:encoded>` (falling back to `<description>`), and `<pubDate>` as `PostedAt`.

Jobspresso lists only remote work, so every mapped `Job` SHALL have `Remote: true` and work mode
`remote`.

The adapter is **boardless** (jobspresso.co is one site with no per-tenant board) and an
**aggregator** (one global feed whose company comes from each posting), so it stays in the source
facet. The configured board file entry carries no `board` value.

#### Scenario: An RSS item maps to a job

- **WHEN** the adapter fetches the feed and reads an `<item>` with a `<guid>` carrying `p=163363`,
  a `<link>`, a `<title>`, a `<dc:creator>` of `Acme<br>⚲&nbsp;Remote`, a `<content:encoded>`
  body, and a `<pubDate>`
- **THEN** it yields one `Job` with `ExternalID` `163363`, the item link as the canonical URL, the
  title, company `Acme`, location `Remote`, the sanitized `content:encoded` body, `Remote: true`
  with work mode `remote`, and `PostedAt` parsed from `pubDate`

#### Scenario: The full body is preferred over the summary

- **WHEN** an item carries both a truncated `<description>` summary and a fuller
  `<content:encoded>` body
- **THEN** the mapped `Job` description comes from `<content:encoded>`

### Requirement: Jobspresso splits company and location from dc:creator

The adapter SHALL parse `<dc:creator>`, whose text is of the form `Company<br>⚲&nbsp;Location`,
by splitting on the literal `<br>`: the left side is the company and the right side is the
location after stripping the `⚲` (U+26B2) marker and unescaping HTML entities. A `<dc:creator>`
with no `<br>` SHALL be treated as company-only with an empty location.

#### Scenario: Creator without a location marker yields company-only

- **WHEN** an item's `<dc:creator>` is `Acme` with no `<br>` separator
- **THEN** the mapped `Job` has company `Acme` and an empty location

### Requirement: Jobspresso drops unusable items

The adapter SHALL drop any `<item>` that yields no usable `ExternalID` (no `p=` and no link slug)
or whose company resolves empty — an empty dedup key or empty company would break the posting's
public slug. A single dropped item SHALL NOT abort the crawl.

#### Scenario: An item with an empty company is dropped

- **WHEN** an item's `<dc:creator>` resolves to an empty company
- **THEN** the adapter yields no `Job` for that item and continues mapping the rest

