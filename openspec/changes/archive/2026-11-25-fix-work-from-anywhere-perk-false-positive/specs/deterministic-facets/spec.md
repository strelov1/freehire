## ADDED Requirements

### Requirement: A bounded travel/PTO perk phrase does not fill the remote work mode

The `work_mode` facet's description-derived tier (the lowest-priority source in the
`countries`/`regions`/`work_mode`/`skills`/`seniority`/`category` precedence chain)
SHALL NOT resolve `remote` from the phrase "work from anywhere" (or the hyphenated
"work-from-anywhere") when a bounded-duration qualifier — the word pattern "up to" —
appears near the match, on either side, within a measured window. Such a
description is stating a bounded travel/PTO allowance (a benefit), not the
posting's own work arrangement, and treating it as an unconditional remote
statement produces a wrong facet on a hybrid or onsite posting that has no
higher-priority signal to override it. The guard SHALL NOT apply to the other
description-derived remote phrases (`fully remote`, `100% remote`, `remote-first`,
`remote position`, `remote role`, `remote job`, `remote opportunity`,
`remote vacancy`, "this position is remote", "role is remote", "position is
remote") — none of those double as a common bounded-benefit idiom the way "work
from anywhere" does, so guarding them would only cost coverage with no measured
gain. When the guard suppresses the "work from anywhere" match and no other phrase
in the description resolves a work mode, the facet stays empty (never guessed),
consistent with this dictionary's existing behavior when no anchored arrangement
phrase is present.

#### Scenario: A bounded travel perk after the phrase does not resolve remote

- **WHEN** a job's structured source and location marker both leave `work_mode`
  empty, and its description reads "the freedom to work from anywhere in the world
  for up to a month"
- **THEN** the derived `work_mode` is empty, not `remote`

#### Scenario: A bounded travel perk before the phrase does not resolve remote

- **WHEN** a job's structured source and location marker both leave `work_mode`
  empty, and its description reads "benefits include up to 12 days work from
  anywhere"
- **THEN** the derived `work_mode` is empty, not `remote`

#### Scenario: An unqualified "work from anywhere" statement still resolves remote

- **WHEN** a job's structured source and location marker both leave `work_mode`
  empty, and its description reads "you can work from anywhere in the EU" with no
  bounded-duration qualifier nearby
- **THEN** the derived `work_mode` is `remote`

#### Scenario: A qualified phrase does not block a genuinely remote description

- **WHEN** a job's description contains both "work from anywhere ... for up to 12
  weeks a year" and, elsewhere, an unqualified remote phrase such as "this is a
  fully remote position"
- **THEN** the derived `work_mode` is `remote`, from the unqualified phrase

#### Scenario: The other remote phrases are unaffected by the guard

- **WHEN** a job's structured source and location marker both leave `work_mode`
  empty, and its description reads "This is a 100% remote role, with a bonus of up
  to $5,000 for relocation"
- **THEN** the derived `work_mode` is `remote` (the qualifier guard applies only to
  "work from anywhere"/"work-from-anywhere", not to `100% remote`)
