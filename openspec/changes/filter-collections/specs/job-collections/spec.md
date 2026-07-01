## ADDED Requirements

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

## MODIFIED Requirements

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
