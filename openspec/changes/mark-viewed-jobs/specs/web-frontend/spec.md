## ADDED Requirements

### Requirement: Already-viewed jobs are visually marked in the browse list

The SPA SHALL visually de-emphasise job cards that the signed-in user has already
viewed, in both the jobs list and the search results, so they can tell at a
glance what they have already opened. The marking SHALL be driven by the set of
viewed slugs read from `GET /api/v1/me/jobs/viewed`, loaded once when a signed-in
user opens the browse view. A viewed card SHALL be dimmed (reduced opacity) and
SHALL return to full strength on hover to signal it remains clickable. For
anonymous (signed-out) visitors no card SHALL be dimmed. Surfaces where every
listed job is by definition already viewed (the My Jobs history and board) SHALL
NOT dim their cards.

#### Scenario: Signed-in user sees viewed jobs dimmed

- **WHEN** a signed-in user who has viewed some jobs opens the jobs list or runs
  a search
- **THEN** the cards for jobs in their viewed-slug set are rendered dimmed
- **AND** cards for jobs they have not viewed are rendered at full strength

#### Scenario: Hovering a viewed card restores it

- **WHEN** the user hovers a dimmed (viewed) job card
- **THEN** the card returns to full strength while hovered

#### Scenario: Anonymous visitor sees no dimming

- **WHEN** a signed-out visitor opens the jobs list or runs a search
- **THEN** no job card is dimmed

#### Scenario: Opening a job marks it viewed without a reload

- **WHEN** a signed-in user opens a job from the list and navigates back
- **THEN** that job's card is shown dimmed without requiring a full reload

#### Scenario: My Jobs surfaces are not dimmed

- **WHEN** a signed-in user opens the My Jobs history or board, where every card
  is already viewed
- **THEN** no card is dimmed
