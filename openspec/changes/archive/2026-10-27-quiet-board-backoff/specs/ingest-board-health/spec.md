## ADDED Requirements

### Requirement: A board whose listing is unchanged is crawled progressively less often
After each complete, successful crawl the system SHALL compute the board's listing fingerprint: the
sha256 of the board's de-duplicated, sorted posting external ids, counting every posting the listing
named whatever its later outcome. When the fingerprint equals the one stored from the previous complete
successful crawl, the system SHALL increment the board's quiet streak and SHALL set a quiet pause of
4h·2^(streak−1), capped at 72h. When it differs, or no fingerprint is stored, the system SHALL store
the new fingerprint, reset the quiet streak to zero and clear the quiet pause.

#### Scenario: Unchanged listing grows the pause
- **WHEN** a board's complete successful crawl yields the same set of posting ids as its previous one, for the third time in a row
- **THEN** its quiet streak is 3 and its quiet pause ends 16h after that crawl

#### Scenario: The pause is capped
- **WHEN** a board's quiet streak reaches 7
- **THEN** its quiet pause is 72h, not longer

#### Scenario: A posting appearing resets the board
- **WHEN** a board's complete crawl yields one posting id that its previous crawl did not
- **THEN** its quiet streak is reset to 0 and it has no quiet pause

#### Scenario: A posting disappearing resets the board
- **WHEN** a board's complete crawl no longer yields one posting id its previous crawl yielded
- **THEN** its quiet streak is reset to 0 and it has no quiet pause

#### Scenario: Order and duplicates do not count as change
- **WHEN** a board's crawl yields the same posting ids in a different order, or with one id repeated
- **THEN** the fingerprint is unchanged and the crawl counts as quiet

### Requirement: An incomplete crawl leaves the quiet state untouched
A crawl that did not read the board's whole listing SHALL NOT change the board's fingerprint, quiet streak
or quiet pause. That covers a crawl interrupted by the run's deadline, one that failed, one cut short by the
context ending, one where a listed posting could not be read or failed to save, and one where a hydrating
adapter left a NEW posting it listed un-hydrated (detail budget spent or detail failed).

#### Scenario: Deferred new postings
- **WHEN** a hydrating adapter lists three new postings but hydrates only one before its detail budget runs out
- **THEN** the board's quiet state is unchanged

#### Scenario: Interrupted crawl
- **WHEN** a board's crawl is interrupted by the run's crawl deadline
- **THEN** its stored fingerprint, quiet streak and quiet pause are exactly what they were before the run

### Requirement: A board in its quiet pause is skipped
Before invoking a board's adapter the system SHALL skip the board when its quiet pause lies in the future,
exactly as it skips a board in failure cooldown, and SHALL count it in the run's quiet total, separately
from cooled boards. The check SHALL fail open: a health-store error crawls the board.

#### Scenario: Quiet board skipped
- **WHEN** a run reaches a board whose quiet pause ends in the future
- **THEN** the adapter is not invoked and the run's quiet count increases by one

#### Scenario: Health-store error crawls the board
- **WHEN** reading the board's pauses fails
- **THEN** the board is crawled

### Requirement: The quiet pause is independent of the failure cooldown
The quiet pause SHALL be stored separately from the failure cooldown. Clearing cooldowns after a
provider-wide recovery SHALL NOT clear any quiet pause, and a failed crawl SHALL NOT change the quiet state.

#### Scenario: Recovery probe keeps quiet pauses
- **WHEN** a provider's recovery probe succeeds and clears its boards' cooldowns
- **THEN** every quiet pause of that provider's boards is unchanged

### Requirement: Self-closing providers are never paced by quietness
For a provider that closes its own postings from a change stream, the system SHALL NOT compute a
fingerprint or apply a quiet pause, because its emissions are changes, not a listing.

#### Scenario: Self-closing provider crawl
- **WHEN** a self-closing provider's board completes a crawl that emits nothing
- **THEN** its quiet streak stays 0 and it has no quiet pause

### Requirement: A board the run skipped is never swept by the company or source scope
The post-run unseen sweep SHALL exclude every posting of a board the run skipped — in quiet pause or in
failure cooldown — from its company-scoped and source-scoped closes. A skipped board's postings were
not looked at, so their age since last seen says nothing about them. Without this, a quiet pause longer
than the sweep window (48h) would close the live postings of a company whose other board the run did crawl.

#### Scenario: Quiet board of a company the run crawled elsewhere
- **WHEN** a company has two boards on one provider, the run crawls one and skips the other as quiet, and the skipped board's postings were last seen 60h ago
- **THEN** the company-scoped close leaves the skipped board's postings open

#### Scenario: Cooled board under a source-scoped close
- **WHEN** a full-catalogue provider's run skips one of its boards in failure cooldown
- **THEN** the source-scoped close leaves that board's postings open

### Requirement: A quiet board reads as fresh in health rollups
While a board's quiet pause holds, the provider health rollups that back the public status page and the
silence alerts SHALL treat the board as freshly successful. The board skips crawls on purpose for longer
than their 48h freshness window, and the pause is only ever set by a success.

#### Scenario: All of a provider's boards are quiet
- **WHEN** every board of a provider is in its quiet pause and their last success was 60h ago
- **THEN** the status page and the silence alert report the provider as fresh
