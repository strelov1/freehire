## ADDED Requirements

### Requirement: Primary CTAs sit under the title, not on the tab row

The job detail page SHALL render its call-to-action buttons (the external apply link and,
where offered, auto-apply) in the header block, on a row of their own under the job title
and right-aligned, and SHALL NOT render them on the row that carries the content tabs. The tab row SHALL carry only the quiet
per-posting actions — Discussion, Report, Save, Add-to-list — so that the content TabStrip
keeps enough width to show its labels without scrolling on a desktop column.

This applies from the `lg` breakpoint up. Below `lg` the furniture differs but the rule does
not: the sticky bottom bar carries whichever control the page made primary, and the quiet
strip under the title carries the other one.

#### Scenario: Desktop tab row is not starved by the actions

- **WHEN** a signed-in reader opens a Greenhouse posting on a desktop-width viewport
- **THEN** the tab row shows every content tab label in full
- **AND** neither the auto-apply button nor the external apply button appears on that row

#### Scenario: The CTAs keep their own row

- **WHEN** the job detail page renders at `lg` or wider
- **THEN** the CTA buttons sit on a row between the title and the content tabs
- **AND** they are right-aligned, whatever the length of the title above them

#### Scenario: Quiet actions stay on the tab row

- **WHEN** the job detail page renders at `lg` or wider
- **THEN** Discussion, Report, Save and Add-to-list render to the right of the content tabs
- **AND** they share the single rule drawn under the tab row

#### Scenario: The phone's sticky bar carries the primary control

- **WHEN** the page renders narrower than `lg` on a posting whose auto-apply can be started
- **THEN** the sticky bottom bar carries the auto-apply button, with its `Pro` marker
- **AND** the apply link renders in the quiet strip under the title instead

#### Scenario: The phone falls back to the apply link

- **WHEN** the page renders narrower than `lg` and auto-apply is not the primary control
- **THEN** the sticky bottom bar carries the apply link, at whatever rank the page gave it
- **AND** the quiet strip does not repeat it

### Requirement: Auto-apply is the primary CTA when it can be started

When auto-apply is offered for a posting and is in its clickable state, the job detail page
SHALL render it as the page's primary call to action — the brand fill that the external
apply button otherwise carries — and SHALL render a `Pro` marker inside the button naming
the plan the action requires.

Every non-clickable auto-apply state SHALL keep the quiet, disabled treatment it has today
and SHALL NOT carry the `Pro` marker: a filled button that cannot be pressed misstates what
the reader can do.

Eligibility is deliberately not pre-empted client-side. A reader without Pro still sees the
primary button and learns why the action is unavailable from the backend's own refusal
after clicking.

#### Scenario: Clickable auto-apply is the primary CTA

- **WHEN** a Greenhouse posting has no prior auto-apply attempt and the reader has not applied
- **THEN** the auto-apply button renders with the primary (brand fill) treatment
- **AND** it carries a `Pro` marker
- **AND** it is enabled

#### Scenario: A standing or spent attempt is not a primary CTA

- **WHEN** the reader already has a live auto-apply attempt for the posting
- **THEN** the auto-apply button renders quiet and disabled, reading `Auto-apply queued`
- **AND** it carries no `Pro` marker

#### Scenario: Auto-apply is absent where it is not offered

- **WHEN** the posting did not come from a source auto-apply can drive
- **THEN** no auto-apply button renders anywhere on the page

### Requirement: The external apply button yields its rank to auto-apply

The external apply button SHALL be demoted to an outline treatment labelled `Show origin`
whenever auto-apply is the posting's primary CTA or an auto-apply attempt is queued. In
every other case it SHALL keep its primary (brand fill) treatment and its `Apply` label.

Every bar on the page that renders the external button beside an auto-apply button SHALL
give it the same treatment the title row does. One link SHALL NOT read at two ranks on one
page.

The page SHALL NOT render two primary CTAs at once, and SHALL render one in every state
where the reader still has an action left to take. A queued attempt is the one state where
none is rendered: a submission is in flight, and a loud button would only invite a
duplicate.

A reader who already applied to the posting by hand SHALL NOT change the external button.
That fact is true of a posting from any source, and demoting on it would make a posting
auto-apply can drive read differently from an identical one it cannot, for a reader in the
identical situation.

Demotion changes only the button's label and treatment. Its destination, its
`nofollow noopener noreferrer` rel, its new-tab target, and the apply-intent tracking and
"Did you apply?" prompt its click raises SHALL be unchanged.

#### Scenario: Demoted beside a clickable auto-apply

- **WHEN** auto-apply is offered and clickable for the posting
- **THEN** the external button reads `Show origin` with an outline treatment

#### Scenario: Demoted while an attempt stands

- **WHEN** an auto-apply attempt for the posting is queued
- **THEN** the external button reads `Show origin` with an outline treatment

#### Scenario: The pinned header agrees with the title row

- **WHEN** the reader scrolls past the title on a posting auto-apply can drive
- **THEN** the pinned header carries the same two buttons, with the same labels and treatments

#### Scenario: Promoted when auto-apply will not act

- **WHEN** the posting's auto-apply attempt was declined by the reader or failed
- **THEN** the external button reads `Apply` with the primary (brand fill) treatment
- **AND** it is the only primary CTA on the page

#### Scenario: No primary CTA while a submission is in flight

- **WHEN** an auto-apply attempt for the posting is queued
- **THEN** neither the auto-apply button nor the apply link carries the primary (brand fill) treatment

#### Scenario: Applying by hand does not demote anything

- **WHEN** the reader has already applied to the posting themselves
- **THEN** the external button reads `Apply` with the primary (brand fill) treatment
- **AND** it reads the same way whether or not auto-apply can drive the posting

#### Scenario: Unchanged where auto-apply is not offered

- **WHEN** the posting did not come from a source auto-apply can drive
- **THEN** the external button reads `Apply` with the primary (brand fill) treatment

#### Scenario: Demotion does not change what the click does

- **WHEN** the reader clicks the external button while it reads `Show origin`
- **THEN** the posting's own URL opens in a new tab with `nofollow noopener noreferrer`
- **AND** the apply-intent event fires and the "Did you apply?" prompt is raised, exactly as for `Apply`
