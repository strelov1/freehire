## MODIFIED Requirements

### Requirement: The canonical job unions its cluster's geography

The search document for a canonical job SHALL carry the union of `countries`, `regions`,
and `cities` across all open rows of its role cluster (not only the canon's own row), so a
collapsed multi-city or multi-country role remains findable by every city and country it is
open in. Non-canonical reposts remain excluded from the index.

This binds every writer of the document, not only the full reindex. Because the incremental
push is a field-level document update and the three geography facets are always present in
the pushed payload, a writer that omits the union does not merely fail to widen the canon —
it replaces the widened values with the canon's own, undoing the rebuild.

A writer MAY skip the union when the cluster has at most one open row, since the union is
then the canon's own geography and the merge is a no-op.

#### Scenario: A collapsed multi-country role is found by any of its countries

- **WHEN** a role cluster is open in several countries and collapses to one canon
  in country A, and the search is filtered by country B (a non-canon row's
  country)
- **THEN** the canonical job is returned

#### Scenario: The canon lists every city of its cluster

- **WHEN** the canonical job of a multi-city cluster is indexed by a full reindex
- **THEN** its `cities` facet contains every open city in the cluster

#### Scenario: An incremental push does not narrow the canon

- **WHEN** a canonical job in a multi-city cluster is pushed to the index by the ingest
  write path, a link import, or an incremental embed — rather than by a full reindex
- **THEN** the pushed document still carries the cluster's union, so the role stays findable
  by the cities its reposts hold

#### Scenario: A singleton cluster costs no extra query

- **WHEN** a job's role cluster has at most one open row
- **THEN** the writer skips the cluster-geography lookup, and the document carries the job's
  own geography unchanged
