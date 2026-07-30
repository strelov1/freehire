## ADDED Requirements

### Requirement: Near-identical-description reposts collapse within a company+title bucket

The system SHALL collapse open canonical postings that share a `company_slug` and a
normalized (city-suffix-stripped) title AND whose normalized descriptions exceed a
configured word-similarity threshold, marking all but one `duplicate_of` the chosen canon
(the deterministic `min(id)`), reusing the existing collapse column and mechanism.
Comparison SHALL be bucketed by `(company_slug, normalized-title)` so it is bounded per
bucket and never compares postings of different roles.

#### Scenario: Same role, lightly-localized descriptions, collapses

- **WHEN** a company posts one role in several cities whose descriptions differ only in a
  small localized block (word-similarity above the threshold)
- **THEN** the postings collapse to one canonical card, the rest referencing it via
  `duplicate_of`

#### Scenario: Genuinely distinct jobs under a generic title do not collapse

- **WHEN** postings share a company and a generic stripped title (e.g. "software
  development engineer") but describe substantially different jobs (word-similarity far
  below the threshold)
- **THEN** they remain separate canonical rows

#### Scenario: Distinct specialties under one stripped title do not collapse

- **WHEN** a stripped-title bucket mixes specialties (e.g. "software engineer" over Data
  Infrastructure vs Platform), whose descriptions overlap only partially
- **THEN** each specialty stays its own canon; only same-specialty city variants collapse

### Requirement: The fuzzy pass runs after and never overrides the exact pass

The fuzzy-description pass SHALL run AFTER the exact role-cluster recompute and operate only
over its remaining open canonical rows, so it merges what byte-exact matching left split and
never re-splits or contradicts a deterministic collapse. It SHALL be idempotent and
reversible by the standard recompute.

#### Scenario: Exact-collapsed reposts are untouched

- **WHEN** the exact pass has already collapsed byte-identical-description reposts
- **THEN** the fuzzy pass leaves those `duplicate_of` markers unchanged

#### Scenario: Re-running is stable

- **WHEN** the fuzzy pass runs twice with no new postings
- **THEN** the second run changes no `duplicate_of` markers

### Requirement: Over-merge guards are enforced

The fuzzy pass SHALL guard against merging distinct roles: a conservative word-similarity
threshold, the shared stripped-title bucket, and a seniority/grade guard so postings that
differ only by grade are not merged.

#### Scenario: Different grades of one title are not merged

- **WHEN** two postings share a company and base title but carry different seniority grades
- **THEN** they are not collapsed together, regardless of description similarity
