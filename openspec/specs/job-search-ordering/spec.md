# job-search-ordering Specification

## Purpose
How the jobs feed is ordered: newest-added first by default, or by profile match. Split out of job-search.

## Requirements

### Requirement: Default ordering is newest-added first

A search request with no query text and no valid `sort` parameter SHALL return
jobs ordered by the source's posting date (`posted_at`), newest first. A request
with query text and no `sort` SHALL keep relevance order. An explicit valid
`sort` parameter SHALL always take precedence. `posted_at`, `created_at` and
`view_count` SHALL all be sortable attributes of the index and accepted `sort`
values. The DB-backed jobs list keeps its own stable default (`created_at`
descending) and is no longer required to match the search default — it accepts no
`sort` parameter at all, so every ordering the feed offers is served by the search
endpoint.

`sort=view_count` orders by how many people have opened each posting. Like the
other attribute sorts it honours `order` and defaults to descending, so the
common case — the most-viewed postings first — needs no second parameter.

`sort=match` is the one accepted value that is NOT an index attribute: it orders
by the caller's own skill vector (see `skill-match-sort`). When it is served, the
request SHALL carry no attribute sort directive at all, because an explicit sort
directive takes precedence over vector ranking in the engine and would silently
discard the ordering the caller asked for.

#### Scenario: Browsing without a query shows freshest postings first

- **WHEN** the search endpoint is called with empty `q` and no `sort`
- **THEN** results are ordered `posted_at` descending

#### Scenario: A text query keeps relevance order

- **WHEN** the search endpoint is called with `q=golang` and no `sort`
- **THEN** results are in relevance order (no sort directive)

#### Scenario: Explicit sort wins

- **WHEN** the search endpoint is called with `sort=created_at&order=desc`
- **THEN** results are ordered by `created_at` descending regardless of `q`

#### Scenario: Most-viewed ordering defaults to descending

- **WHEN** the search endpoint is called with `sort=view_count` and no `order`
- **THEN** results are ordered by `view_count` descending

#### Scenario: Most-viewed ordering honours an ascending order

- **WHEN** the search endpoint is called with `sort=view_count&order=asc`
- **THEN** results are ordered by `view_count` ascending

#### Scenario: A served match sort suppresses the attribute sort

- **WHEN** the search endpoint serves `sort=match` for an eligible caller
- **THEN** the engine request carries the caller's vector and no sort directive

### Requirement: The jobs feed can be ordered by profile match

The system SHALL accept `sort=match` on `GET /api/v1/jobs/search`, ordering
results by the cosine between each job's skill vector and the authenticated
caller's, computed from their profile skills.

The endpoint SHALL remain public. It SHALL attach the caller's identity when a
session is present but MUST NOT reject a request that lacks one.

Ranking SHALL compose with every existing facet filter in a single engine query,
and pagination SHALL behave as it does for any other ordering — there is no
window, and no ceiling beyond the endpoint's existing pagination guard.

The caller's vector SHALL be built per request from their profile alone. It SHALL
require no database read: the vector depends on nothing but the skills the profile
names.

**`sort=match` SHALL NEVER return an error.** Every reason it cannot be served —
no session, no profile, no skills, no recognised skills —
SHALL degrade to the endpoint's default ordering. A saved search or a shared link
carrying `sort=match` must not break when opened by someone it cannot be served
for, which is the same reason the jobs list ignores unknown filters rather than
refusing them.

#### Scenario: A signed-in caller with skills is ranked by match

- **WHEN** a caller with a session and a profile naming recognised skills requests
  `sort=match`
- **THEN** the engine query carries a vector of the declared width and results are
  ordered by their cosine against it

#### Scenario: An anonymous caller gets the default feed

- **WHEN** a caller with no session requests `sort=match`
- **THEN** the response is `200` with the default freshest-first ordering, and the
  engine query carries no vector

#### Scenario: A profile with no skills gets the default feed

- **WHEN** a signed-in caller whose profile names no skills requests `sort=match`
- **THEN** the response is `200` with the default ordering and no vector

#### Scenario: The match sort composes with facet filters

- **WHEN** an eligible caller requests `sort=match` together with country and
  seniority filters
- **THEN** one engine query carries both the vector and the facet filter, and
  neither is dropped

#### Scenario: Deep pagination under the match sort behaves normally

- **WHEN** an eligible caller pages into the match-ordered feed within the
  endpoint's pagination guard
- **THEN** results continue in match order, with no separate depth limit
