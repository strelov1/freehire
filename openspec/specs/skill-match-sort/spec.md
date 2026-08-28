# skill-match-sort Specification

## Purpose
TBD - created by archiving change skill-match-sort. Update Purpose after archive.
## Requirements
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

### Requirement: Every recognised skill counts the same

The system SHALL weight every recognised skill equally. It MUST NOT scale a skill's
contribution by how rare it is in the catalogue.

This was tried and removed. Rarity weighting is defensible on its own terms — a scarce
skill distinguishes a candidate more than a ubiquitous one — but it is incompatible with
the ordering requirement below: an inverse-document-frequency spread of 1..13 is far
wider than the gap between 100% and 95% coverage, so a 93% match on scarce skills
outranked a 100% match on ordinary ones. Measured against a real 162-skill profile, the
top forty carried fourteen such inversions, the worst a 33-point drop.

Damping the effect does not reconcile the two: swept over production data, a factor of
0.05 held the order across forty results and broke by a hundred, 0.01 by forty. Only
removing the tilt holds it at depth.

A consequence the system accepts: full coverage of `[git, sql]` ranks with full coverage
of `[erlang, rust]`.

#### Scenario: Two postings covered equally rank together

- **WHEN** two vacancies of the same size are both fully covered by the candidate, one
  naming skills most postings ask for and one naming scarce ones
- **THEN** neither is ranked above the other on rarity grounds

### Requirement: The order is a descending run of coverage

Results SHALL be ordered by what share of the vacancy's skills the candidate holds. The
order MUST NOT climb back: every fully covered posting precedes every 95% one, and so on
down. A reader scanning the feed sees one descending run of percentages, not a sequence
that jumps between them.

Within one coverage band, the vacancy asking for MORE skills SHALL rank higher: engaging
twenty skills the candidate holds is a better match than engaging six.

A vacancy asking for fewer skills than a floor SHALL be treated as asking for the floor.
Without that, a single tag the candidate holds is 100% covered and would lead the feed —
measured on production data, a ranking without the floor filled its top ten with
single-skill postings.

A vector SHALL be absent — not zero-valued — when it would be meaningless: no skills
given, or no skill recognised. A zero vector is not "no opinion"; it ranks against
everything.

#### Scenario: Coverage never climbs back

- **WHEN** results are read in order
- **THEN** each posting's coverage is less than or equal to the one before it

#### Scenario: A fully covered posting outranks a larger partial overlap

- **WHEN** one vacancy names nine skills the candidate holds every one of, and another
  names forty of which they hold thirty
- **THEN** the fully covered vacancy ranks higher, despite the smaller overlap

#### Scenario: Within a band, breadth wins

- **WHEN** two fully covered vacancies name twenty and six skills
- **THEN** the twenty-skill vacancy ranks higher

#### Scenario: A single-tag posting does not lead

- **WHEN** a vacancy naming one skill the candidate holds is ranked against one naming
  nine they hold entirely
- **THEN** the nine-skill vacancy ranks higher

#### Scenario: A skill listed twice does not count twice

- **WHEN** a vector is built from a skill set naming the same slug more than once
- **THEN** the vector is identical to one built from the deduplicated set

### Requirement: The vacancy's vector is derived at index time

Each indexed job document SHALL carry its skill vector, derived when the document is
built from the job row — never fetched, inferred, or computed at query time.

The derivation SHALL NOT call an embedding model, an LLM, or any network service.
Skills are canonical slugs from a finite dictionary, so the vector is arithmetic. It
SHALL depend on nothing but the job's own skills: no catalogue statistics, no rollup,
no external state that could be stale or missing.

Every document SHALL carry the vector field, with either a vector or an explicit
clearing value. It SHALL NEVER be omitted, for two independent reasons: the index
merges document fields rather than replacing them, so an omission leaves a previously
stored vector in place; and an index with a declared embedder REJECTS a document that
omits the field, which would drop the posting out of the index entirely.

The job's vector SHALL carry a component the reader's vector never sets, sized by how
many skills the posting asks for. That component is what turns the cosine into coverage.

#### Scenario: An indexed job carries its vector

- **WHEN** a job whose skills are recognised is turned into an index document
- **THEN** the document carries a vector of the declared width under the skill
  embedder's name

#### Scenario: A job that loses its skills has its stored vector cleared

- **WHEN** a job whose skills are all unrecognised, or which has none, is turned into an
  index document
- **THEN** the document carries an explicit clearing value, so pushing it removes any
  vector the index already held rather than merging around it

#### Scenario: The reader's vector carries no sizing component

- **WHEN** a vector is built for a reader's profile
- **THEN** the component the job side uses for sizing is zero, so it contributes nothing
  to any comparison

