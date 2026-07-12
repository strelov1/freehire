# aggregator-fuzzy-title-dedup Specification

## Purpose
TBD - created by archiving change aggregator-fuzzy-title-dedup. Update Purpose after archive.
## Requirements
### Requirement: Suppression also matches on an entity-decoded, suffix-stripped title

The system SHALL suppress an open aggregator posting as `duplicate_of` an open canonical
ATS posting when, in addition to the existing exact normalized-title match, both sides
agree on a normalized key that decodes HTML entities and strips one trailing separator
suffix. The normalized key SHALL be computed by decoding common HTML entities (at least
`&amp;`, `&#38;`, `&quot;`, `&#39;`, `&lt;`, `&gt;`) to their characters, removing a single
trailing ` - `, ` | `, or ` — ` separated segment when a non-empty base remains, then
applying the same lowercase-and-collapse normalization as the exact key. All other
suppression conditions are unchanged: same `company_slug`, compatible country, aggregator
side only, ATS side canonical and open.

#### Scenario: An appended location suffix on the ATS title still matches

- **WHEN** an aggregator posting `Assistant Director of Sales` and an ATS posting
  `Assistant Director of Sales - Leisure` share a company and compatible country
- **THEN** the aggregator posting is marked `duplicate_of` the ATS posting

#### Scenario: An undecoded HTML entity still matches

- **WHEN** an aggregator posting `Assistant F&amp;B Marketing Manager` and an ATS posting
  `Assistant F&B Marketing Manager` share a company and compatible country
- **THEN** the aggregator posting is marked `duplicate_of` the ATS posting

#### Scenario: The exact-key behavior is preserved

- **WHEN** an aggregator posting exactly matches an ATS twin's normalized title
- **THEN** it is suppressed exactly as before (the normalized key is additive, never a
  replacement)

#### Scenario: A hyphenated role name is not shredded

- **WHEN** a title contains a hyphen without surrounding spaces (e.g. `Full-Stack Engineer`)
- **THEN** the separator-strip does not remove any segment, so it is not spuriously matched
  against an unrelated `Full` role

### Requirement: Aggregator-only, ATS-canonical, and failover invariants are preserved

The system SHALL apply the normalized-key match under every invariant of the exact
suppression: only an aggregator posting may be suppressed (an ATS posting is never demoted),
only against an open canonical ATS posting, only within a compatible country, and a match
lost because the ATS twin closed SHALL release the aggregator copy on the next reindex.

#### Scenario: ATS row stays canonical under a normalized-key match

- **WHEN** an aggregator posting is suppressed via the normalized key
- **THEN** the matched ATS posting's `duplicate_of` stays NULL and the aggregator posting's
  points to it

#### Scenario: Normalized-key suppression releases when the ATS twin closes

- **WHEN** the ATS posting that suppressed an aggregator copy via the normalized key closes
  and the reindex runs again
- **THEN** the aggregator copy's `duplicate_of` is cleared and it re-enters search,
  embedding, and enrichment

