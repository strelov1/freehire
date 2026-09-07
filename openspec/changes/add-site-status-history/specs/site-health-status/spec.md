## ADDED Requirements

### Requirement: Daily status history sampling

The system SHALL periodically sample the site's own derived status (the same
derivation the live read uses) and maintain, per UTC calendar day, the WORST
status observed that day. A day already recorded as `down` SHALL NOT be
downgraded to `degraded` or `operational` by a later, better sample on the
same day; a day's recorded status SHALL only move to a worse value, never a
better one, within that same day. Sampling SHALL run independently of, and
SHALL NOT block or be blocked by, any request serving the live status read.

#### Scenario: A later good sample does not erase an earlier bad one

- **WHEN** the site was sampled as `down` earlier today, and a later sample
  the same day is `operational`
- **THEN** today's recorded status remains `down`

#### Scenario: A worse sample overwrites an earlier better one

- **WHEN** the site was sampled as `operational` earlier today, and a later
  sample the same day is `degraded`
- **THEN** today's recorded status becomes `degraded`

#### Scenario: Concurrent samples for the same day never lose the worse one

- **WHEN** two samples for the same day are recorded at nearly the same time,
  one `operational` and one `down`, in either order
- **THEN** today's recorded status is `down`

### Requirement: Site status history read

The system SHALL report, alongside the live site status, the recorded daily
history for the trailing 90 days, oldest first, as a list of `{ day, status }`
entries. A day within that window with no recorded sample SHALL be absent
from the list — it SHALL NOT be reported as `operational` or any other
status, since no observation was actually made that day.

#### Scenario: A day before sampling began is absent, not "operational"

- **WHEN** the trailing 90-day window includes a day before this capability
  started sampling
- **THEN** that day has no entry in the reported history

#### Scenario: History reflects the worst-of-day value

- **WHEN** a day's recorded status is `degraded`
- **THEN** that day's entry in the reported history is `degraded`, regardless
  of what the live status reads at the moment of the read
