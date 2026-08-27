## ADDED Requirements

### Requirement: Permanent skill vector positions

The system SHALL assign every canonical skill slug a position in a fixed-width
vector, and that position SHALL be permanent: once assigned it MUST NOT be
reused, reordered, or removed.

The assignment SHALL live in a generated, committed registry whose generator only
appends. The declared vector width (`Dimensions`) SHALL exceed the number of
assigned positions, so a growing dictionary does not force a re-declaration.

A skill slug the registry does not name SHALL contribute nothing to any vector.
It MUST NOT be guessed at, hashed into an arbitrary position, or cause an error.

Rationale for the permanence rule: a stored vector records values BY POSITION.
Shifting one position invalidates every vector already written to the search
index, with no error raised anywhere — the feed simply begins ranking wrongly.

#### Scenario: The registry covers the whole dictionary

- **WHEN** the registry is checked against the skill dictionary's canonical slugs
- **THEN** every canonical slug has a position, and a slug without one fails the
  build rather than silently ranking as absent

#### Scenario: A new dictionary skill is appended, not inserted

- **WHEN** a skill is added to the dictionary and the registry is regenerated
- **THEN** the new skill takes the next free position and every previously
  assigned position is unchanged

#### Scenario: The registry cannot outgrow the declared width

- **WHEN** the number of assigned positions would exceed `Dimensions`
- **THEN** the build fails, because widening `Dimensions` requires a full index
  rebuild and MUST NOT happen implicitly

#### Scenario: An unknown slug is ignored

- **WHEN** a vector is built from a skill set containing a slug with no assigned
  position
- **THEN** the resulting vector is identical to one built from the same set with
  that slug removed

### Requirement: Rare skills weigh more than common ones

The system SHALL weight each skill position by how rare that skill is in the open
catalogue, so that an overlap on a widely-required skill contributes less than an
overlap on an uncommon one.

The weights SHALL be derived from the existing facet-distribution snapshot
(`insights_facet_stats`, facet `skills`), which records how many open jobs name
each skill. No new counting, worker, or schema is required.

A skill present in the dictionary but absent from the snapshot SHALL be treated
as maximally rare rather than as weightless: it is either newly added or genuinely
obscure, and both warrant weight.

Weights are expected to drift as the catalogue changes. Unlike a position, a stale
weight SHALL be treated as acceptable: it nudges the ordering and cannot corrupt a
stored vector.

#### Scenario: A rare skill outranks a common one

- **WHEN** two vacancies each overlap the candidate on exactly one skill, one of
  which many open jobs name and one of which few do
- **THEN** the vacancy overlapping on the rarer skill ranks higher

#### Scenario: No snapshot yields no vectors

- **WHEN** the facet snapshot holds no `skills` rows
- **THEN** weight loading succeeds and produces weights that build no vectors,
  rather than failing indexing or producing an unweighted ranking

#### Scenario: Unloadable weights leave stored vectors alone

- **WHEN** a document is built while no weights could be loaded
- **THEN** it omits the vector field entirely rather than clearing it, so an index
  already carrying vectors keeps them — an absence of knowledge is not knowledge of
  an absence

### Requirement: Vector ordering rewards overlap and coverage together

The system SHALL build unit-length vectors, so that the cosine between a
vacancy's and a candidate's expresses both how many of the candidate's skills the
vacancy engages and what share of the vacancy's requirements they cover.

Neither half SHALL be sufficient alone, **at equal skill rarity**: among vacancies
whose skills carry comparable weight, one naming a single skill the candidate holds
MUST NOT outrank one that engages many of them, and one listing a large
undifferentiated set MUST NOT outrank a well-targeted one on volume.

The rarity qualifier is not a hedge, it is the design. A vacancy asking only for a
scarce skill the candidate happens to hold CAN outrank a broader match on common
ones, and SHOULD: that is what weighting by rarity means. The system SHALL NOT cap
or flatten the weights to force a count-based order.

A vector SHALL be absent — not zero-valued — when it would be meaningless: no
weights available, no skills given, or no skill recognised. An absent vector is an
omission the caller propagates, never a vector that ranks against everything.

The two absences SHALL be distinguished where they reach the index. A job whose
skills are gone SHALL have any stored vector CLEARED, since the index merges rather
than replaces documents and an omission would leave it ranking by skills it no longer
has. A job built without loaded weights SHALL leave the stored vector untouched.

#### Scenario: A well-targeted vacancy outranks both extremes

- **WHEN** a candidate holding five skills of comparable rarity is ranked against a
  vacancy naming only one of them, a vacancy naming exactly those five, and a
  vacancy naming those five among twelve
- **THEN** the vacancy naming exactly the five ranks above the one-skill vacancy,
  and the one-skill vacancy ranks last

#### Scenario: A scarce skill can outweigh a larger common overlap

- **WHEN** one vacancy asks only for a skill few open jobs name, which the candidate
  holds, and another asks for several skills that nearly every job names
- **THEN** the scarce-skill vacancy MAY rank higher, and this is correct rather than
  a violation of the ordering above

#### Scenario: A skill listed twice does not count twice

- **WHEN** a vector is built from a skill set naming the same slug more than once
- **THEN** the vector is identical to one built from the deduplicated set

#### Scenario: An unusable input produces no vector

- **WHEN** a vector is requested with no weights loaded, or from an empty skill
  set, or from a skill set no slug of which is recognised
- **THEN** no vector is produced, and the caller omits it rather than sending a
  zero vector

### Requirement: The vacancy's vector is derived at index time

Each indexed job document SHALL carry its skill vector, derived when the document
is built from the job row — never fetched, inferred, or computed at query time.

The derivation SHALL NOT call an embedding model, an LLM, or any network service.
Skills are canonical slugs from a finite dictionary, so the vector is arithmetic.

Every code path that builds an index document SHALL supply the weights. The
weights SHALL be a parameter of document construction rather than a field a caller
may attach afterwards, so that a path which omits them fails to compile.

A document whose weights are loaded but whose skills yield no vector SHALL carry an
explicit CLEARING value, not an omission: the index merges document fields rather than
replacing them, so an omitted field leaves a previously stored vector in place. Only a
document built without loaded weights SHALL omit the field.

#### Scenario: An indexed job carries its vector

- **WHEN** a job whose skills are recognised is turned into an index document
- **THEN** the document carries a vector of the declared width under the skill
  embedder's name

#### Scenario: A job that loses its skills has its stored vector cleared

- **WHEN** a job whose skills are all unrecognised, or which has none, is turned into
  an index document while the weights are loaded
- **THEN** the document carries an explicit clearing value, so pushing it removes any
  vector the index already held for that job rather than merging around it

#### Scenario: An indexer that omits the weights does not build

- **WHEN** a code path constructs an index document without supplying weights
- **THEN** compilation fails, rather than the path silently producing documents
  that drop out of the match ordering
