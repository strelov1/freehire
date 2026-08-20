## ADDED Requirements

### Requirement: An explicit no-experience statement resolves to zero years

`internal/jobfacts`'s `ExperienceYearsMin(description)` SHALL resolve to `0` when the
description explicitly states that no prior experience is required
("no prior experience required", "no experience necessary", "no previous experience
needed", and like phrasings). Today such a posting yields nothing, because the
extractor reads only a digit adjacent to a year word, and an entry-level posting
states the requirement in prose rather than as a figure — leaving it
indistinguishable from a posting that says nothing about experience at all.

The phrase list SHALL be precision-first and matched on word boundaries, in keeping
with every other dictionary in this package: a description that is merely silent
about experience SHALL continue to yield nothing, and the extractor SHALL NOT infer
`0` from the absence of a figure. Where a description carries both an explicit
no-experience statement and a years figure, the smallest resolved value SHALL win,
which preserves the extractor's existing conservative-floor behaviour.

This requires no schema change: the value lands in the existing
`jobs.experience_years_min` column and the existing
`enrichment.experience_years_min` index attribute.

#### Scenario: An explicit no-experience statement yields zero

- **WHEN** a description says "No prior experience required — we will train you"
- **THEN** `ExperienceYearsMin` returns `0`

#### Scenario: Silence still yields nothing

- **WHEN** a description mentions no experience requirement in any form
- **THEN** `ExperienceYearsMin` returns nil, not `0`

#### Scenario: A stated figure is unaffected

- **WHEN** a description says "5+ years of experience"
- **THEN** `ExperienceYearsMin` returns `5`, unchanged by this rule

#### Scenario: The conservative floor still wins when both appear

- **WHEN** a description says "No prior experience required" and elsewhere
  "3 years of experience with Go is a plus"
- **THEN** `ExperienceYearsMin` returns `0`
