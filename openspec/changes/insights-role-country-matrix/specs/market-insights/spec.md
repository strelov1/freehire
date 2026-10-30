## MODIFIED Requirements

### Requirement: Precomputed insights rollups

Insight reads SHALL be served from precomputed rollup tables rather than
aggregating the full `jobs` table on each request. The rollups SHALL be a pure
function of current `jobs` state, recomputed by a cron-scheduled run-once worker and
swapped in atomically so readers never observe a partially rebuilt rollup.

A posting SHALL count as open at a past instant when it EXISTED by then and had not
closed by then. A posting existed from the EARLIER of its publication date
(`posted_at`) and the date freehire first saw it (`created_at`); a posting with no
publication date existed from `created_at`. First-seen alone SHALL NOT decide it:
it records when freehire found a posting, and a board onboarded inside the window
would otherwise publish its entire backlog as growth. This rule SHALL be applied
identically by every rollup that publishes a prior-window count (role demand, skill
demand, company hiring signal, role × country).

#### Scenario: Reader never sees a partial rebuild

- **WHEN** the rollup worker is mid-recompute
- **THEN** concurrent insight reads return the previous complete snapshot until
  the recompute commits

#### Scenario: Recompute is idempotent

- **WHEN** the rollup worker runs twice with unchanged `jobs` state
- **THEN** the resulting rollup tables are identical

#### Scenario: A newly discovered old posting is not growth

- **WHEN** an open posting was published 60 days ago and first seen by freehire
  5 days ago, and the growth window is 30 days
- **THEN** it counts in both the current open count and the prior-window open count,
  so it contributes zero to growth

#### Scenario: A genuinely new posting is growth

- **WHEN** an open posting was published 5 days ago and first seen 4 days ago
- **THEN** it counts in the current open count only, contributing one to growth

#### Scenario: A posting with no publication date falls back to first-seen

- **WHEN** an open posting has no `posted_at` and was first seen 5 days ago
- **THEN** it counts in the current open count only

#### Scenario: A publication date later than first-seen does not delay existence

- **WHEN** a posting's `posted_at` is later than its `created_at` (a source that
  re-stamps the date on repost)
- **THEN** it existed from `created_at`, the earlier of the two
