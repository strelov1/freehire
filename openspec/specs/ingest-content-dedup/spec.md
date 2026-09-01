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

### Requirement: A repost is marked when it is first written

The ingest write path SHALL determine, in the same transaction that persists a newly inserted
posting, whether the posting's role cluster already holds an open canonical row older than it,
and SHALL mark the new row `duplicate_of` that canon when one exists. The canon consulted
SHALL be the one the periodic recompute would choose (the `min(id)` among the cluster's open
rows), so the two never disagree; a candidate canon that is NEWER than the row being written
SHALL be ignored rather than marked onto. A row so marked SHALL be excluded from the live
search index and from the enrichment enqueue by the same rules that already govern a row
marked by the recompute.

The determination SHALL be limited to newly inserted rows. A posting that becomes a duplicate
only later — because an edit made its title and description match a sibling's — and the
release of a row whose canon has closed both remain the periodic recompute's responsibility. A
failed lookup SHALL leave the row unmarked rather than fail the write, since deduplication is
an improvement on the write and never a condition of keeping the vacancy.

#### Scenario: A per-city fan-out collapses as it is ingested

- **WHEN** one crawl writes several postings of the same role that differ only by location, so
  they share a `role_fingerprint`
- **THEN** the oldest is canonical, every later copy carries `duplicate_of` pointing at it, and
  only the canonical row is pushed to the live search index

#### Scenario: A fresh repost is invisible before the next recompute

- **WHEN** a subscription digest or the jobs list is served after such a crawl but before the
  next batch recompute runs
- **THEN** the role appears once, not once per copy

#### Scenario: A newly written repost is not enriched

- **WHEN** a posting is marked `duplicate_of` as it is written
- **THEN** it is not enqueued for enrichment, so an invisible row costs no LLM call

#### Scenario: A re-crawled posting is not re-examined

- **WHEN** an existing posting is written again by a later crawl, whether unchanged or edited
- **THEN** no canon lookup is made for it and its existing marker stands, leaving the periodic
  recompute to revise it

