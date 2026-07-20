# ingest-content-dedup Specification

## Purpose
TBD - created by archiving change ingest-content-dedup. Update Purpose after archive.
## Requirements
### Requirement: One canonical job per role cluster

The system SHALL designate exactly one **canonical** open job per role cluster
(`company_slug` + `role_fingerprint`), reusing the existing `role_fingerprint`. The
canonical row is chosen deterministically — the `min(id)` among the cluster's open
rows — so the choice is stable across recomputes. Non-canonical open rows in a cluster
carry a `duplicate_of` reference to their canonical job; the canonical row and any
row with an empty fingerprint carry no reference. Rows are never deleted or
un-inserted, so the job-reality repost/mass-posting counts are unaffected.

#### Scenario: A cluster resolves to one canonical row

- **WHEN** a company has several open jobs sharing one `role_fingerprint`
- **THEN** exactly one (the `min(id)`) is canonical and the rest reference it via
  `duplicate_of`

#### Scenario: Unfingerprinted and singleton rows stay canonical

- **WHEN** an open job has an empty `role_fingerprint`, or is the only open row in its
  cluster
- **THEN** it is canonical (`duplicate_of` is null)

#### Scenario: Canon fails over when the canonical closes

- **WHEN** the canonical row of a cluster is closed and the recompute runs
- **THEN** the new `min(id)` among the remaining open rows becomes canonical

### Requirement: Non-canonical reposts are hidden from catalogue and search

The jobs list and the search index SHALL exclude non-canonical reposts, so a role
cluster appears once. A job addressed directly by its slug is still served (like a
closed job) so existing links do not break.

#### Scenario: List returns one row per cluster

- **WHEN** the jobs list is queried and a cluster has a canonical row plus reposts
- **THEN** only the canonical row is returned

#### Scenario: Search omits reposts

- **WHEN** the search index is built or incrementally pushed
- **THEN** rows with a non-null `duplicate_of` are not indexed

#### Scenario: A repost is still reachable by slug

- **WHEN** a non-canonical repost is requested by its public slug
- **THEN** the detail read still serves it

### Requirement: Enrichment skips non-canonical reposts

The enrichment enqueue SHALL exclude non-canonical reposts, so duplicate postings do
not consume LLM budget; only the canonical row of a cluster is enriched.

#### Scenario: Only the canonical row is enqueued

- **WHEN** the enrichment enqueue runs over open jobs
- **THEN** a job with a non-null `duplicate_of` is not enqueued

### Requirement: The canonical job surfaces its openings count

The canonical job SHALL surface how many open postings its cluster holds (the existing
role-cluster open count), so a collapsed cluster communicates "N openings" rather than
hiding that N postings exist.

#### Scenario: Canon reports its cluster's open count

- **WHEN** the canonical job of a cluster with N open postings is read
- **THEN** its openings count is N

### Requirement: The role fingerprint ignores a location-bearing title suffix

The `role_fingerprint` that keys a role cluster SHALL normalize the title by
stripping a single trailing separator clause — the text after the last ` , `,
` | `, ` @ `, or space-delimited ` - ` / ` — ` / ` – ` — before hashing, so a role
whose only difference is a city (or other qualifier) appended to the title
resolves to the same fingerprint as its siblings. The strip SHALL remove only a
trailing clause (never a prefix, so a seniority grade like `Senior …` is
preserved) and SHALL leave the title unchanged when stripping would drop it below
two words. The description SHALL remain part of the fingerprint, so two postings
collapse only when both the stripped title AND the description match.

#### Scenario: Per-city title variants share one fingerprint

- **WHEN** a company posts one role in several cities and each posting appends the
  city to the title (e.g. `"… Engineer, Krakau"`, `"… Engineer, Wien"`) with an
  identical description
- **THEN** all the postings resolve to the same `role_fingerprint` and collapse to
  one canonical card

#### Scenario: Distinct roles with different descriptions stay separate

- **WHEN** two postings share a stripped title but carry different descriptions
  (e.g. two engineering specialties)
- **THEN** they resolve to different `role_fingerprint`s and are not collapsed

#### Scenario: A seniority prefix is never stripped

- **WHEN** a title carries a leading grade (e.g. `"Senior Software Engineer"`)
- **THEN** the grade is retained in the fingerprint, so it does not collapse into
  the ungraded role

### Requirement: The canonical job unions its cluster's geography

When the search document for a canonical job is built by a full reindex, it SHALL
carry the union of `countries`, `regions`, and `cities` across all open rows of
its role cluster (not only the canon's own row), so a collapsed multi-city or
multi-country role remains findable by every city and country it is open in.
Non-canonical reposts remain excluded from the index.

#### Scenario: A collapsed multi-country role is found by any of its countries

- **WHEN** a role cluster is open in several countries and collapses to one canon
  in country A, and the search is filtered by country B (a non-canon row's
  country)
- **THEN** the canonical job is returned

#### Scenario: The canon lists every city of its cluster

- **WHEN** the canonical job of a multi-city cluster is indexed by a full reindex
- **THEN** its `cities` facet contains every open city in the cluster

