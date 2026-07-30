# job-profile-match Specification

## Purpose
TBD - created by archiving change job-profile-match. Update Purpose after archive.
## Requirements
### Requirement: Per-job match endpoint

The system SHALL expose `GET /api/v1/jobs/:slug/match` behind `RequireAuthOrKey` (session cookie or API key), addressed by a job's public slug. It SHALL classify each skill of the open job against the caller's profile skills and return the classification plus a coverage percent in the standard `{"data": ...}` envelope. The computation SHALL be deterministic and MUST NOT call an LLM.

#### Scenario: Authenticated caller with a profile

- **WHEN** an authenticated caller requests the match for a job whose skills are `[react, typescript, graphql, nodejs, aws]` and their profile skills are `[react, typescript, gcp]`
- **THEN** the response `data` reports `total: 5`, `exact_count: 2` (`react`, `typescript`), `adjacent_count: 1` (`aws` via `gcp`), the `missing` list `[graphql, nodejs]`, and `coverage_percent: 50`

#### Scenario: Unauthenticated caller

- **WHEN** a caller without a valid session cookie or API key requests the match endpoint
- **THEN** the system SHALL respond `401` and MUST NOT return any match data

#### Scenario: Unknown job slug

- **WHEN** an authenticated caller requests the match for a slug that resolves to no job
- **THEN** the system SHALL respond `404`

### Requirement: Skill classification and coverage formula

Each job skill SHALL be classified as **exact** when the profile contains that canonical skill, else **adjacent** when the profile contains a neighbour of it per the curated adjacency dictionary (`internal/verdict/adjacent.go`), else **missing**. An adjacent classification SHALL carry the `via` skill — the specific held neighbour that satisfied it. Coverage percent SHALL be `round((exact_count + 0.5 × adjacent_count) / total × 100)`, where an exact match weighs 1 and an adjacent match weighs one half.

#### Scenario: Exact takes precedence over adjacent

- **WHEN** a job skill is present exactly in the profile and also has a held neighbour
- **THEN** it SHALL be classified `exact`, not `adjacent`

#### Scenario: Adjacent names its via skill

- **WHEN** a job requires `aws`, the profile lacks `aws` but holds `gcp`, and the dictionary treats them as neighbours
- **THEN** `aws` SHALL be classified `adjacent` with `via: "gcp"`

#### Scenario: Percent rounds half-weighted adjacents

- **WHEN** a job has 5 skills with 2 exact and 1 adjacent
- **THEN** `coverage_percent` SHALL be `50` (`round((2 + 0.5) / 5 × 100)`)

#### Scenario: Job with no recognised skills

- **WHEN** the job's skill list is empty
- **THEN** the endpoint SHALL report `total: 0` with empty `matched`, `adjacent`, and `missing` lists, and the sidebar SHALL render a "not enough data" state rather than a match block

### Requirement: Match response contract

The match response `data` SHALL contain `total`, `exact_count`, `adjacent_count`, `coverage_percent`, `matched` (list of skill names), `adjacent` (list of `{name, via}`), and `missing` (list of skill names). A corresponding TypeScript type SHALL be generated via `cmd/gen-contracts` so the SPA consumes a typed shape.

#### Scenario: Response envelope shape

- **WHEN** the endpoint returns a successful match
- **THEN** the body SHALL be `{"data": {total, exact_count, adjacent_count, coverage_percent, matched, adjacent, missing}}` with `adjacent` entries carrying both `name` and `via`

### Requirement: Sidebar match block states

The job-detail sidebar SHALL render a match block at its top with exactly four mutually exclusive states, choosing the state without redundant network calls. The guest and no-profile states SHALL show a lightly-blurred teaser built from the open job's own skills with a single footer call-to-action, and MUST NOT call the match endpoint.

#### Scenario: Not-enough-data state

- **WHEN** the open job has no recognised skills
- **THEN** the block SHALL show a "not enough data" card and SHALL NOT call the match endpoint

#### Scenario: Guest state

- **WHEN** the viewer is not authenticated and the job has two or more skills
- **THEN** the block SHALL show a lightly-blurred teaser carrying the job's own skills, its deterministic percentage, and a footer "Sign in" button, and SHALL NOT call the match endpoint

#### Scenario: No-profile state

- **WHEN** the viewer is authenticated but has no profile or an empty profile skill list
- **THEN** the block SHALL show the same lightly-blurred teaser with a footer "Upload CV" button, and SHALL NOT call the match endpoint

#### Scenario: Single-skill job shows the call-to-action alone

- **WHEN** a locked viewer opens a job carrying exactly one skill
- **THEN** the block SHALL show its call-to-action without a teaser and without the rule that would divide them

#### Scenario: Teaser chips stay on one row

- **WHEN** the open job carries more skills than fit the sidebar's width
- **THEN** the teaser SHALL cap the chips it renders at a count that leaves each name legible and clip the row at the panel's edge, so it never wraps to a second line

#### Scenario: A short chip row still shows both tones

- **WHEN** the capped chip row would otherwise consist entirely of skills the teaser marks as held
- **THEN** the row SHALL trade its last chip for the job's first missing skill, so the have/missing contrast survives the cap

#### Scenario: Real match state

- **WHEN** the viewer is authenticated with a non-empty profile skill list and the job has skills
- **THEN** the block SHALL call the match endpoint and render the percentage, a two-colour progress bar (exact segment plus a half-weight adjacent segment), and three chip groups — You have (exact), Close (adjacent, each hinting its `via` skill), and Missing

### Requirement: Deterministic locked-state teaser figures

The teaser figures SHALL be derived deterministically from the job's public slug, so the same job yields the same teaser on every render, during server-side rendering and after hydration, and in every surface that shows it. The teaser MUST be built from the job's own skills — never from a hardcoded skill list — and MUST NOT be presented as a computed match. It SHALL report a coverage percent between 60 and 90 inclusive, a matched count consistent with that percent over the job's real skill count, and a have/missing split marking at least one skill as held and at least one as missing. A job carrying fewer than two skills SHALL yield no teaser: there is no contrast to draw, and a "1 of 1 skills" label beside a partly-filled bar is a figure a viewer could catch out.

#### Scenario: Same job renders the same teaser

- **WHEN** the teaser is derived twice for one job slug
- **THEN** both derivations SHALL yield an identical percent and an identical have/missing split

#### Scenario: Percent stays inside the teaser band

- **WHEN** the teaser is derived for any job slug
- **THEN** the percent SHALL be at least 60 and at most 90

#### Scenario: Matched count agrees with the percent

- **WHEN** the teaser reports a percent for a job with a known skill count
- **THEN** the matched count SHALL be that percent of the skill count, rounded, so the "N of M skills" label cannot contradict the percentage shown beside it

#### Scenario: Both tones are present

- **WHEN** the teaser is derived for any job it applies to
- **THEN** at least one skill SHALL read as held and at least one SHALL read as missing

#### Scenario: A skill-less job has no teaser

- **WHEN** the teaser is derived for a job with an empty skill list
- **THEN** it SHALL yield nothing, leaving the not-enough-data state to render

#### Scenario: A single-skill job has no teaser

- **WHEN** the teaser is derived for a job carrying exactly one skill
- **THEN** it SHALL yield nothing, and the surfaces SHALL fall back to what they show for a viewer with no teaser

### Requirement: Card-level teaser for locked viewers

A job card SHALL show the blurred profile-match teaser to a viewer in either locked state — not authenticated, or authenticated without profile skills — where a viewer with profile skills sees the real client-computed coverage bar. The card's skill chips SHALL take their held/missing tint from the same teaser that feeds its bar, so the chips and the percentage cannot disagree. Blurring SHALL cover the chips and the coverage strip only, leaving the rest of the card — salary included — legible.

#### Scenario: Guest sees the blurred teaser

- **WHEN** an unauthenticated viewer sees a card for a job with two or more skills
- **THEN** the card SHALL render the tinted skill chips and the coverage strip under a light blur, and SHALL NOT issue a per-card match request

#### Scenario: Signed-in viewer without skills sees the blurred teaser

- **WHEN** an authenticated viewer whose profile has no skills sees a card for a job with two or more skills
- **THEN** the card SHALL render the same blurred teaser as a guest

#### Scenario: A viewer with a profile sees the real bar

- **WHEN** an authenticated viewer with profile skills sees a card for a job with two or more skills
- **THEN** the card SHALL render the client-computed coverage bar unblurred, with chips tinted by the viewer's actual skills

#### Scenario: Salary stays legible under the teaser

- **WHEN** a card showing the blurred teaser also carries a salary
- **THEN** the salary SHALL render unblurred

### Requirement: The analysis offer is shown to a guest

The sidebar SHALL offer the deep-dive fit analysis to an unauthenticated viewer as well, with the same description and button a signed-in viewer sees, because the offer needs no computed match to make sense. Its button MUST open sign-in in place, and MUST NOT navigate to the analysis page: that page streams an authenticated compute client-side, so a guest reaching it would issue a rejected request per visit. The credit line SHALL be replaced by a statement of what signing in is for. The no-profile state SHALL NOT show the offer, its own "Upload CV" call-to-action already standing directly above.

#### Scenario: Guest presses the analysis button

- **WHEN** an unauthenticated viewer presses "Analyze match" in the sidebar
- **THEN** the sign-in dialog SHALL open, the viewer SHALL remain on the job page, and no analysis request SHALL be issued

#### Scenario: The offer states what sign-in buys

- **WHEN** the offer renders for an unauthenticated viewer
- **THEN** it SHALL show the same description and button as for a signed-in viewer, with a line saying the analysis needs a signed-in account and a CV, and no AI-credit count

#### Scenario: No-profile viewer is not offered it twice

- **WHEN** an authenticated viewer with no profile skills opens the block
- **THEN** the analysis offer SHALL be absent, leaving the block's own "Upload CV" call-to-action as the single next step

### Requirement: The blurred teaser is not announced as a score

The blurred teaser SHALL be hidden from assistive technology, and a screen reader MUST NOT be given its fabricated percentage as if it were the viewer's match. A card showing the teaser SHALL expose, in place of the hidden figures, a text alternative pointing at the sign-in affordance.

#### Scenario: Screen reader is offered the invitation, not the number

- **WHEN** a card renders the blurred teaser
- **THEN** the teaser strip SHALL be marked hidden from assistive technology, and a visually-hidden invitation to sign in for a real match SHALL be exposed instead

### Requirement: Hard-constraint blockers beside skill coverage

The profile-match result SHALL, when the caller's structured résumé and the job's structured requirements are available, include the deterministic hard-constraint blockers alongside the skill-coverage classification. The blockers MUST be advisory: they never hide, downrank, or filter the job. When the structured inputs are unavailable, the result MUST degrade to skill coverage only, with no blockers and no error.

#### Scenario: Blockers surface next to coverage

- **WHEN** an authenticated caller with a structured résumé views a job whose requirements they do not fully meet
- **THEN** the profile-match payload carries both the skill coverage and the unmet hard-constraint blockers, and the job remains fully visible and clickable

#### Scenario: No structured résumé degrades to coverage only

- **WHEN** the caller has no structured résumé
- **THEN** the profile-match payload carries skill coverage with no blockers and returns no error

