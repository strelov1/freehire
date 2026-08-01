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

### Requirement: Claiming a missing skill from the match block

In the real-match state, every chip in the **Missing** and **Close** groups SHALL be an activatable
control that discloses a claim row naming that skill and offering to add it to the caller's profile.
Confirming SHALL write the skill into the caller's profile skill set and MUST NOT write to the stored
CV, the structured résumé, or the experience bank. Chips in the **You have** group SHALL remain inert:
this affordance adds skills and never removes them. The claim affordance SHALL be absent from the
guest, no-profile, no-skills and loading states, whose chips are a fabricated teaser rather than a
match.

#### Scenario: Missing chip discloses the claim row

- **WHEN** a signed-in viewer with a profile presses the `entra-id` chip in the Missing group
- **THEN** a claim row naming `entra-id` SHALL appear with an action to add it to the profile, and the
  pressed chip SHALL report itself as expanded to assistive technology

#### Scenario: Close chip discloses the same claim row

- **WHEN** the viewer presses a Close chip — a skill they hold only through a neighbour, such as
  `azure · you have aws`
- **THEN** the claim row SHALL offer to add `azure` itself, not the neighbour it was matched through

#### Scenario: Only one claim row is open at a time

- **WHEN** a claim row is open for one skill and the viewer presses a different Missing or Close chip
- **THEN** the row SHALL move to the newly pressed skill, and pressing the same chip again SHALL close it

#### Scenario: Held chips offer nothing

- **WHEN** the viewer presses a chip in the You have group
- **THEN** no claim row SHALL open and the profile SHALL NOT change

#### Scenario: A locked viewer cannot claim

- **WHEN** an unauthenticated viewer, or a signed-in viewer with no profile skills, sees the blurred
  teaser
- **THEN** its chips SHALL NOT be activatable and no claim row SHALL be reachable

### Requirement: The claimed skill is reclassified before the write settles

Confirming a claim SHALL move the skill into the **You have** group and recompute the coverage
percentage and the two-colour bar client-side, without waiting for the profile write. The client-side
recomputation SHALL use the same weighting as the server — an exact match weighs 1, an adjacent match
one half — so the optimistic figure agrees with the server's for the skills it can classify. Once the
write succeeds the block SHALL refetch the match and render the server's classification in place of the
optimistic one, because a claim can also turn a third skill from missing to close through the adjacency
dictionary, which the client does not hold.

#### Scenario: Chip moves and the percentage rises immediately

- **WHEN** the viewer confirms a claim for a Missing skill of a job carrying 4 skills, 1 of which was
  already exact
- **THEN** the skill SHALL appear in You have, the reported counts SHALL read 2 of 4, and the coverage
  SHALL read 50% before the profile request settles

#### Scenario: A claimed Close skill stops being half-weighted

- **WHEN** the viewer claims a skill that was classified adjacent
- **THEN** it SHALL leave the Close group, the adjacent count SHALL fall by one, the exact count SHALL
  rise by one, and the coverage SHALL rise by the half weight the adjacency was contributing

#### Scenario: The server's classification replaces the optimistic one

- **WHEN** the profile write succeeds
- **THEN** the block SHALL refetch the match and render the returned classification, so a skill the
  claim newly made adjacent is shown as Close rather than left in Missing

#### Scenario: A failed refetch keeps the optimistic view

- **WHEN** the profile write succeeds but the follow-up match request fails
- **THEN** the block SHALL keep showing the optimistic classification and MUST NOT revert the claim,
  which the server has already accepted

### Requirement: A claim is confirmed and reversible

After a successful claim the block SHALL show a confirmation naming the skill and offering to undo it.
The confirmation SHALL name the most recent claim; a further claim replaces it. Undo SHALL subtract
only that skill from the profile skill set — never restore a whole earlier profile, which would roll
back any claim made after it — and SHALL restore the block to the classification it had before that
claim. A claimed skill SHALL also be dropped from the profile's excluded-skills set, so the profile
cannot simultaneously claim and avoid the same skill; undo SHALL NOT restore that exclusion.

#### Scenario: Confirmation offers undo

- **WHEN** a claim for `entra-id` is written successfully
- **THEN** the block SHALL show that `entra-id` was added to the profile, with an undo action

#### Scenario: A second claim takes over the confirmation

- **WHEN** the viewer claims `entra-id` and then claims `powershell`
- **THEN** the confirmation SHALL name `powershell`, and undoing it SHALL leave `entra-id` in the
  profile

#### Scenario: Undo returns the skill to the group it came from

- **WHEN** the viewer undoes a claim for `entra-id`
- **THEN** `entra-id` SHALL return to the Missing group and the coverage SHALL read what it did
  before the claim

#### Scenario: Claiming an excluded skill resolves the contradiction

- **WHEN** the viewer claims a skill their profile currently lists among its excluded skills
- **THEN** the saved profile SHALL carry that skill among its skills and no longer among its excluded
  skills

### Requirement: Concurrent claims cannot drop each other

Because the profile endpoint replaces the whole profile row, claims SHALL be serialised so that each
write is built from the result of the previous one. Two claims confirmed in quick succession SHALL both
survive.

#### Scenario: Two rapid claims both persist

- **WHEN** the viewer confirms a claim for `bash` and, before that request settles, confirms one for
  `powershell`
- **THEN** the profile SHALL end up holding both skills, and neither write SHALL be sent with a skill
  list that predates the other

### Requirement: A failed claim is rolled back and reported

When the profile write fails, the block SHALL return the skill to the group it came from, restore the
previous counts and coverage, and state that the skill could not be added. It MUST NOT show the
confirmation or the undo action for a claim that did not land.

#### Scenario: Write failure restores the chip

- **WHEN** the profile write for a claimed skill fails
- **THEN** the skill SHALL reappear in the group it was claimed from, the coverage SHALL return to its
  previous value, and an error naming the failure SHALL be shown

### Requirement: Avoiding a skill from the match block

The claim row SHALL offer, beside the action that claims the skill, an action that records the skill
as one the viewer wants to avoid — written to the profile's excluded-skills set. Because the row
carries two answers, it SHALL name the skill rather than pose a yes/no question. Avoiding a skill
SHALL remove it from the profile's skills, mirroring the rule that claiming one removes it from the
excluded set: a skill is never in both lists.

#### Scenario: The row offers both answers

- **WHEN** the viewer presses a Missing or Close chip
- **THEN** the row SHALL name that skill and offer both an action that adds it to the profile's
  skills and an action that adds it to the profile's avoided skills

#### Scenario: Avoiding writes the excluded set

- **WHEN** the viewer avoids `wordpress`
- **THEN** the saved profile SHALL carry `wordpress` among its excluded skills and not among its
  skills

#### Scenario: Avoiding a skill that was somehow held clears the contradiction

- **WHEN** the profile holds `php` among its skills and the viewer avoids `php`
- **THEN** the saved profile SHALL carry `php` only among its excluded skills

### Requirement: The match does not move when a skill is avoided

Avoiding a skill SHALL leave the coverage percentage, the progress bar and the three chip groups
exactly as they were. The server computes the match from the profile's skills alone, so an avoided
skill is still a skill the candidate does not have, and the block MUST NOT imply otherwise by
re-scoring or by dropping the chip from Missing.

#### Scenario: Coverage is unchanged

- **WHEN** the viewer avoids a skill from the Missing group
- **THEN** the coverage percentage and the counts SHALL read exactly what they did before, and the
  skill SHALL remain in the Missing group

#### Scenario: No match request is issued

- **WHEN** an avoid is written successfully
- **THEN** the block SHALL NOT refetch the match, there being nothing in it that could have changed

### Requirement: An avoided skill is marked wherever it appears

A chip naming a skill in the viewer's excluded set SHALL render as avoided — visually distinct from
an ordinary missing skill and marked as such for assistive technology. The marking SHALL be derived
from the profile the block already holds, so it appears on every job asking for that skill without
any additional request.

#### Scenario: The mark survives to another job

- **WHEN** the viewer avoids `wordpress` on one job and opens another job that also asks for
  `wordpress`
- **THEN** that job's `wordpress` chip SHALL render as avoided, with no further request made to
  obtain the avoided set

#### Scenario: The mark is announced, not merely drawn

- **WHEN** a chip renders as avoided
- **THEN** its accessible name SHALL say the skill is one the viewer avoids

### Requirement: An avoided skill can be un-avoided where it was marked

Pressing an avoided chip SHALL open the row offering to claim the skill or to stop avoiding it.
Stopping SHALL remove the skill from the excluded set and leave the skills set untouched, and the
chip SHALL return to an ordinary missing chip.

#### Scenario: The mark is lifted from the block

- **WHEN** the viewer presses an avoided chip and chooses to stop avoiding it
- **THEN** the skill SHALL leave the profile's excluded skills and the chip SHALL render as an
  ordinary missing skill

#### Scenario: An avoided skill can still be claimed

- **WHEN** the viewer presses an avoided chip and chooses to claim it instead
- **THEN** the skill SHALL be added to the profile's skills and removed from its excluded skills,
  and the chip SHALL move to You have

### Requirement: An avoid is confirmed, reversible and rolled back on failure

A successful avoid SHALL be confirmed by a line naming the skill and what happened to it, offering
undo — the same affordance a claim gets, and distinguishable from it in wording. A failed write
SHALL leave the profile and the chip as they were and state the failure.

#### Scenario: Confirmation distinguishes the two writes

- **WHEN** the viewer avoids `wordpress`
- **THEN** the confirmation SHALL say the skill was added to the skills the viewer avoids, not that
  it was added to their profile skills

#### Scenario: Undo lifts the avoid

- **WHEN** the viewer undoes an avoid
- **THEN** the skill SHALL leave the excluded set and the chip SHALL stop rendering as avoided

#### Scenario: A failed avoid changes nothing

- **WHEN** the profile write for an avoided skill fails
- **THEN** the chip SHALL render as it did before, no confirmation SHALL be shown, and an error
  naming the failure SHALL be shown

