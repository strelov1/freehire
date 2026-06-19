## ADDED Requirements

### Requirement: Curated collections are a company-level membership fact

The system SHALL model a curated collection as a company-level fact: each company
MAY belong to zero or more collections, stored as a set of collection slugs on the
company. A collection slug SHALL come from a fixed, code-owned registry; the v1
registry SHALL define exactly `yc` and `bigtech`. Each registry entry SHALL carry
a `slug`, a human `title`, a `description`, and a member resolver. Membership SHALL
NOT be derivable from a job's text or its ATS source — it is an editorial fact
about the company and is populated only from the registry's resolvers.

#### Scenario: A company belongs to multiple collections

- **WHEN** a company is a member of both the `yc` and `bigtech` collections
- **THEN** the company's collection set contains both `yc` and `bigtech`

#### Scenario: Only registry slugs are valid

- **WHEN** the collection registry is read
- **THEN** it lists exactly the defined collections (`yc`, `bigtech` in v1), each
  with a slug, title, description, and resolver

### Requirement: Collection membership is propagated onto jobs for the search facet

The system SHALL denormalize a company's collection set onto every job that
company owns, into a `jobs.collections` field, so that "jobs in a collection" is a
single-table/search filter with no join — mirroring `company_slug`. The
propagation SHALL set each job's `collections` to its company's `collections`
(matched by `company_slug`). A job whose company has no collections SHALL carry an
empty `collections` set. Propagation is a deterministic copy, distinct from
`jobderive` (which derives only from the job's own text).

#### Scenario: A tagged company's job carries the collection

- **WHEN** company `acme` is in collection `yc` and propagation runs
- **THEN** every job with `company_slug = acme` has `yc` in its `collections`

#### Scenario: An untagged company's job carries no collections

- **WHEN** a company has an empty collection set and propagation runs
- **THEN** its jobs carry an empty `collections` set

### Requirement: The import worker resolves and populates membership idempotently

The system SHALL provide a run-once-and-exit import worker that, for each
collection in the registry, resolves the member companies, writes
`companies.collections` for the tags it manages, and propagates the result onto
`jobs.collections`. The worker SHALL be idempotent and re-runnable (re-running
with the same inputs yields the same membership). The `yc` resolver SHALL match an
external YC company dataset onto existing companies by **normalized name** (using
the same normalization as company slugs); companies it cannot match SHALL be
omitted and logged, never guessed. The `bigtech` resolver SHALL use a hand-coded
slug list from the registry. The worker SHALL only modify the collection tags it
manages, leaving any other tags on a company untouched. After propagation the
worker SHALL signal that a search reindex is required.

#### Scenario: Re-running the import is idempotent

- **WHEN** the import worker runs twice with the same external dataset
- **THEN** the resulting `companies.collections` and `jobs.collections` are
  identical after each run

#### Scenario: Unmatched YC companies are omitted and logged

- **WHEN** a YC dataset entry has no company whose normalized name matches
- **THEN** no company is tagged for that entry and the unmatched entry is logged

#### Scenario: Big Tech membership comes from the hand list

- **WHEN** the `bigtech` resolver runs
- **THEN** exactly the companies whose slugs are in the registry's hand list are
  tagged `bigtech`

### Requirement: Collections are a job-search facet plus a discovery hub

The system SHALL expose `collections` as a selectable facet in the main job-search
filter sidebar (`/jobs`), rendering one option per registry collection, so a user
can filter the job feed by collection — composably with every other facet — and the
filter is reflected in the URL (`/jobs?collections=<slug>`). The system SHALL also
expose a discovery hub at `/collections` listing the registry's collections, each
with its title, description, and a count of its open jobs (read from the
`collections` search-facet distribution); each entry SHALL link into the filtered
job feed (`/jobs?collections=<slug>`). The hub's first render SHALL be
server-rendered. There SHALL NOT be a separate per-collection page — the facet on
`/jobs` is the single rendering of a collection's jobs.

#### Scenario: Collection is a facet on the job search

- **WHEN** a user opens `/jobs` and selects the `yc` collection in the sidebar
- **THEN** the URL carries `collections=yc` and the feed contains only open jobs
  whose `collections` include `yc`, composable with the other facets

#### Scenario: The hub lists collections with open-job counts

- **WHEN** a user opens `/collections`
- **THEN** the page lists `yc` and `bigtech`, each with its title, description, and
  the number of its open jobs, linking to `/jobs?collections=<slug>`
