# panel-contribute-page Specification

## Purpose
TBD - created by archiving change panel-contribute-page. Update Purpose after archive.
## Requirements
### Requirement: The panel offers the current page when no catalog job backs it

The panel SHALL show an "Add to freehire" action whenever the signed-in user is on a page
that resolved to no catalog job — both when a text-matched card is showing and when the
match area is empty. The action SHALL be absent while a curated catalog card is showing,
and absent when signed out.

#### Scenario: A page freehire does not carry offers the action

- **WHEN** the panel shows a text-matched card or the empty state for the current page
- **THEN** the "Add to freehire" action is available

#### Scenario: A curated card does not offer the action

- **WHEN** the panel shows the curated card of a catalog posting
- **THEN** the action is not shown

### Requirement: A resolved page replaces the ad-hoc card with the curated one

The panel SHALL, when the server answers with a catalog slug — whether the posting was
imported or already carried — reload the match for the current page so the curated card
replaces whatever was showing, without the user reloading the panel.

#### Scenario: An imported page becomes a curated card

- **WHEN** the user adds a page and the server answers with a slug
- **THEN** the panel loads that posting's curated match card

### Requirement: Every outcome is reported in the panel's own words

The panel SHALL report each outcome as a notice: a posting imported into the catalog, a
posting freehire already carried, a link queued for a maintainer to look at, and a failed
attempt. A failure SHALL leave the previous card intact.

#### Scenario: A queued link is reported without a card

- **WHEN** the server answers that the link was queued for triage
- **THEN** the panel says freehire will look at the link, and the previous card stays

#### Scenario: A failure leaves the panel as it was

- **WHEN** the request fails
- **THEN** the panel reports the failure and the previous card stays

