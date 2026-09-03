## MODIFIED Requirements

### Requirement: Choosing a suggestion applies its facets and keeps the typed text

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
