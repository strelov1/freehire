## ADDED Requirements

### Requirement: The role catalogue is matched as labels plus aliases

The system SHALL match a typed query against a matchable surface built from BOTH
the role catalogue's display labels (`ROLE_LABELS`) and the role search-alias map
the role facet picker already uses, so a query that names a role by a synonym
(`swe`, `devrel`) resolves the same as one that names it by its label. Matching
SHALL reuse the picker's existing matcher rather than introducing a second,
divergent notion of "this query names this role".

A query SHALL match a role when it matches that role's label or any of its
aliases. When several aliases map a query to the same role slug, that role SHALL
be offered once.

#### Scenario: Query names a role by its label
- **WHEN** the query is `data analyst`
- **THEN** the `data_analytics` role is among the matches

#### Scenario: Query names a role by an alias
- **WHEN** the query is `swe`
- **THEN** the `software_engineer` role is among the matches

#### Scenario: Query is a prefix of a role name
- **WHEN** the query is `data an`
- **THEN** the `data_analytics` role is among the matches

#### Scenario: One role matched through several aliases is offered once
- **WHEN** the query matches two aliases that both resolve to `ai_engineering`
- **THEN** `ai_engineering` appears exactly once in the matches

### Requirement: Suggestions are ranked by match quality before open vacancies

The matcher is typo-tolerant and matches curated aliases, so a query can reach a
role it plainly does not name: `devops` reaches Sales Specialist through the alias
`revops`, and `backend` reaches Marketing Specialist through `growth hacker`.
Ranking such a set by open-vacancy count alone promotes whichever unrelated role
owns the largest bucket — measured against the live catalogue, it put Sales
Specialist (147,223) above DevOps Engineer for `devops`, and Marketing Specialist
(55,768) above Backend Engineer for `backend`.

The system SHALL therefore rank by match quality FIRST, in these tiers:

1. the query prefixes the role's label or one of its aliases AND finishes a word
   there — `data` naming "Data Analyst";
2. the query prefixes one but stops mid-word — `data` against the Software
   Generalist alias "database developer", and every half-typed query on its way to
   tier 1;
3. the query starts a word further in — `design` inside "Product Design Lead".

Tiers 1 and 2 are separate because a plain prefix rule is not enough: "database
developer" carries 75,427 jobs behind it, so under one combined tier a search for
`data` led with Software Generalist rather than Data Analyst.

A role the matcher admitted ONLY through typo tolerance SHALL NOT be offered at
all. Nothing distinguishes two such matches, so ordering them by count hands the
lead to the biggest bucket: `backedn` led with Marketing Specialist (55,768,
reached by edit distance against its `growth hacker` alias) ahead of Backend
Engineer. Offering nothing is the honest answer — the dropdown always keeps its
free-text row, and the search index tolerates typos itself.

Within a tier the system SHALL order by open-vacancy count, most first, and SHALL
break a remaining tie by label so the list does not reshuffle between renders.
The system SHALL offer at most five roles.

#### Scenario: A mid-word prefix loses to a role the query names outright
- **WHEN** the query is `data` and the Software Generalist alias "database developer" has more open vacancies than Data Analyst
- **THEN** Data Analyst is offered first

#### Scenario: A typo-only match is not offered
- **WHEN** the query is `backedn`, which reaches roles only through typo tolerance
- **THEN** no role is offered

### Requirement: One row per role, never one per grade

The role catalogue carries every seniority grade as its own slug
(`senior_data_analytics`, `intern_qa`), and graded slugs outnumber ungraded ones
in the live distribution roughly six to one. Offering them individually spends the
whole five-row budget on one role: `data analyst` measured as Data Analyst, Senior
Data Analyst, Lead Data Analyst, Junior Data Analyst, Intern Data Analyst, with
Data Engineer and Data Scientist pushed out entirely.

The system SHALL offer at most one row per BASE role — the slug with any seniority
grade stripped — keeping whichever graded or ungraded variant ranks highest under
the rule above. Grade remains reachable through the seniority facet, and a query
that names a grade still reaches it: naming it makes that variant the better match
under tier 1, so it is the variant kept.

#### Scenario: Grades of one role do not crowd out other roles
- **WHEN** the query is `data analyst` and the distribution carries Data Analyst and its senior, lead, junior and intern grades alongside Data Engineer
- **THEN** Data Analyst is offered once and Data Engineer is still offered

#### Scenario: Naming a grade keeps that grade
- **WHEN** the query is `senior data analyst`
- **THEN** the row offered for that role is Senior Data Analyst, not Data Analyst

#### Scenario: An unrelated role reached only by typo tolerance never leads
- **WHEN** the query is `devops` and Sales Specialist (reached through its `revops` alias) has far more open vacancies than DevOps Engineer
- **THEN** DevOps Engineer is offered first

#### Scenario: A large fuzzy match does not outrank the named role
- **WHEN** the query is `backend` and Marketing Specialist (reached through its `growth hacker` alias) has more open vacancies than Backend Engineer
- **THEN** Backend Engineer is offered first

When the facet distribution has not loaded yet, the system SHALL still offer the
matched roles — ranked by match quality alone, since no count exists to order
within a tier — and SHALL omit the count rather than render a zero or hide the
suggestion, because an absent measurement must not read as "no vacancies". Match
quality is what makes this state usable: ordering the unmeasured case by label
alone fills every row with `C-Level …` and `Intern …` and can omit the named role
entirely, which is what a jobs page shows on every cold load.

When the distribution HAS loaded, a matched role absent from it SHALL NOT be
offered: the catalogue carries more role slugs than the open catalogue currently
has jobs for, and a suggestion that leads to an empty result page is worse than no
suggestion. This is distinct from the case above — an absent distribution means
"not measured yet", an absent entry within a present distribution means "measured,
and it is zero".

A role the user has already selected in the `role` facet SHALL NOT be offered.

#### Scenario: More vacancies rank higher
- **WHEN** `data_analytics` has more open vacancies than `data_engineering` and both match
- **THEN** `data_analytics` is offered above `data_engineering`

#### Scenario: At most five roles are offered
- **WHEN** a query matches nine roles
- **THEN** exactly five are offered

#### Scenario: Counts have not loaded
- **WHEN** the role facet distribution is unavailable and the query is `swe`
- **THEN** Software Engineer is the first role offered, and no row carries a count

#### Scenario: A role with no open vacancies is not offered
- **WHEN** the distribution is loaded and a matched role has no entry in it
- **THEN** that role is not among the offered roles

#### Scenario: An already-selected role is not re-offered
- **WHEN** `role=data_analytics` is active and the query matches `data_analytics`
- **THEN** `data_analytics` is not among the offered roles

### Requirement: The header search offers role suggestions on jobs-backed lists only

The header list-search input SHALL render a role suggestion dropdown when, and
only when, the registered list-search target publishes a `roleSuggest` capability
and the typed query is at least two characters long and matches at least one role.

The jobs views SHALL publish `roleSuggest`; the companies list SHALL NOT. The
header SHALL decide purely from the published capability and SHALL NOT branch on
the page path — the same opt-in bridge pattern `filterScope` and `openFilters`
already use.

#### Scenario: Jobs feed offers suggestions
- **WHEN** a user types `product man` into the header search on the jobs feed
- **THEN** a dropdown offers `Product Manager` with its open-vacancy count

#### Scenario: Companies list offers none
- **WHEN** a user types `product man` into the header search on `/companies`
- **THEN** no role dropdown appears

#### Scenario: One character offers nothing
- **WHEN** the query is a single character
- **THEN** no role dropdown appears

### Requirement: Choosing a suggestion replaces the text query with the role facet

Choosing a role suggestion SHALL apply that role slug to the `role` facet AND
clear the text query in the same interaction, so the resulting search is an exact
tag filter rather than a tag filter narrowed by a redundant full-text match. The
change SHALL flow through the list's existing filter store, so the URL, the filter
chips, the result list, and the facet counts update the way any other facet
selection does.

#### Scenario: Selecting a role applies the facet and empties the box
- **WHEN** the user chooses `Data Analyst` from the dropdown
- **THEN** `role=data_analytics` is applied, the search input is empty, and the URL carries the role and no `q`

#### Scenario: The role reads as a normal filter afterwards
- **WHEN** a role has been applied from a suggestion
- **THEN** it appears as an active filter chip and can be removed like any other facet

### Requirement: Free-text search is never taken away

The dropdown SHALL always offer, as its last row, running the typed text as a
free-text search. Pressing Enter while no suggestion is highlighted SHALL run the
free-text search exactly as it does today — the dropdown SHALL NOT capture Enter
by default.

#### Scenario: Enter without a highlighted suggestion searches text
- **WHEN** the user types `revolut` and presses Enter without arrowing into the list
- **THEN** the free-text search for `revolut` runs and the role facet is untouched

#### Scenario: The text row is always present
- **WHEN** the dropdown is open with role matches
- **THEN** a final row offers searching the typed text as text

### Requirement: The dropdown is keyboard and dismissal complete

The dropdown SHALL be operable without a mouse: Down and Up SHALL move the
highlight through the offered rows, Enter SHALL activate the highlighted row, and
Escape SHALL close the dropdown while leaving the typed text intact. Clicking
outside the dropdown SHALL close it. The dropdown SHALL expose its state to
assistive technology using the combobox/listbox roles and `aria-activedescendant`.

#### Scenario: Arrow keys and Enter select a role
- **WHEN** the user presses Down until `Data Analyst` is highlighted and presses Enter
- **THEN** the `data_analytics` role facet is applied and the input is cleared

#### Scenario: Escape closes without losing the query
- **WHEN** the dropdown is open and the user presses Escape
- **THEN** the dropdown closes and the typed text remains in the input

#### Scenario: Clicking away closes the dropdown
- **WHEN** the dropdown is open and the user clicks elsewhere on the page
- **THEN** the dropdown closes
