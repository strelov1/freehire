## MODIFIED Requirements

### Requirement: A posting's requirements are derived deterministically from its description markup

The system SHALL derive a posting's stated requirements from its description
markup alone, with no model call, producing the same shape the enrichment
contract defines: an ordered list of entries each carrying a `text` and a
`priority` of `required` or `preferred`.

The derivation SHALL be gated on a controlled vocabulary of section headings
(for example `Requirements`, `Qualifications`, `What you'll need`,
`Nice to have`). Under a matching heading, the derivation SHALL read items from
either shape a section is stated in: the list items of the first list that
follows the heading, stopping at the next heading; or, when no list is found
before the section closes, a run of two or more paragraph-shaped items stated
directly under the heading, each becoming one entry. A single paragraph-shaped
item alone SHALL NOT be read as a section's content, since one paragraph cannot
be told apart from prose that merely explains the section away. A description
with no matching heading SHALL yield no entries: there is no fallback that
infers which list or paragraph run in a posting is the requirements content,
because a benefits or perks section must never be read as requirements.

Each entry's `priority` SHALL be decided by the heading it was found under —
headings expressing optionality (`nice to have`, `preferred`, `bonus`, `a plus`)
yield `preferred`, and every other heading in the vocabulary yields `required`.
A description carrying more than one matching heading SHALL yield the entries of
each, so a posting with both a required and an optional section produces both
priorities.

Entry text SHALL be extracted as plain text: markup inside a list item or
paragraph is stripped, character entities are decoded, and surrounding and
repeated whitespace is collapsed. An entry that is empty after this SHALL be
dropped.

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

#### Scenario: A matching heading followed by two or more paragraph items yields those items

- **WHEN** a description contains a heading matching the vocabulary followed by two or more paragraph-shaped items, with no list found before the section closes
- **THEN** the derivation returns one entry per paragraph, in document order, carrying the heading's priority

#### Scenario: A matching heading with no list yields nothing

- **WHEN** a description contains a heading matching the vocabulary but the content that follows is neither a list nor two or more paragraph-shaped items — no content at all, or exactly one paragraph-shaped item, before the section closes
- **THEN** the derivation returns no entries for that section

#### Scenario: Genuine prose before a list closes the section instead of being read as items

- **WHEN** a description contains a heading matching the vocabulary, followed by a paragraph of prose, followed by a list
- **THEN** the derivation returns no entries for that section: the list belongs to whatever follows, not to this heading

#### Scenario: A description with no markup yields nothing

- **WHEN** a description is plain prose with no headings
- **THEN** the derivation returns no entries

#### Scenario: Item markup is reduced to plain text

- **WHEN** a list item or paragraph item contains nested markup or character entities
- **THEN** the derived entry's text is the item's plain text with markup stripped, entities decoded, and whitespace collapsed

#### Scenario: The derivation obeys the enrichment contract's bounds

- **WHEN** a description's requirements list exceeds the enrichment contract's maximum entry count, or an item's text exceeds its maximum text length
- **THEN** the derivation truncates the list to that maximum count and clips each text to that maximum length
