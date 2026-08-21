## ADDED Requirements

### Requirement: The trigger says when the scope was inferred

When the active scope came from the visitor's IP country rather than from anything
they did, the header filter trigger SHALL say so, and clearing it SHALL be the same
single action that clears any chosen scope.

A visitor who did not pick "LATAM" has no way to tell a small catalogue from a
filtered one. The chip is the only place the site can answer "why am I seeing so
few jobs" before they conclude there are few jobs.

#### Scenario: The scope was guessed

- **WHEN** the jobs feed is scoped by the IP-derived region and the visitor has not touched the filters
- **THEN** the trigger marks the scope as inferred, and its accessible name says the same in words

#### Scenario: The visitor clears the guessed scope

- **WHEN** the visitor clears the scope from the trigger
- **THEN** the region facet is emptied, the list reloads unscoped, and the trigger returns to its neutral state

#### Scenario: The visitor edits the guessed scope

- **WHEN** the visitor changes the scope to something else — another region, a country, a city
- **THEN** the scope is theirs from that point on and the trigger no longer marks it as inferred

#### Scenario: A chosen scope is never marked as inferred

- **WHEN** the scope came from the URL, from a restored filter set, or from a pill the visitor clicked
- **THEN** the trigger renders exactly as it does today, with no inferred marking
