## ADDED Requirements

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
