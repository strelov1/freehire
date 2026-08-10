## ADDED Requirements

### Requirement: A page read names the page it read

When the agent reads the current page, the panel SHALL show which page that was —
its origin and path — rather than only that a read happened. A read the user cannot
see is a read they cannot object to, and this is the surface where they would.

The display SHALL omit the query string and the fragment. Both routinely carry
one-time tokens and session identifiers, and showing them would leak into the
transcript exactly what this is meant to guard.

A result the panel cannot read a url out of SHALL fall back to the plain label
rather than showing nothing or an error: the read still happened, and saying so
imprecisely beats saying nothing.

#### Scenario: The read is attributed

- **WHEN** the agent has read `https://boards.greenhouse.io/acme/jobs/12` during a turn
- **THEN** the panel's line for that call shows `boards.greenhouse.io/acme/jobs/12`

#### Scenario: Tokens in the url are not shown

- **WHEN** the page read carried a query string or fragment
- **THEN** neither appears in what the panel shows

#### Scenario: An unreadable result still reads sensibly

- **WHEN** the call's result is missing, malformed, or carries no url
- **THEN** the line shows the plain "reading the page" label with nothing appended
