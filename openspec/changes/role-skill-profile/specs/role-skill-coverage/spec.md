## ADDED Requirements

### Requirement: Signed-in coverage of a role's skills

When a request for a single role's skill demand carries an authenticated session, the
system SHALL additionally return that caller's coverage of the role's ranked skills:
which of them the caller's profile holds outright, which they hold a recognised neighbour
of, and which they hold nothing for.

Coverage SHALL be computed from the caller's `userprofile.skills` — the one canonical
candidate skill set, into which the experience bank already folds banked skills — and
SHALL reuse the existing deterministic matcher (`internal/candidate/jobmatch`) and the
curated adjacency dictionary (`internal/dict/skilladjacency`) rather than introducing a
second notion of what counts as a match. A neighbour match SHALL name the skill it matched
through, so the claim is inspectable rather than asserted.

Coverage is a per-caller read and SHALL never be cached at a shared layer, never be
included in an anonymous response, and never widen or narrow the aggregate distribution
beside it: the same role returns the same skills to everyone, and only the overlay
differs.

#### Scenario: Signed-in caller sees their coverage

- **WHEN** a signed-in caller requests a single role's skill demand
- **THEN** the response carries a coverage section naming, for each of the role's ranked
  skills, whether the caller holds it, holds a neighbour of it, or holds nothing, plus a
  held-out-of-total count

#### Scenario: Neighbour match names its evidence

- **WHEN** the role ranks `aws` and the caller's profile holds `gcp` but not `aws`
- **THEN** that skill is reported as an adjacent match and names `gcp` as the skill it
  matched through

#### Scenario: Anonymous caller gets the aggregate alone

- **WHEN** an unauthenticated client requests the same role
- **THEN** the response is `200` with the skill distribution and no coverage section, and
  the request is never refused for lack of a session

#### Scenario: Caller with no profile skills

- **WHEN** a signed-in caller has no skills on their profile
- **THEN** the response carries a coverage section reporting zero held out of the role's
  total, never an absent section that a client could not distinguish from being signed out

#### Scenario: Coverage is not shared-cacheable

- **WHEN** a response carries a coverage section
- **THEN** its cache headers forbid shared caching, so one caller's coverage can never be
  served to another
