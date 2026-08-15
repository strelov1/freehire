# industry-vocabulary Specification

## Purpose

The curated alias-to-canonical vocabulary behind `companies.industries`: how a
free-text industry label resolves to a stored value, and why an unrecognized label
stores nothing at all.

It is the finer level beneath `vocab.DomainValues` — domains names ~20 coarse
verticals derived from job enrichment, and this vocabulary names what a company
does when that lands in the "other" bucket.

## Requirements

### Requirement: Industry labels resolve through a curated dictionary

The system SHALL resolve free-text industry labels to a curated canonical
vocabulary, and SHALL emit nothing for a label the dictionary does not know.
Guessing — including mechanically reformatting an unrecognized label into
canonical shape — SHALL NOT occur.

A canonical value SHALL be a lowercase slug matching `[a-z0-9]+(-[a-z0-9]+)*`, so
it survives use as a filter URL parameter. Display text SHALL be a separate lookup
from the canonical value, so storage and presentation can differ.

Resolution SHALL be insensitive to case, to separator style, and to `&` versus
`and`, so that `Financial Services`, `Financial-Services` and `financial services`
all reach one canonical value. A value that is already canonical SHALL resolve to
itself, so re-running a normalization pass over its own output changes nothing.

The result SHALL be sorted and de-duplicated.

#### Scenario: Separator and case variants collapse to one value

- **WHEN** the labels `Financial-Services` and `Financial Services` are resolved
- **THEN** the result is the single canonical value `financial-services`

#### Scenario: Curated synonyms collapse to one value

- **WHEN** the labels `AI` and `Artificial Intelligence` are resolved
- **THEN** the result is the single canonical value `ai`

#### Scenario: An unknown label emits nothing

- **WHEN** a label absent from the dictionary is resolved
- **THEN** no value is emitted for it, and no canonical value is invented from it

#### Scenario: Resolution is idempotent

- **WHEN** an already-canonical value is resolved
- **THEN** it resolves to itself, unchanged

### Requirement: Unresolved labels are reported, not silently discarded

A process that resolves industry labels in bulk SHALL report the labels the
dictionary failed to recognize — how many distinct labels were dropped, how many
occurrences that covers, and the most frequent among them.

Without this report a dict-only rule is indistinguishable from silent data loss,
and the dictionary has no evidence-driven way to grow.

#### Scenario: A bulk pass reports what it dropped

- **WHEN** a bulk resolution pass finishes having encountered labels outside the
  dictionary
- **THEN** it reports the count of distinct unrecognized labels, their total
  occurrences, and the most frequent of them
