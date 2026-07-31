# ghost-job-signal Specification

## Purpose
TBD - created by archiving change ghost-job-signal. Update Purpose after archive.
## Requirements
### Requirement: A job carries a ghost signal derived from four criteria in two strength tiers

The system SHALL derive, for every open job, a `ghost` signal consisting of a level, the set of
criteria that fired, and the total number of criteria considered. The criteria SHALL be exactly
four, in two tiers that differ by what they observe:

| Criterion | Tier | Fires when |
|---|---|---|
| `evergreen_posting` | structural | the job's `jobreality` class is `likely-evergreen` |
| `ats_absent` | structural | the job comes from an aggregator source and its role is absent from the company's own crawled board |
| `silent_applications` | outcome | applications tracked here have passed their stage's silence threshold with no reply |
| `user_reports` | outcome | people have stated they applied and received no answer |

Structural criteria describe the **shape of a posting**; outcome criteria describe **what happened
to someone who applied**. The tiers are not interchangeable, and the distinction is what the level
rules below rest on.

The derivation SHALL be deterministic — no randomness, no LLM call — and SHALL reuse the existing
`jobreality` classification verbatim rather than restating its logic.

#### Scenario: A job with no criteria carries no signal

- **WHEN** a job's reality class is not `likely-evergreen`, it has no absence stamp, and no outcome evidence exists
- **THEN** its ghost level is `none` and the signal is omitted from the served payload

#### Scenario: The evergreen criterion mirrors the reality verdict

- **WHEN** a job's `jobreality` class is `likely-evergreen`
- **THEN** the `evergreen_posting` criterion fires, with no separate age or repost rule of its own

#### Scenario: The derivation is stable for identical input

- **WHEN** the same evidence and clock are classified twice
- **THEN** the level and the set of fired criteria are identical

### Requirement: Structural evidence alone can never reach the higher level

The system SHALL judge the evidence against two separate gates:

- **convergence** — at least two criteria fired, of any tier;
- **witnesses** — outcome evidence came from **at least two distinct people**.

The level SHALL be `likely` when both gates pass, `possible` when exactly one passes, and `none`
when neither does. Structural criteria, however many fire, MUST NOT produce `likely`, since they
observe the shape of a posting and can witness nothing about it.

Outcome evidence passing the witness gate SHALL reach `possible` even as a lone criterion. Two
strangers independently reporting that nobody answered is a stronger fact than any two artifacts
of posting shape, and requiring structural corroboration would leave the outcome tier — the only
one that observes reality — unable to mark anything by itself.

Two independent constraints force the same two-person threshold, and both must hold: a count of
one identifies the single applicant to the employer, and one account must not be able to mark a
posting on its own.

#### Scenario: One criterion is not enough

- **WHEN** a job's only fired criterion is `evergreen_posting`
- **THEN** its ghost level is `none`

#### Scenario: Two structural criteria reach possible

- **WHEN** `evergreen_posting` and `ats_absent` both fire and no outcome evidence exists
- **THEN** the ghost level is `possible`

#### Scenario: Structural convergence never reaches likely

- **WHEN** both structural criteria fire and outcome evidence comes from fewer than two distinct people
- **THEN** the ghost level is `possible`, never `likely`

#### Scenario: Outcome evidence from two people reaches likely

- **WHEN** two distinct people contribute outcome evidence and at least one other criterion fires
- **THEN** the ghost level is `likely`

#### Scenario: Outcome evidence alone reaches possible but not likely

- **WHEN** two distinct people contribute outcome evidence through the same channel and no other criterion fires
- **THEN** the ghost level is `possible` — the witness gate passes, the convergence gate does not

#### Scenario: One person firing both outcome criteria is still one witness

- **WHEN** a single user has both a silent tracked application and a filed report on a job, and no structural criterion fires
- **THEN** two criteria fire but the ghost level is `possible`, because the gate counts people rather than criteria

### Requirement: One person contributes at most one unit of outcome evidence

The system SHALL count **distinct people** across both outcome channels together, not rows. A
person who both applied through freehire and filed a report contributes one unit, not two.

#### Scenario: The same person on both channels counts once

- **WHEN** one user has a silent tracked application on a job and has also filed a ghost report for it
- **THEN** the job's distinct-contributor count is one, and the level cannot be `likely`

#### Scenario: Two people on different channels reach the gate

- **WHEN** one user has a silent tracked application and a different user has filed a report
- **THEN** the distinct-contributor count is two and the outcome gate is met

### Requirement: Application silence counts as evidence only for a connected mailbox

The system SHALL count a tracked application as outcome evidence only when its owner has a
connected mailbox — a `connected` Gmail connection or an allocated hosted mailbox. For a user with
no connected mailbox, the silence derivation falls back to the apply date because no mail can ever
be linked, so every such application reads silent once the threshold passes, whether or not the
employer answered.

Absence of a reply is evidence only where a reply would have been observed. Everywhere else it is
a gap in our data, and reporting a gap as a silence would tell a person they were ignored when
they were not.

#### Scenario: Silence without a connected mailbox is not evidence

- **WHEN** a user with no Gmail connection and no hosted mailbox has an application 30 days past its threshold
- **THEN** that application contributes no outcome evidence, though the user's own tracking board still marks it silent

#### Scenario: Silence with a connected mailbox is evidence

- **WHEN** a user with a connected mailbox has an application 30 days past its threshold and no linked reply
- **THEN** that application contributes one unit of outcome evidence

#### Scenario: A linked reply withdraws the evidence

- **WHEN** mail is linked to a previously silent application, moving its last activity inside the threshold
- **THEN** the application stops contributing outcome evidence

#### Scenario: A terminal application contributes nothing

- **WHEN** an application is in a terminal stage (`rejected`, `accepted`, `withdrawn`)
- **THEN** it contributes no outcome evidence, since a settled application awaits no reply

### Requirement: The ATS cross-check fires only under a coverage gate and expires

The system SHALL stamp `ats_absent` on a job only when all of the following hold: the job's source
is of kind `aggregator`; the company has at least one open job from a source of kind `ats` or
`company`; and no open job of that company from such a source shares the job's role key. The role
key SHALL be the company slug together with the normalized, trailing-clause-stripped title — not
the full role fingerprint, whose description component does not survive an aggregator's rewriting.

A stamp older than 14 days SHALL be ignored. The worker re-stamps on every run, so an expired
stamp means the worker has stopped — and a stopped worker must fall silent rather than keep
accusing the catalogue from a frozen snapshot.

#### Scenario: No company board means no evidence

- **WHEN** a company appears only on aggregator sources and has no crawled board of its own
- **THEN** no job of that company carries the `ats_absent` criterion, whatever its titles

#### Scenario: The role is present on the company's own board

- **WHEN** an aggregator posting's role key matches an open job of the same company from an ATS source
- **THEN** the absence stamp is cleared and the criterion does not fire

#### Scenario: The role is absent from a board we do crawl

- **WHEN** a company has open jobs from its own ATS board and an aggregator posting's role key matches none of them
- **THEN** the posting is stamped absent and the criterion fires

#### Scenario: A description rewritten by the aggregator still matches

- **WHEN** an aggregator lists a role whose description it truncated, while the company's board carries the full text under the same title
- **THEN** the role keys match and the criterion does not fire

#### Scenario: An expired stamp yields no evidence

- **WHEN** a job's absence stamp is older than 14 days
- **THEN** the `ats_absent` criterion does not fire

### Requirement: Outcome counts are withheld below the anonymity gate

The served ghost payload SHALL carry the outcome counts only when at least two distinct people
have contributed. Below that threshold the count fields SHALL be absent from the response
entirely, rather than present and zeroed or rounded.

Absence is what makes the guarantee structural: a future handler has nothing to remember to
redact, because there is nothing to redact.

#### Scenario: A single contributor's counts are not served

- **WHEN** exactly one person has contributed outcome evidence for a job
- **THEN** the served payload contains no outcome count fields

#### Scenario: Counts appear once the gate is met

- **WHEN** two or more distinct people have contributed
- **THEN** the served payload reports the number of distinct contributors

### Requirement: A user files and retracts a ghost report

An authenticated user with a verified email address SHALL be able to record, for one open job,
that they applied on a stated date and received no answer, and SHALL be able to retract it. A
retracted report contributes no evidence.

The claim SHALL contribute evidence only once 21 days — the measured `applied` silence threshold —
have elapsed since the stated apply date. A stated date in the future, or more than 12 months
past, SHALL be rejected: a year-old application says nothing about whether a posting is live now.

This channel is deliberately separate from `job_reports`. That queue exists so a moderator can
**close** a job, and a report that is merely evidence cannot be expressed as a close.

#### Scenario: A report is filed and matures

- **WHEN** a verified user reports having applied 25 days ago with no answer
- **THEN** the report is recorded and contributes one unit of outcome evidence

#### Scenario: A fresh claim does not yet count

- **WHEN** a user reports having applied 5 days ago
- **THEN** the report is recorded but contributes no evidence until 21 days have passed

#### Scenario: Retraction withdraws the evidence

- **WHEN** the employer answers and the user retracts their report
- **THEN** the report stops contributing evidence and the level is recomputed without it

#### Scenario: A second report by the same user is refused

- **WHEN** a user who already has an active report for a job files another
- **THEN** the request is refused as a conflict and the evidence count is unchanged

#### Scenario: A closed job cannot be reported

- **WHEN** a user files a ghost report for a job that is already closed
- **THEN** the request is refused — there is nothing left to warn anyone about

#### Scenario: An unverified account cannot file

- **WHEN** a signed-in user whose email is not verified files a ghost report
- **THEN** the request is refused

#### Scenario: A user filing en masse is rate-limited

- **WHEN** a user files more ghost reports in a day than the daily cap allows
- **THEN** further requests are refused until the window resets

### Requirement: The interface states criteria, never an accusation

The web SPA SHALL present the signal as a hedged statement — wording that the job may be inactive,
never that it is a ghost or that the employer is not hiring — accompanied by the number of criteria
that fired out of the total.

In a list card the presentation SHALL be a text chip carrying that wording and scale. On the job page
it SHALL instead be a single row: a segmented gauge, the hedged wording, the scale, and a disclosure
control that expands the full checklist in place. The checklist SHALL NOT be expanded by default, and
the job page SHALL carry the signal exactly once — the row replaces both the card-style chip and the
separate always-open panel.

The expanded checklist SHALL account for every criterion the classifier weighs. Each criterion that
fired SHALL be listed, carrying the facts behind it where the payload supplies any — for a criterion
whose firing IS the fact the interface SHALL add nothing further.

The criteria that did not fire SHALL be named together in a single line, rather than one line each,
and that line SHALL claim only that they were not OBSERVED. It MUST NOT state that there is no data
on them: the payload reports which criteria fired and never distinguishes a criterion checked and
found clear from one with nothing to check, so `evergreen_posting` — derived from a class computed
for every job — is never a criterion the system lacks data on. This is the same constraint that
governs the gauge's uncoloured segments, and prose beside the gauge must not concede what the colour
withholds.

The checklist SHALL carry a link to the page explaining the signal, which states the standing caveat
that these are observations about the posting rather than a claim about the employer.

The gauge SHALL have one segment per criterion the classifier weighs and SHALL colour exactly the
number that fired, in a tone that escalates with that number. An uncoloured segment SHALL carry no
claim about the criterion behind it: the served payload distinguishes a criterion that fired from one
that did not, and never a criterion checked and found clear from one with no data at all, so the
gauge MUST NOT render an uncoloured segment in a tone that reads as reassurance.

The word *ghost* SHALL remain internal to the codebase — package name, API field, criterion codes —
and MUST NOT appear in the interface. "Open 240 days, found only on an aggregator" is a claim about
observable facts; "ghost job" is a claim about an employer's intent, which the system cannot
observe.

Where the ghost signal is present it SHALL supersede the reality badge, which would otherwise
restate the `evergreen_posting` criterion as a second, louder badge for the same fact. Where the
ghost level is `none`, the reality badge renders unchanged.

#### Scenario: The chip is hedged and carries the scale

- **WHEN** a job reaches level `possible` with two of four criteria
- **THEN** the card shows a hedged chip and a two-of-four scale, and no accusatory wording

#### Scenario: The job page states the signal once, as a gauge row

- **WHEN** a user opens a job page at level `possible` with two of four criteria
- **THEN** one row renders with two of four segments coloured, the hedged wording and the scale, and no separate panel repeating the same signal

#### Scenario: The checklist is disclosed on demand

- **WHEN** a user opens a job page carrying the signal
- **THEN** the criteria are collapsed behind a disclosure control, and activating it accounts for every criterion and links to the explanation

#### Scenario: The checklist explains what did not fire

- **WHEN** a user expands the checklist on a job whose outcome criteria did not fire
- **THEN** one line names those criteria as not observed, so the reader can see why the level is not higher

#### Scenario: The summary does not claim an absence of data

- **WHEN** the criteria that did not fire include one the system evaluates for every job
- **THEN** the line says they were not observed, and does not say that there is no data on them

#### Scenario: The caveat is reachable from the checklist

- **WHEN** a user expands the checklist
- **THEN** it links to the page explaining the signal, where the observations-not-accusations caveat and the limits of each criterion are stated

#### Scenario: An uncoloured segment claims nothing

- **WHEN** a job fires two of four criteria and the other two did not fire
- **THEN** the two uncoloured segments render in a neutral tone that does not read as those criteria having been checked and cleared

#### Scenario: The tone escalates with the count

- **WHEN** one job fires one criterion and another fires four
- **THEN** the four-of-four gauge renders in a stronger tone than the one-of-four gauge

#### Scenario: The ghost chip replaces the reality chip

- **WHEN** a job is both `likely-evergreen` and at ghost level `possible`
- **THEN** only the ghost signal renders, and the evergreen fact appears inside its checklist

#### Scenario: Reality is unaffected where ghost is silent

- **WHEN** a job is `likely-evergreen` and its ghost level is `none`
- **THEN** the reality badge renders exactly as before

### Requirement: The report dialog routes a no-response complaint to the evidence channel

The existing "Report this job" dialog SHALL, when the user picks the `no_response` reason, ask
when they applied and file a ghost report, rather than filing a moderation-queue report. The
other four reasons SHALL continue to reach the moderation queue unchanged.

One dialog, two destinations, and the user is told neither: they are describing what happened to
them, not choosing a routing. Today `no_response` reaches a queue whose only lever is closing the
job — a person says "nobody answered me" and the only available response is to delete the posting,
which helps nobody and is why such reports accumulate undecided.

The apply date is what the reason was always missing. Without it a no-response complaint cannot be
told from impatience, which is precisely why it could never be more than a moderator's judgement
call.

#### Scenario: No-response becomes evidence

- **WHEN** a user picks `no_response` and states when they applied
- **THEN** a ghost report is filed for that job and no moderation-queue report is created

#### Scenario: The other reasons are unchanged

- **WHEN** a user picks `not_relevant`, `spam`, `fraud` or `other`
- **THEN** a moderation-queue report is filed exactly as before

#### Scenario: The dialog asks for a date it can use

- **WHEN** a user picks `no_response`
- **THEN** the dialog asks for the apply date before it will submit, and refuses a future date

### Requirement: The verdict is computed on read and never stored

The system SHALL compute the ghost signal at read time from the evidence, and MUST NOT persist the
level or the criteria set. The evidence itself is stored or queried according to its nature: a
person's statement is a stored row, an application's silence is derived live from tracking and
linked mail, and the absence stamp is a dated fact the cross-check worker maintains.

A closed job SHALL carry no ghost signal.

#### Scenario: A threshold change needs no backfill

- **WHEN** a level or criterion threshold changes
- **THEN** the next read reflects it, with no stored column to migrate and no reindex required

#### Scenario: A closed job carries no signal

- **WHEN** a job with fired criteria is closed
- **THEN** its served payload omits the ghost signal

### Requirement: The explaining page previews the signal with the components that render it

The `/features/ghost-jobs` landing SHALL illustrate the signal by rendering the same components the
product renders, fed illustrative payloads, and MUST NOT reproduce their markup as a copy.

A copy is stale the moment the component is redesigned, and the page then describes an interface
that no longer exists — to a reader who has come to it precisely because they did not understand
what they saw. Screenshots fail the same way, and additionally freeze one theme and one employer's
name.

The page SHALL additionally state the level rule as a diagram rather than as prose: which
combinations of the two gates produce which wording, and that the strongest wording sits in exactly
one of them. Each cell's wording SHALL be derived from the rule rather than written into the
diagram, so the picture cannot caption a level the classifier stopped producing.

To derive it the frontend SHALL hold the level rule as a tested function, taking the criteria that
fired and the number of contributors, and that function SHALL live in the same module that already
declares itself the mirror of the classifier's constants. The page previously asserted the rule —
that structural evidence alone cannot reach the higher level — in a sentence no test could check, so
the claim could silently outlive the thresholds it describes. A mirrored rule can drift from the
classifier, but the constants it depends on were already mirrored in the frontend; expressing the
rule as a function makes the drift detectable instead of invisible, and gathers a mirror that was
spread across two modules into one.

#### Scenario: The preview follows a redesign

- **WHEN** the job page's presentation of the signal changes shape
- **THEN** the landing's preview changes with it, because it renders the same component rather than a copy of its markup

#### Scenario: The preview carries the caveat

- **WHEN** a reader reaches the landing's preview of the signal
- **THEN** the observations-not-accusations caveat is stated on the page beside it

#### Scenario: The ceiling is shown, not asserted

- **WHEN** a reader looks at the diagram of the two gates
- **THEN** the cell reached by posting shape alone carries the lower wording, and the strongest
  wording appears only in the cell that also required people who applied

#### Scenario: The diagram cannot caption a level the rule stopped producing

- **WHEN** the level rule changes which combination yields which wording
- **THEN** the diagram's cells change with it, because each is derived from the rule rather than
  written into the picture

#### Scenario: The level rule is pinned by test

- **WHEN** the frontend's level rule is exercised across the combinations of convergence and
  contributor gates
- **THEN** each combination yields the level the classifier defines, and no set of structural-only
  criteria yields the higher level

