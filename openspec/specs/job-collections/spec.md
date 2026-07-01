# job-collections Specification

## Purpose
TBD - created by archiving change add-collections-pages. Update Purpose after archive.
## Requirements
### Requirement: Curated collections are a company-level membership fact

The system SHALL model a curated collection as a company-level fact: each company
MAY belong to zero or more collections, stored as a set of collection slugs on the
company. A collection slug SHALL come from a fixed, code-owned registry. Each
registry entry SHALL carry a `slug`, a human `title`, a `description`, and a
membership source — exactly one of a static hand list of canonical company slugs
or a remote dataset (a URL plus a parser that yields company names). Adding a
collection SHALL be a single registry entry. Membership SHALL NOT be derivable
from a job's text or its ATS source — it is an editorial fact about the company,
populated only from the registry's sources.

#### Scenario: A company belongs to multiple collections

- **WHEN** a company qualifies for two collections (e.g. `yc` and `bigtech`)
- **THEN** the company's collection set contains both slugs

#### Scenario: The registry defines each collection's display copy and source

- **WHEN** the collection registry is read
- **THEN** each entry exposes a slug, title, description, and exactly one
  membership source (a static slug list or a dataset)

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
collection in the registry, resolves its member companies — a dataset collection
is fetched and parsed to company names, a static-list collection uses its slugs —
matches them onto existing companies by **normalized name** (the same
normalization as company slugs; unmatched candidates are omitted and logged, never
guessed), writes `companies.collections` for the tags it manages, and propagates
the result onto `jobs.collections`. The worker SHALL be idempotent and re-runnable
(re-running with the same inputs yields the same membership) and SHALL only modify
the collection tags it manages, leaving any other tags on a company untouched. If
any collection's source cannot be resolved (e.g. a dataset fetch fails) the worker
SHALL abort before writing — a partial resolve would reconcile a collection's tag
off every company. After propagation the worker SHALL signal that a search reindex
is required.

#### Scenario: Re-running the import is idempotent

- **WHEN** the import worker runs twice with the same inputs
- **THEN** the resulting `companies.collections` and `jobs.collections` are
  identical after each run

#### Scenario: Unmatched dataset companies are omitted and logged

- **WHEN** a dataset entry has no company whose normalized name matches
- **THEN** no company is tagged for that entry and the unmatched count is logged

#### Scenario: A failed dataset resolve aborts before writing

- **WHEN** a collection's dataset cannot be fetched or parsed
- **THEN** the worker aborts without writing any membership (no collection is
  reconciled off existing companies)

#### Scenario: Static-list membership comes from the hand list

- **WHEN** a static-list collection (e.g. `bigtech`) is resolved
- **THEN** exactly the existing companies whose slugs are in the registry's hand
  list are tagged with that collection

### Requirement: Collections are a job-search facet plus a discovery hub

The system SHALL expose `collections` as a selectable facet in the main job-search
filter sidebar (`/jobs`), rendering one option per **company-collection** registry
entry, so a user can filter the job feed by collection — composably with every
other facet — and the filter is reflected in the URL (`/jobs?collections=<slug>`).
The system SHALL also expose a discovery hub at `/collections` listing **both**
kinds of collection — company collections and filter collections — as visually
uniform cards, each with its title, description, and a count of its open jobs. A
company-collection card's count SHALL come from the `collections` search-facet
distribution and it SHALL link to `/jobs?collections=<slug>`; a filter-collection
card's count SHALL come from a job-search total for its filter `params` and it
SHALL link to `/jobs?<query>` built from those params. Counts are decorative: a
failed count fetch SHALL degrade to no count rather than failing the page. The
hub's first render SHALL be server-rendered. There SHALL NOT be a separate
per-collection page — the `/jobs` feed is the single rendering of a collection's
jobs, for both kinds.

#### Scenario: Collection is a facet on the job search

- **WHEN** a user opens `/jobs` and selects the `yc` collection in the sidebar
- **THEN** the URL carries `collections=yc` and the feed contains only open jobs
  whose `collections` include `yc`, composable with the other facets

#### Scenario: The hub lists company collections with open-job counts

- **WHEN** a user opens `/collections`
- **THEN** the page lists `yc` and `bigtech`, each with its title, description, and
  the number of its open jobs, linking to `/jobs?collections=<slug>`

#### Scenario: The hub lists filter collections linking to a filtered feed

- **WHEN** a user opens `/collections`
- **THEN** the page lists the `remote-worldwide` filter collection with its title,
  description, and open-job count, linking to `/jobs?work_mode=remote&regions=global`

#### Scenario: A failed count fetch does not break the hub

- **WHEN** a collection's open-job count cannot be fetched
- **THEN** the hub still renders that collection's card, without a count

### Requirement: Filter collections map a curated card to an arbitrary job-search filter

The system SHALL support a second kind of collection — a **filter collection** —
whose membership is an arbitrary job-search filter rather than company membership.
A filter collection SHALL be defined entirely in the frontend (no Go registry
entry, no company/job membership, no `collections` facet value, no database or API
change) as a data entry carrying a `slug`, a human `title`, a `description`, and a
`params` map of job-search facet params (the same param names the `/jobs` feed
accepts, e.g. `work_mode`, `regions`). A param value MAY be a single string or a
list; a list SHALL expand into repeated query keys (OR semantics), matching the
`/jobs` filter contract. The `params` map SHALL be the single source from which
both the card's link and its open-job count are built. Adding a filter collection
SHALL be a single data entry. The registry SHALL seed one filter collection,
`remote-worldwide`, defined as `work_mode=remote` and `regions=global`.

#### Scenario: A filter collection maps a slug to filter params

- **WHEN** the `remote-worldwide` filter collection is read
- **THEN** it exposes a slug, title, description, and a `params` map equal to
  `{ work_mode: "remote", regions: "global" }`

#### Scenario: A list param expands into repeated query keys

- **WHEN** a filter collection's `params` maps a key to a list of two values
- **THEN** building its query yields that key repeated once per value (OR
  semantics), matching the `/jobs` filter contract

