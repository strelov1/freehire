## MODIFIED Requirements

### Requirement: A title whose "design" names no craft resolves to no category

The system SHALL emit no category for a title where a category alias appears but
states no category of its own — "Software Design Engineer" is software engineering,
where "design" qualifies what is engineered.

Such a phrase SHALL be an ORDERED TABLE ENTRY carrying a sentinel canonical, not a
mask applied before the match. A mask fails twice over: cutting the span exposes
whatever alias sits further down the table (the region below the design block is
almost entirely the business categories, so "Software Design Engineer - Sales Tools"
resolved to `sales` and lost its enrichment), and cutting is boundary-blind where
every matcher is boundary-aware. As an entry it simply wins the first-match walk.
Every exit that reads the table — the parsed category, the multi-category CV path,
and the alias map feeding the generated web contracts — MUST translate the sentinel
away, so it can never reach a column, an API response or a picker.

The tech-title detector SHALL recognize the software forms, including the `-ing`
spelling, so `is_tech` stays `true` even though the sub-category is unresolved. A
title that DOES have a better category SHALL be routed to it rather than made blind,
and the blind phrases MUST stay narrow: a qualified draughting title such as
"HVAC Systems Design Engineer" must keep the placement that vetoes its deletion.

#### Scenario: A software title keeps no category but stays technical

- **WHEN** a job titled "Senior Software Design Engineer" or
  "Software Design Engineering Manager" is classified
- **THEN** its category is empty, and its derived `is_tech` is `true`

#### Scenario: The sentinel never reaches a consumer

- **WHEN** the same title is parsed, run through the multi-category CV path, or
  enumerated in the category alias map the web contracts are generated from
- **THEN** none of them carries the sentinel value

#### Scenario: A qualified draughting title is not made blind

- **WHEN** a job titled "HVAC Systems Design Engineer" is classified
- **THEN** its category is `engineering_design`, so the deletion veto still applies

#### Scenario: A title with a better category is routed, not masked

- **WHEN** a job titled "Cloud Design Engineer" or "Solution Design Engineer" is classified
- **THEN** its category is `devops` and `solutions_engineering` respectively

#### Scenario: Design disciplines of their own stay on the product side

- **WHEN** a job titled "Service Design Engineer", "Experience Design Engineer"
  or "Game Design Engineer" is classified
- **THEN** its category is `design`

#### Scenario: The audio spellings leave the product side

- **WHEN** a job titled "Sound Design Engineer" or "Audio Design Engineer" is
  classified
- **THEN** its category is `creative`, not `design` and not
  `engineering_design` — audio moved out of product design with the rest of its
  craft, and both of these spellings have to be declared above the draughting
  block or they fall through the bare "design engineer" alias into draughting,
  which would scatter one craft across three categories
