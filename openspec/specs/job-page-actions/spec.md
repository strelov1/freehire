# job-page-actions Specification

## Purpose
How the job detail page ranks and places what a reader can do with a posting: which
control is the page's single primary call to action, where the CTAs sit relative to the
title and the content tabs, and how the pair changes as auto-apply becomes available,
starts, completes or gives up.
## Requirements
### Requirement: Primary CTAs sit under the title, not on the tab row

The job detail page SHALL render its call-to-action buttons (`Tailor my CV`, the external
apply link and, where offered, auto-apply) in the header block, on a row of their own under
the job title and right-aligned, and SHALL NOT render them on the row that carries the content
tabs. The tab row SHALL carry only the quiet per-posting actions — Discussion, Report, Save,
Add-to-list — so that the content TabStrip keeps enough width to show its labels without
scrolling on a desktop column.

On that row the controls SHALL read left to right in ascending rank: auto-apply where offered,
then the external apply link, then `Tailor my CV`, so the brand fill lands at the row's end.

This applies from the `lg` breakpoint up. Below `lg` the furniture differs but the rule does
not: the sticky bottom bar carries `Tailor my CV` plus whichever single control the page made
the offered way to apply, and the quiet strip under the title carries the other one.

#### Scenario: Desktop tab row is not starved by the actions

- **WHEN** a signed-in reader opens a Greenhouse posting on a desktop-width viewport
- **THEN** the tab row shows every content tab label in full
- **AND** none of the auto-apply, external apply or `Tailor my CV` buttons appears on that row

#### Scenario: The CTAs keep their own row

- **WHEN** the job detail page renders at `lg` or wider
- **THEN** the CTA buttons sit on a row between the title and the content tabs
- **AND** they are right-aligned, whatever the length of the title above them
- **AND** `Tailor my CV` is the last of them

#### Scenario: Quiet actions stay on the tab row

- **WHEN** the job detail page renders at `lg` or wider
- **THEN** Discussion, Report, Save and Add-to-list render to the right of the content tabs
- **AND** they share the single rule drawn under the tab row

#### Scenario: The phone's sticky bar carries the primary control

- **WHEN** the page renders narrower than `lg` on a posting whose auto-apply can be started
- **THEN** the sticky bottom bar carries `Tailor my CV` and the auto-apply button, with its `Pro` marker
- **AND** the apply link renders in the quiet strip under the title instead

#### Scenario: The phone falls back to the apply link

- **WHEN** the page renders narrower than `lg` and auto-apply is not the offered way to apply
- **THEN** the sticky bottom bar carries `Tailor my CV` and the apply link
- **AND** the quiet strip does not repeat the apply link

### Requirement: The external apply button yields its rank to auto-apply

The external apply button SHALL be labelled `Show origin` whenever auto-apply is the posting's
offered way to apply or an auto-apply attempt is queued, and SHALL read `Apply` in every other
case.

It SHALL render with an outline treatment in every state. The brand fill belongs to
`Tailor my CV`, so the apply link's rank is now carried by its label and its place in the row,
not by its colour.

Every bar on the page that renders the external button beside an auto-apply button SHALL give
it the same label the title row does. One link SHALL NOT read at two ranks on one page.

The page SHALL NOT render two primary CTAs at once. `Tailor my CV` is the one primary CTA, in
every state including a queued auto-apply attempt: a tailoring session is not a second
submission, so a loud button there invites no duplicate.

A reader who already applied to the posting by hand SHALL NOT change the external button's
label. That fact is true of a posting from any source, and relabelling on it would make a
posting auto-apply can drive read differently from an identical one it cannot, for a reader in
the identical situation.

Relabelling changes only the button's word. Its destination, its `nofollow noopener noreferrer`
rel, its new-tab target, and the apply-intent tracking and "Did you apply?" prompt its click
raises SHALL be unchanged.

#### Scenario: Demoted beside a clickable auto-apply

- **WHEN** auto-apply is offered and clickable for the posting
- **THEN** the external button reads `Show origin` with an outline treatment

#### Scenario: Demoted while an attempt stands

- **WHEN** an auto-apply attempt for the posting is queued
- **THEN** the external button reads `Show origin` with an outline treatment

#### Scenario: The pinned header agrees with the title row

- **WHEN** the reader scrolls past the title on a posting auto-apply can drive
- **THEN** the pinned header carries the same buttons, with the same labels and treatments

#### Scenario: Promoted when auto-apply will not act

- **WHEN** the posting's auto-apply attempt was declined by the reader or failed
- **THEN** the external button reads `Apply` with an outline treatment

#### Scenario: No primary CTA while a submission is in flight

- **WHEN** an auto-apply attempt for the posting is queued
- **THEN** neither the auto-apply button nor the apply link carries the primary (brand fill) treatment
- **AND** `Tailor my CV` still does

#### Scenario: Applying by hand does not demote anything

- **WHEN** the reader has already applied to the posting themselves
- **THEN** the external button reads `Apply` with an outline treatment
- **AND** it reads the same way whether or not auto-apply can drive the posting

#### Scenario: Unchanged where auto-apply is not offered

- **WHEN** the posting did not come from a source auto-apply can drive
- **THEN** the external button reads `Apply` with an outline treatment

#### Scenario: Demotion does not change what the click does

- **WHEN** the reader clicks the external button while it reads `Show origin`
- **THEN** the posting's own URL opens in a new tab with `nofollow noopener noreferrer`
- **AND** the apply-intent event fires and the "Did you apply?" prompt is raised, exactly as for `Apply`

### Requirement: Tailoring the CV is the page's single primary CTA

The job detail page SHALL render a `Tailor my CV` button in its call-to-action row and SHALL
give it the primary (brand fill) treatment in every state. No other control in the
call-to-action row — the external apply link, the auto-apply button — SHALL carry the brand
fill, at any of the three widths that row is rendered at.

The rule is about the CTA row, not the viewport. Elsewhere on the page a brand-filled button
still means "the one thing to do HERE": the sidebar's `Upload CV` when there is no CV to
match against, the `Sign in` in its locked teaser, `Yes, save` in the "Did you apply?"
prompt. None of those competes with the CTA row — each is the single action of a block that
is showing the reader something else entirely.

The button SHALL appear in every place the CTA row is rendered — under the title from `lg` up,
in the pinned header once the title has scrolled away, and in the sticky bottom bar below `lg`
— so the page's primary action does not disappear as the reader scrolls or narrows the
viewport.

Pressing it SHALL raise the pre-flight tailoring confirmation (the deterministic skill and
requirement check) and navigate to the tailoring workspace only on confirmation, which is what
the sidebar's button did before this change. An unauthenticated reader SHALL be offered
sign-in in place instead, and SHALL NOT be navigated to the tailoring workspace.

The button SHALL be withheld only once the page knows the reader has no stored CV. While that
is unknown — including for an unauthenticated reader — it SHALL be rendered, because the offer
is what brings a reader to sign in.

What the action costs and whether today's allowance is spent SHALL be stated by the
confirmation dialog, not beside the button: the dialog already carries both, and it says them
at the moment the reader commits.

#### Scenario: The primary CTA is the tailoring button

- **WHEN** a signed-in reader with a stored CV opens any posting at `lg` or wider
- **THEN** the `Tailor my CV` button renders with the primary (brand fill) treatment
- **AND** it is the only control in the call-to-action row carrying that treatment

#### Scenario: It survives the scroll and the pinned header

- **WHEN** the reader scrolls past the title
- **THEN** the pinned header carries the `Tailor my CV` button with the same label and treatment

#### Scenario: The phone's sticky bar carries it

- **WHEN** the page renders narrower than `lg`
- **THEN** the sticky bottom bar carries the `Tailor my CV` button with the primary treatment
- **AND** it shares that bar with exactly one apply control

#### Scenario: Pressing it confirms before navigating

- **WHEN** a signed-in reader presses `Tailor my CV`
- **THEN** the pre-flight confirmation dialog opens showing the deterministic skill and requirement check
- **AND** the reader reaches the tailoring workspace only after confirming

#### Scenario: A guest is offered sign-in

- **WHEN** an unauthenticated reader presses `Tailor my CV`
- **THEN** the sign-in dialog opens, the reader stays on the job page, and no tailoring request is issued

#### Scenario: Withheld from a reader with no CV

- **WHEN** the page has read that the signed-in reader has no stored CV
- **THEN** the `Tailor my CV` button is absent from every CTA row on the page

#### Scenario: The allowance is not repeated beside the button

- **WHEN** the `Tailor my CV` button renders
- **THEN** no count of remaining tailorings and no spent-allowance message renders beside it

### Requirement: Auto-apply is the offered way to apply when it can be started

When auto-apply is offered for a posting and is in its clickable state, the job detail page
SHALL treat it as the posting's offered way to apply: it takes the phone's sticky bottom bar,
and the external link is demoted to `Show origin` and moved into the quiet strip under the
title. It SHALL carry a `Pro` marker naming the plan the action requires, and SHALL render
with a secondary treatment — never the brand fill, which belongs to `Tailor my CV`.

Every non-clickable auto-apply state SHALL keep the quiet, disabled treatment it has today,
SHALL NOT carry the `Pro` marker, and SHALL NOT be the offered way to apply.

Eligibility is deliberately not pre-empted client-side. A reader without Pro still sees the
button and learns why the action is unavailable from the backend's own refusal after clicking.

#### Scenario: Clickable auto-apply is the offered way to apply

- **WHEN** a Greenhouse posting has no prior auto-apply attempt and the reader has not applied
- **THEN** the auto-apply button renders enabled with a secondary (non-brand-fill) treatment
- **AND** it carries a `Pro` marker
- **AND** it takes the phone's sticky bottom bar beside `Tailor my CV`

#### Scenario: A standing or spent attempt is not the offered way to apply

- **WHEN** the reader already has a live auto-apply attempt for the posting
- **THEN** the auto-apply button renders quiet and disabled, reading `Auto-apply queued`
- **AND** it carries no `Pro` marker
- **AND** the phone's sticky bottom bar carries the external apply link instead

#### Scenario: Auto-apply is absent where it is not offered

- **WHEN** the posting did not come from a source auto-apply can drive
- **THEN** no auto-apply button renders anywhere on the page

