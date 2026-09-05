## ADDED Requirements

### Requirement: A posting's requirements are derived deterministically from its description markup

The system SHALL derive a posting's stated requirements from its description
markup alone, with no model call, producing the same shape the enrichment
contract defines: an ordered list of entries each carrying a `text` and a
`priority` of `required` or `preferred`.

The derivation SHALL be gated on a controlled vocabulary of section headings
(for example `Requirements`, `Qualifications`, `What you'll need`,
`Nice to have`). It SHALL take the list items of the first list that follows a
matching heading, stopping at the next heading. A description with no matching
heading SHALL yield no entries: there is no fallback that infers which list in a
posting is the requirements list, because a benefits or perks list must never be
read as requirements.

Each entry's `priority` SHALL be decided by the heading it was found under —
headings expressing optionality (`nice to have`, `preferred`, `bonus`, `a plus`)
yield `preferred`, and every other heading in the vocabulary yields `required`.
A description carrying more than one matching heading SHALL yield the entries of
each, so a posting with both a required and an optional section produces both
priorities.

Entry text SHALL be extracted as plain text: markup inside a list item is
stripped, character entities are decoded, and surrounding and repeated
whitespace is collapsed. An entry that is empty after this SHALL be dropped.

The derivation SHALL be bounded by the same maximum entry count and maximum text
length that the enrichment contract's sanitization enforces, read from those same
constants rather than restated, so both producers of the field obey one ceiling.

#### Scenario: A requirements heading followed by a list yields its items

- **WHEN** a description contains a heading matching the vocabulary followed by a list of items
- **THEN** the derivation returns one entry per list item, in document order, each with priority `required`

#### Scenario: An optional-section heading yields preferred entries

- **WHEN** a description contains a heading expressing optionality, such as "Nice to have", followed by a list
- **THEN** the derivation returns those items with priority `preferred`

#### Scenario: Both sections yield both priorities

- **WHEN** a description contains a required-section heading and an optional-section heading, each followed by a list
- **THEN** the derivation returns the items of both, each carrying the priority of the heading it appeared under

#### Scenario: A benefits list is not read as requirements

- **WHEN** a description's only heading-and-list pair is a benefits or perks section whose heading is outside the vocabulary
- **THEN** the derivation returns no entries

#### Scenario: A matching heading with no list yields nothing

- **WHEN** a description contains a heading matching the vocabulary but the content that follows is prose rather than a list
- **THEN** the derivation returns no entries

#### Scenario: A description with no markup yields nothing

- **WHEN** a description is plain prose with no headings
- **THEN** the derivation returns no entries

#### Scenario: Item markup is reduced to plain text

- **WHEN** a list item contains nested markup or character entities
- **THEN** the derived entry's text is the item's plain text with markup stripped, entities decoded, and whitespace collapsed

#### Scenario: The derivation obeys the enrichment contract's bounds

- **WHEN** a description's requirements list exceeds the enrichment contract's maximum entry count, or an item's text exceeds its maximum text length
- **THEN** the derivation truncates the list to that maximum count and clips each text to that maximum length

### Requirement: The derived requirements are stored per job

Each job SHALL carry its deterministically derived requirements as a stored
column, separate from the enrichment payload, so the two producers of the field
never overwrite one another and the provenance of a served list stays readable.

The column SHALL default to an empty list, so a job whose description yields
nothing is indistinguishable in shape from one never processed, and SHALL be
written by the ingest write path from the same description the rest of the
deterministic facets are derived from.

#### Scenario: A newly ingested posting carries its derived requirements

- **WHEN** a posting whose description contains a requirements heading and list is written by ingest
- **THEN** the stored job's derived-requirements column holds the derived entries

#### Scenario: A posting yielding nothing stores an empty list

- **WHEN** a posting whose description yields no derivable requirements is written by ingest
- **THEN** the stored job's derived-requirements column is an empty list, not null

### Requirement: A one-off backfill fills the derived requirements for the existing catalogue

A dedicated one-off worker SHALL derive and store the requirements of postings
that predate the column. It SHALL walk open postings only, in keyset order over
the job id, in bounded chunks, and SHALL be idempotent: a chunk update writes
only where the derived value differs from what is stored, so a re-run writes
nothing and stopping the worker part-way through is free and resumable.

The worker SHALL bound how much of the backlog one run takes and how large a
chunk is, each through its own environment knob, and SHALL require no
configuration beyond the database connection.

#### Scenario: The backfill stores the derivation for a pre-existing posting

- **WHEN** the worker runs against an open posting whose derived-requirements column is empty but whose description yields entries
- **THEN** the posting's derived-requirements column holds those entries afterwards

#### Scenario: Re-running the backfill writes nothing

- **WHEN** the worker runs a second time over postings it has already processed and whose descriptions have not changed
- **THEN** no rows are updated

#### Scenario: The backfill leaves closed postings alone

- **WHEN** the worker runs over a range of ids containing closed postings
- **THEN** those postings' derived-requirements columns are unchanged

#### Scenario: One run is bounded

- **WHEN** the worker runs with a per-run maximum configured
- **THEN** it stops after processing at most that many postings and exits successfully, leaving the remainder for a later run
