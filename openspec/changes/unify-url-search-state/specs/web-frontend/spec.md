# web-frontend Specification (delta)

## MODIFIED Requirements

### Requirement: Jobs list with pagination

The frontend SHALL present a list of jobs from `GET /api/v1/jobs`, showing for
each job its title, company, location, work arrangement (and, for remote roles,
its reach), source, and posted date, and SHALL paginate using the API's
`limit`/`offset` driven by `meta.total`. The work arrangement SHALL be derived
from `enrichment.work_mode`; for a remote role the reach SHALL be shown from
`enrichment.regions` (e.g. `Global`, `Europe`). The frontend SHALL NOT rely on a
raw `remote` field (it is no longer in the API). The first page of the list
SHALL be **server-rendered** — its rows present in the initial HTML — and then
hydrate on the client for subsequent interaction.

When more jobs remain, the frontend SHALL load the next page automatically once
the user scrolls to the bottom of the current list (infinite scroll), without
pre-fetching ahead of the viewport. An accessible control SHALL remain available
as a fallback so that keyboard and screen-reader users — who do not
scroll-trigger — can load the next page, and so that a failed page load can be
retried.

#### Scenario: Jobs are listed

- **WHEN** a user opens the jobs route `/jobs`
- **THEN** the server returns HTML already containing the first page of job rows,
  each linking to its job detail

#### Scenario: User reaches the bottom of the list

- **WHEN** more jobs exist than the current page (`offset + limit < meta.total`)
  and the user scrolls to the bottom of the loaded rows
- **THEN** the next page is fetched and appended automatically, without the user
  clicking a control

#### Scenario: Fallback control loads and retries

- **WHEN** more jobs remain but the user navigates by keyboard or a page load
  failed
- **THEN** an accessible control is present that fetches the next page on
  activation, and a failed load surfaces a retry affordance rather than silently
  stopping

#### Scenario: A global-remote job shows its reach explicitly

- **WHEN** a listed job has `work_mode=remote` and `regions=[global]`
- **THEN** its row shows a "Global" reach indicator rather than a bare "Remote"
  with no reach

## ADDED Requirements

### Requirement: Responsive URL-synced search and filter input

The frontend's search and filter inputs SHALL stay responsive while the user
types — across the jobs search box and facet filters, the companies name search,
and the analytics filters — so that characters are never dropped or reverted
during fast typing, even while a data reload triggered by an earlier keystroke is
still in flight. The current search/filter state SHALL be mirrored into the URL
query string so it survives reload, sharing, and browser back/forward, and the
data reload that reflects a change SHALL be debounced so that typing does not
issue one request per character.

#### Scenario: Fast typing keeps every character

- **WHEN** a user types into a search/filter input faster than the reload
  debounce window, and an earlier reload is still in flight
- **THEN** the input retains exactly what was typed — no characters are dropped or
  reverted by the in-flight reload completing

#### Scenario: Filter state survives back/forward

- **WHEN** a user changes filters, navigates away, and returns via browser
  back/forward
- **THEN** the inputs and results are restored to match the URL of that history
  entry

#### Scenario: Typing debounces the reload

- **WHEN** a user types a multi-character query in quick succession
- **THEN** the URL updates to reflect the query but the data reload is coalesced
  rather than issued once per keystroke
