## MODIFIED Requirements

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

## ADDED Requirements

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

### Requirement: The blurred teaser is not announced as a score

The blurred teaser SHALL be hidden from assistive technology, and a screen reader MUST NOT be given its fabricated percentage as if it were the viewer's match. A card showing the teaser SHALL expose, in place of the hidden figures, a text alternative pointing at the sign-in affordance.

#### Scenario: Screen reader is offered the invitation, not the number

- **WHEN** a card renders the blurred teaser
- **THEN** the teaser strip SHALL be marked hidden from assistive technology, and a visually-hidden invitation to sign in for a real match SHALL be exposed instead
