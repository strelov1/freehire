## ADDED Requirements

### Requirement: Geography derivation also reads the job title for a restriction marker

When the `location` string alone resolves to `work_mode=remote` with no country and no
region, the system SHALL additionally scan the job `title` for an explicit geography
restriction marker before falling back to the description. A title-embedded marker (e.g. a
bracketed or parenthetical suffix naming a country, US state grouping, or macro-region, such
as `[Remote-US]` or `(Location - Australia or New Zealand)`) SHALL resolve against the same
curated country/region dictionaries the location parser uses, and SHALL NOT guess: a title
suffix that does not resolve against the dictionaries yields no geography from this step and
falls through to the description-based check. This step SHALL run only when `location` left
geography unpinned — a `location` that already resolves a country or region is never
overridden by the title.

#### Scenario: A title-embedded country suffix resolves a bare-remote posting

- **WHEN** a job's `location` is a bare `Remote` marker (no country, no region) and its
  `title` ends with a bracketed country suffix such as `[Remote-US]`
- **THEN** the derived `countries` include `us` and the `regions` include `north_america`

#### Scenario: A title-embedded macro-region phrase resolves a bare-remote posting

- **WHEN** a job's `location` is a bare `Remote` marker and its `title` contains a
  parenthetical restriction such as `(Location - Australia or New Zealand)`
- **THEN** the derived `countries` include `au` and `nz` (or the derived `regions` include
  `apac`, per the existing macro-region vocabulary)

#### Scenario: A title with no resolvable marker falls through unchanged

- **WHEN** a job's `location` is a bare `Remote` marker and its `title` contains no token the
  curated dictionaries can resolve
- **THEN** this step yields no geography and derivation proceeds to the description-based
  check exactly as before this requirement existed

#### Scenario: A location-resolved country is never overridden by the title

- **WHEN** a job's `location` already resolves to a country or region
- **THEN** the title is not consulted for geography, regardless of its content

### Requirement: Description-based geography restriction recognizes region-scoping prose

When `location` and `title` both leave geography unpinned, the description-based restriction
check SHALL match not only citizenship/work-authorization phrasing but also an explicit,
role/candidate-qualified "based/located/hiring/restricted/remote role within
`<country/region>`" scoping statement (for example: "a fully remote role within New Zealand,
Australia East Coast, or nearby time zones", "candidates based in the United States, Canada,
Argentina, or Brazil"). A match SHALL resolve the named countries/regions against the same
curated dictionaries as the location parser and SHALL NOT guess a geography from prose that
names no resolvable country/region token. The anchor phrases SHALL be role- or
candidate-qualified rather than a bare preposition ("based in"/"located in" alone) — an
unqualified anchor is ambiguous between a role restriction and an unrelated company-HQ
mention ("Our company is based in Berlin, but this role is fully remote and open
worldwide"), and matching the HQ sentence as if it were the role's own restriction is exactly
the kind of mislabeling this capability exists to prevent (found in code review; see
design.md's Decision 3).

#### Scenario: Region-scoping description prose resolves a bare-remote posting

- **WHEN** a job's `location` and `title` leave geography unpinned, and its `description`
  states the role is "a fully remote role within New Zealand, Australia East Coast, or
  nearby time zones"
- **THEN** the derived `countries` include `nz` — the clean dictionary token — while the
  noisy tokens ("Australia East Coast", "nearby time zones") resolve nothing rather than a
  guess, the same never-guess contract the location parser itself follows

#### Scenario: A multi-country scoping list resolves every named country

- **WHEN** a description states the role is "open to candidates based in the United States,
  Canada, Argentina, or Brazil"
- **THEN** the derived `countries` include `us`, `ca`, `ar`, and `br`

#### Scenario: A company-HQ mention is not mistaken for a role restriction

- **WHEN** a description states "Our company is based in Berlin, Germany, but this role is
  fully remote and open worldwide" — a company-location statement, not a role restriction
- **THEN** this step yields no geography, since the qualifying anchor phrases match only a
  role- or candidate-referring statement, never a bare "based in"/"located in"

#### Scenario: Non-restriction prose is still not mistaken for a signal

- **WHEN** a description mentions a country or region only incidentally (e.g. "we serve
  customers across Europe") with no role/based/located restriction phrasing
- **THEN** this step yields no geography, unchanged from prior behavior

<!-- Note: the base spec's existing "A bare remote marker yields no geography" requirement
     (and its "explicit open-anywhere marker yields global" counterpart) already state the
     target behavior; this change brings the implementation into compliance rather than
     changing the requirement text. See design.md for why the current implementation
     conflates the two and how the fix separates them without a spec delta. -->
