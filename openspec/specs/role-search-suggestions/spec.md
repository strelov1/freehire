# role-search-suggestions Specification

## Purpose

Offering the `role` facet from the header search box the moment someone types a role
name into it. Covers matching a query against the role catalogue, ranking and bounding
what is offered, and the dropdown's behaviour — which surfaces show it, what choosing a
suggestion does, and how the keyboard drives it.

Measured on five days of production access logs (71,174 searches, 8,340 distinct
queries): 37.6% of searches name a role, while the `role` facet appeared in 1.1% of
requests. The gap is discoverability, not relevance.
## Requirements
### Requirement: The header search offers role suggestions on jobs-backed lists only

The header list-search input SHALL render a suggestion dropdown when, and only
when, the registered list-search target publishes a suggestion capability. The
jobs views SHALL publish it; the companies list SHALL NOT. The header SHALL decide
purely from the published capability and SHALL NOT branch on the page path — the
same opt-in bridge pattern `filterScope` and `openFilters` already use.

The dropdown SHALL open on focus with an empty query, showing the curated entry
point the suggest endpoint serves, rather than requiring two characters before it
appears. Not knowing what to type is the state the dropdown exists to answer, and
a box that stays silent until the second keystroke never reaches it.

Suggestions SHALL come from `GET /api/v1/suggest`. The client SHALL NOT match
against a shipped catalogue of its own.

#### Scenario: Jobs feed offers suggestions
- **WHEN** a user types `product man` into the header search on the jobs feed
- **THEN** a dropdown offers `Product Manager` with its open-posting count

#### Scenario: Companies list offers none
- **WHEN** a user types `product man` into the header search on `/companies`
- **THEN** no suggestion dropdown appears

#### Scenario: The empty box suggests where to start
- **WHEN** a user focuses the header search on the jobs feed without typing
- **THEN** the dropdown opens with the curated specialization suggestions

#### Scenario: One character offers nothing
- **WHEN** the query is a single character
- **THEN** no completions are offered — one letter names nothing, and the starting points are what an EMPTY box answers, not a barely-started one

### Requirement: Free-text search is never taken away

The dropdown SHALL always offer, as its last row, running the typed text as a
free-text search. Pressing Enter while no suggestion is highlighted SHALL run the
free-text search — the dropdown SHALL NOT capture Enter by default.

Typing SHALL NOT run the search. The search SHALL run on Enter or on choosing a
suggestion, so the list is not refetched on every keystroke while the visitor is
still composing the query.

#### Scenario: Enter without a highlighted suggestion searches text
- **WHEN** the user types `revolut` and presses Enter without arrowing into the list
- **THEN** the free-text search for `revolut` runs and no facet is touched

#### Scenario: The text row is always present
- **WHEN** the dropdown is open with matches
- **THEN** a final row offers searching the typed text as text

#### Scenario: Typing alone does not refetch the list
- **WHEN** the user types four characters without pressing Enter
- **THEN** the result list below is not refetched

### Requirement: The dropdown is keyboard and dismissal complete

The dropdown SHALL be operable without a mouse: Down and Up SHALL move the
highlight through the offered rows — continuously across all sections, not within
one — Enter SHALL activate the highlighted row, and Escape SHALL close the
dropdown while leaving the typed text intact. Clicking outside the dropdown SHALL
close it. The dropdown SHALL expose its state to assistive technology using the
combobox/listbox roles and `aria-activedescendant`.

#### Scenario: Arrow keys and Enter select a role
- **WHEN** the user presses Down until `Data Analyst` is highlighted and presses Enter
- **THEN** the `data_analytics` role facet is applied

#### Scenario: The highlight crosses section boundaries
- **WHEN** the highlight is on the last completion row and the user presses Down
- **THEN** the highlight moves to the first job row, not back to the top

#### Scenario: Escape closes without losing the query
- **WHEN** the dropdown is open and the user presses Escape
- **THEN** the dropdown closes and the typed text remains in the input

#### Scenario: Clicking away closes the dropdown
- **WHEN** the dropdown is open and the user clicks elsewhere on the page
- **THEN** the dropdown closes

### Requirement: The dropdown shows completions, postings and companies

The dropdown SHALL render three sections in order: completions from the suggest
endpoint, matching job postings, and matching companies. Typing `google` SHALL
show Google's actual postings, not only the word "Google" — the postings are what
the visitor came for.

These rows matter more now than before, not less: with the search no longer
running as the visitor types, the list below is stale mid-query and these rows are
the only live evidence the query is finding anything.

The job and company sections SHALL reuse the existing launcher's data sources and
row rendering rather than introducing a second implementation.

#### Scenario: A company name shows its postings
- **WHEN** the user types `google`
- **THEN** the dropdown shows Google job postings above the Google company rows

#### Scenario: Each section is bounded
- **WHEN** many results match
- **THEN** the dropdown shows at most five completions, five postings and three companies

### Requirement: Company rows carry the company's mark

A `company` suggestion and a job row SHALL render the company's logo, through the
same logo resolution the launcher dropdown already uses. The recognisable mark is
what makes a company scannable in a list. A second logo path SHALL NOT be written.

A suggestion naming no employer — a title, a skill, a category — SHALL carry a
kind glyph instead.

#### Scenario: A company row shows its logo
- **WHEN** the dropdown offers the company Google
- **THEN** the row renders Google's logo beside the name

#### Scenario: A skill row shows no logo
- **WHEN** the dropdown offers the skill Java
- **THEN** the row renders a kind glyph, not a company logo

### Requirement: Choosing a suggestion applies what it names and keeps the typed text

Choosing a suggestion SHALL apply every facet part it names, through the list's
existing filter store, so the URL, the filter chips, the result list and the facet
counts update the way any other facet selection does. Off a list page the same pick
SHALL navigate to the feed carrying the same filter.

The typed text SHALL NOT be cleared. Progressive completion reads the recognised prefix
back out of the box on the next keystroke: clearing it would discard the part of the
query the visitor has already resolved, and `senior software engineer` followed by `go`
could never reach Google.

A suggestion of kind `title` names no facet and SHALL be applied as the free-text query
instead.

No suggestion applies the `role` facet: it no longer exists. A phrase that used to
resolve to a role resolves to the specialization it was built from, to a mined posting
title, or to both as separate rows.

#### Scenario: Selecting a specialization applies the facet and keeps the text
- **WHEN** the user chooses `Data Analytics` from the dropdown
- **THEN** `category=data_analytics` is applied and the typed text remains in the input

#### Scenario: The specialization reads as a normal filter afterwards
- **WHEN** a specialization has been applied from a suggestion
- **THEN** it appears as an active filter chip and can be removed like any other facet

#### Scenario: A composed suggestion applies both of its parts
- **WHEN** the user chooses a row naming a title and a company
- **THEN** the free-text query and `company_slug` are both applied

#### Scenario: No pick applies a role
- **WHEN** any suggestion is chosen
- **THEN** the resulting filters carry no `role` parameter

