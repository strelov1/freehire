## MODIFIED Requirements

### Requirement: The canonical job unions its cluster's geography

The search document for a canonical job SHALL carry the union of `countries`, `regions`,
and `cities` across every open row the canonical job REPRESENTS — its duplicate closure: the
rows whose `duplicate_of` chain terminates at it, at any depth — so a collapsed multi-city or
multi-country role remains findable by every city and country it is open in. Non-canonical
reposts remain excluded from the index.

The closure, not the shared `role_fingerprint`, SHALL define membership. A role cluster's
members point at their canon and are therefore inside its closure, so the exact pass's
behaviour is unchanged; a row suppressed by the fuzzy-description pass or by aggregator
suppression is inside the closure of the row that hides it, which a fingerprint key cannot
express because those passes by construction only ever act on rows with DIFFERING
fingerprints. A chain of markers (a role canon that is itself suppressed by a later pass)
SHALL resolve to its ultimate owner, so no member's geography is stranded on an unsearchable
intermediate row.

Traversal SHALL start from searchable rows (open, `duplicate_of IS NULL`) and walk toward
their members, and SHALL be bounded in depth. Every reader of the closure — the whole-catalogue
one and the by-id-set one alike — SHALL seed this way. A cycle among duplicate markers is
therefore unreachable and cannot be traversed; the depth bound is the backstop, not the
correctness argument.

The walk SHALL follow OPEN rows only, so a closed intermediate ends that branch: an open row
behind a closed parent contributes to no owner. This is deliberate and costs nothing, because
such a row carries a marker and is out of the index regardless, as is the closed row it points
at. Re-pointing it belongs to the marker refresh — the role recompute chooses `min(id)` among a
cluster's OPEN rows, and the fuzzy pass releases a marker whose canon closed.

This binds every writer of the document, not only the full reindex. Because the incremental
push is a field-level document update and the three geography facets are always present in
the pushed payload, a writer that omits the union does not merely fail to widen the canon —
it replaces the widened values with the canon's own, undoing the rebuild.

A writer MAY skip the union when the row represents no other open row, since the union is
then the row's own geography and the merge is a no-op.

#### Scenario: A collapsed multi-country role is found by any of its countries

- **WHEN** a role cluster is open in several countries and collapses to one canon
  in country A, and the search is filtered by country B (a non-canon row's
  country)
- **THEN** the canonical job is returned

#### Scenario: The canon lists every city of its cluster

- **WHEN** the canonical job of a multi-city cluster is indexed by a full reindex
- **THEN** its `cities` facet contains every open city in the cluster

#### Scenario: A fuzzy-suppressed posting's city survives on the fuzzy canon

- **WHEN** a posting in city B is suppressed by the fuzzy-description pass onto a canonical
  posting in city A, and the search is filtered by city B
- **THEN** the fuzzy canon is returned, because its `cities` facet carries B

#### Scenario: A chained marker does not strand geography

- **WHEN** a posting in city C is marked `duplicate_of` a role canon in city B, and that role
  canon is itself later suppressed onto a fuzzy canon in city A
- **THEN** the fuzzy canon's `cities` facet carries A, B and C, and a search filtered by C
  returns it

#### Scenario: A closed intermediate ends the branch

- **WHEN** an open posting is marked `duplicate_of` a row that has since been CLOSED, and that
  closed row is itself marked onto an open canonical posting
- **THEN** the open posting's cities do not reach that canonical posting, and a canonical row
  left with only closed members carries no widening at all

#### Scenario: An incremental push does not narrow the canon

- **WHEN** a canonical job in a multi-city cluster is pushed to the index by the ingest
  write path, a link import, or an incremental embed — rather than by a full reindex
- **THEN** the pushed document still carries the union across its closure, so the role stays
  findable by the cities the rows it represents hold

#### Scenario: A row representing nobody costs no extra query

- **WHEN** an open canonical job has no open row pointing at it
- **THEN** the writer skips the closure-geography lookup, and the document carries the job's
  own geography unchanged
