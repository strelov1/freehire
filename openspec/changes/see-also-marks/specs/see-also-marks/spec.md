## ADDED Requirements

### Requirement: Every "See also" card resolves exactly one visual mark

The system SHALL resolve, server-side, exactly one visual mark for every card in a
job page's "See also" row, using a strict precedence: a backer collection's brand
image, else a technology brand logo, else the collection's country flag, else a
color-coded family icon. No card SHALL render with no mark.

#### Scenario: A backer collection card gets its brand image

- **WHEN** a resolved "See also" collection is a backer collection with a known
  mark in the backer registry
- **THEN** the card's mark is the backer's brand image, unchanged from the
  collection's existing backer badge

#### Scenario: A technology collection with a curated logo gets its brand logo

- **WHEN** a resolved collection carries a `skills` facet param whose value has an
  entry in the technology-mark registry
- **THEN** the card's mark is that brand's logo, rendered on a circular background
  in the brand's own color

#### Scenario: A technology collection with no curated logo falls back to a family icon

- **WHEN** a resolved collection carries a `skills` facet param whose value has no
  entry in the technology-mark registry
- **THEN** the card's mark is the family icon for technology, not a blank mark

#### Scenario: A country collection gets that country's flag

- **WHEN** a resolved collection carries a `countries` facet param with a single
  concrete country code
- **THEN** the card's mark is that country's flag

#### Scenario: A seniority, role, or non-country remote collection gets its family icon

- **WHEN** a resolved collection carries a `seniority` param, a `category` param,
  or a `work_mode` param without a concrete single-country `countries` value
- **THEN** the card's mark is the color-coded family icon matching that param kind

### Requirement: A technology brand logo renders with a contrast-appropriate glyph color

The system SHALL compute the logo glyph's fill color (white or near-black) from the
brand's own background color using a luminance threshold, rather than storing a
hardcoded fill per brand.

#### Scenario: A dark brand color gets a white glyph

- **WHEN** a technology logo's brand background color has luminance below the
  contrast threshold
- **THEN** the glyph renders in white

#### Scenario: A light brand color gets a dark glyph

- **WHEN** a technology logo's brand background color has luminance at or above
  the contrast threshold
- **THEN** the glyph renders in a near-black color
