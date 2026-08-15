# extension-apply-plan Specification

## Purpose

What the browser extension's side panel shows about the application form on the page in
front of the candidate: the questions it asks, which of them are answered, how far along
the ones that gate submission are, and what the panel does while autofill works through
them.

It covers the account of the form and the acts that follow from it — the field-by-field
walk, reaching a question from the panel, and keeping the count honest as the candidate
types. It does not cover what may be written into a form or where those values come from
(`extension-autofill` — the contact block the server assembles), nor how the panel reaches
the page at all (`extension-auth` for the session, the browser-tool wire for the transport).

## Requirements
### Requirement: The panel accounts for the application form on the open page

The side panel SHALL show a checklist of the questions the open page's application form
asks, built from the same question unit the filler works in — a question rendered as
several controls (a legend over 29 country checkboxes) SHALL appear once, not once per
control.

Each item SHALL carry the question's own label, whether the form marks it required, and
whether it is currently answered. An answered question is one whose control already holds
a value, whoever put it there: the panel, the agent, the page's own prefill, or the user.

The checklist SHALL be shown only where the page actually offers an application form; a
page with no form SHALL show no checklist rather than an empty one.

#### Scenario: The checklist describes the form before anything is filled

- **WHEN** the panel is open on a page whose application form asks eight questions, three of
  them already carrying values from the page's own prefill
- **THEN** the checklist lists eight items, marks those three answered, and marks the rest
  unanswered

#### Scenario: A grouped question is one item

- **WHEN** the form asks one question rendered as a group of checkboxes under a single legend
- **THEN** the checklist carries one item for it, labelled by the legend

#### Scenario: A page with no application form shows no checklist

- **WHEN** the panel is open on a page that offers no application form
- **THEN** no checklist and no counter are shown

### Requirement: The counter measures what gates submission

The panel SHALL show how many of the form's REQUIRED questions are answered, as a count and
as a percentage, with a progress bar over the same figure. Optional questions SHALL be
listed but SHALL NOT count toward it.

A form that marks no question required SHALL show no counter — a percentage over zero
required questions states nothing.

#### Scenario: The counter counts required questions only

- **WHEN** the form asks seven required questions of which six are answered, plus four
  optional ones of which none are
- **THEN** the panel shows six of seven, 86%

#### Scenario: A form with no required questions shows no counter

- **WHEN** every question on the form is optional
- **THEN** the checklist is shown and no counter is

### Requirement: Autofill walks the form in view

When autofill runs, the panel SHALL work through the questions one at a time rather than in
one silent batch. For each question it fills, the page SHALL scroll that question into view
and the control SHALL be outlined while its value is written, and the panel SHALL mark the
item answered and advance the counter before moving to the next.

The walk SHALL be interruptible: while it runs, the autofill control SHALL offer to stop it,
and stopping SHALL leave every value already written in place.

A question that has disappeared from the page since the plan was made SHALL be skipped
without ending the walk.

#### Scenario: Each filled question is shown as it is filled

- **WHEN** autofill has three questions to answer
- **THEN** the page scrolls to each in turn, each is outlined as its value is written, and
  the checklist ticks each one off before the next is started

#### Scenario: The walk can be stopped

- **WHEN** the user stops a walk after two of five questions have been answered
- **THEN** the walk ends, those two values remain on the page, and the checklist shows two
  answered

#### Scenario: A question that vanished mid-walk is skipped

- **WHEN** the form re-renders during a walk and drops a question the plan named
- **THEN** that item is left unanswered, the walk continues with the next question, and the
  closing summary names what it could not answer

### Requirement: An unanswered question is reachable from the panel

The panel SHALL let the user jump to any question in the checklist: acting on an item SHALL
scroll the page to that question and place the cursor in it, so a question autofill could
not answer can be answered by hand without hunting for it.

#### Scenario: Acting on an unanswered item goes to it

- **WHEN** the user acts on an unanswered checklist item
- **THEN** the page scrolls that question into view and its control takes focus

#### Scenario: A question the page no longer has reports itself

- **WHEN** the user acts on an item whose question is no longer on the page
- **THEN** the panel says so rather than silently doing nothing

### Requirement: The account follows what the user types

The checklist and the counter SHALL reflect answers the user enters on the page themselves,
without the user returning to the panel to refresh it.

#### Scenario: Typing on the page ticks the item off

- **WHEN** the user types an answer into a question the panel listed as unanswered
- **THEN** the checklist marks it answered and the counter advances

#### Scenario: Clearing an answer counts it back

- **WHEN** the user empties a question the panel listed as answered
- **THEN** the checklist marks it unanswered and the counter falls
