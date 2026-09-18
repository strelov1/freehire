## ADDED Requirements

### Requirement: Per-role skill demand

The system SHALL expose the skill distribution *within* a single role, where a role is the
pair (category, seniority). Naming a single role on the existing public, unauthenticated
`GET /api/v1/insights/roles` endpoint — by supplying both `category` and `seniority` —
SHALL return, in addition to that role's `open_count` and `growth`, the canonical skills
that role's currently-open postings carry, each with the count of those postings and that
count as a share, ordered by count descending.

The share's denominator SHALL be the number of the role's open postings that carry AT
LEAST ONE tagged skill, never the role's whole open count, and that denominator SHALL be
served beside the distribution as `sample_size`. Measured 2026-09-18 on production, 11% of
the eligible postings (392,020 → 348,060) carry no tagged skill at all; dividing by the
whole open count would deflate every share by our own tagging gap, and because that gap
differs per role it would make two roles' shares incomparable — which is the one
comparison the surface exists to support.

The role's open count and its `sample_size` are therefore different numbers, and a surface
SHALL render the share against `sample_size`, never against `open_count`.

The distribution SHALL be served from a precomputed rollup, never counted on the request
path, and SHALL be recomputed by the same worker and in the same atomic
delete-and-reinsert transaction as the sibling insights rollups. A skill whose count
within the role falls below the worker's sample floor SHALL be omitted, so a share is
never published off a sample too small to mean anything.

The distribution measures skills the postings CARRY, which is what the `jobs.skills` facet
records — a skill named anywhere in a posting, including in a "nice to have" list or a
stack description. It is therefore an upper bound on what the role requires, and every
surface rendering it SHALL say "mentioned in", never "required by".

The distribution describes the postings that STATE a seniority, not the role's market.
Measured 2026-09-18 on production, only 39.0% of open `is_tech` postings carry a
non-empty `seniority` (394,610 of 1,012,085), against 98.7% for `category` — and the
missing 61% are not a random sample, being exactly the postings whose title names no
level. Every surface rendering a single role's distribution SHALL therefore state the
`sample_size` it was measured over, and SHALL NOT claim to describe the role's market.

The rollup SHALL NOT be scoped by geography. `GET /api/v1/insights/roles` continues to
accept `country`, and when a country is supplied alongside a single role the response
SHALL carry the country-scoped `open_count` and `growth` as before, together with the
country-agnostic skill distribution, and `meta` SHALL state that the skill distribution is
not geography-scoped.

#### Scenario: Skills for one named role

- **WHEN** a client requests `GET /api/v1/insights/roles?category=backend&seniority=senior`
- **THEN** the response is `200` and `data` carries that single role with `category`,
  `seniority`, `open_count`, `growth`, and a `skills` array whose entries carry `skill`,
  `open_count` and `share`, ordered by `open_count` descending

#### Scenario: Share is relative to the skill-bearing sample, not the open count

- **WHEN** a role has 1,000 open postings, 900 of which carry at least one tagged skill,
  and 710 of those carry `docker`
- **THEN** `sample_size` is `900` and that skill's `share` is `710/900`, never `710/1000`

#### Scenario: Sample size is served beside the distribution

- **WHEN** a client requests a single role's skill demand
- **THEN** the response carries `sample_size` alongside `open_count`, and the two are
  distinct fields because they count different things

#### Scenario: Seniority without category is refused

- **WHEN** a client requests `GET /api/v1/insights/roles?seniority=senior` with no
  `category`
- **THEN** the response is `400` with an `{"error": ...}` body, because a seniority across
  every category is not a role

#### Scenario: Invalid seniority rejected

- **WHEN** a client requests
  `GET /api/v1/insights/roles?category=backend&seniority=archmage`
- **THEN** the response is `400` with an `{"error": ...}` body and no partial data

#### Scenario: Seniority is a read parameter, not an ignored one

- **WHEN** a client supplies `seniority`
- **THEN** it never appears in `meta.ignored_params`, because the endpoint reads it

#### Scenario: Ranking is unchanged when no single role is named

- **WHEN** a client requests `GET /api/v1/insights/roles` with no `seniority`
- **THEN** the response is the existing ranked list of roles, and no entry carries a
  `skills` array

#### Scenario: Role below the sample floor carries no skills

- **WHEN** a named role's postings are too few for any skill to clear the worker's sample
  floor
- **THEN** the response is `200` with that role's counts and an empty `skills` array,
  never a `404` and never an unfloored distribution

#### Scenario: Country scopes the counts but not the skills

- **WHEN** a client requests
  `GET /api/v1/insights/roles?category=backend&seniority=senior&country=DE`
- **THEN** `open_count` and `growth` are scoped to `DE`, the `skills` array is the
  country-agnostic distribution, and `meta` states that the skill distribution is not
  geography-scoped
