## ADDED Requirements

### Requirement: A hydrating crawl re-reads a posting whose stored body has gone stale

The seen-set a hydrating adapter consults SHALL withhold a posting whose stored body was
last fetched longer ago than the configured body-refresh window, so the adapter fetches its
detail again and the ordinary write path re-derives every facet from the fresh text.

A posting that has never recorded a body fetch SHALL count as stale.

#### Scenario: An edited posting is read again

- **WHEN** a hydrating provider is crawled with a body-refresh window of 45 days
- **AND** a stored posting of that provider last recorded a body fetch 90 days ago
- **THEN** that posting is offered to the adapter for a detail fetch as though it were new

#### Scenario: A freshly read posting is not re-read

- **WHEN** the same crawl reaches a posting whose body was fetched within the window
- **THEN** that posting is treated as seen and costs no detail request

### Requirement: The re-reads are bounded by a slice

One crawl SHALL re-read at most its share of the stale postings, where the share is decided
by a deterministic function of the posting's own identity and the number of configured
slices. A posting SHALL belong to the same slice for its whole life, so successive runs
sweep the catalogue rather than repeating one part of it.

#### Scenario: Every stale posting is re-read by exactly one slice

- **WHEN** a provider's stale postings are queried once per slice, for every slice
- **THEN** each posting is withheld by exactly one of those queries

### Requirement: Body refresh is off unless configured

The body-refresh window SHALL be unset by default, and while unset no posting SHALL be
withheld for staleness — crawl behaviour is exactly what it was before. A configuration
value that cannot be read as a positive number of days SHALL fail the run naming the value,
and a slice setting supplied without a window SHALL fail the run rather than having no
effect.

#### Scenario: An unconfigured deployment crawls as before

- **WHEN** no body-refresh window is configured
- **THEN** no posting is withheld from the seen-set on account of its body's age

#### Scenario: A slice alone is refused

- **WHEN** a slice count is configured and no window is
- **THEN** the run fails rather than silently refreshing nothing

### Requirement: A body fetch is recorded on the row

Every write that carried a freshly fetched body SHALL record the time of that write on the
posting, including a write whose incoming body proved identical to the stored one. A
liveness-only refresh, which fetches nothing, SHALL NOT record one.

#### Scenario: An unchanged body still counts as read

- **WHEN** a crawl re-fetches a posting and its body is byte-identical to the stored one
- **THEN** the posting records the fetch and is not stale on the next crawl
