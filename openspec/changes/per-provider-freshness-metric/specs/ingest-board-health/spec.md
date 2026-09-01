## ADDED Requirements

### Requirement: Ingest freshness is published per provider

The metrics worker SHALL publish, for each ingest provider, the time that provider's most
recent **successful** board crawl completed, labelled by provider — so a provider that has
stopped producing data is visible as itself rather than averaged into the fleet.

The measurement SHALL be taken from the board-health state, not from the catalogue: the
question is whether the crawl succeeded, not whether a posting happened to change, and the
catalogue-side query is a full scan of a table two orders of magnitude larger which the
metrics worker must not run on its once-a-minute schedule.

A provider whose boards have **never** succeeded SHALL publish no sample for that
provider rather than a zero timestamp. An absent series is what a consuming alert rule
reads as no-data; a zero would be read as a provider infinitely overdue, which states far
more than the evidence supports.

The samples SHALL be emitted in a stable order, so a scrape does not differ from the last
one purely by ordering.

This requirement covers publishing the measurement. The alert rule that consumes it lives
outside this repository and is not satisfied by this requirement alone.

#### Scenario: A healthy provider publishes its last success

- **WHEN** a provider has at least one board whose last crawl succeeded
- **THEN** the exposition carries one sample for that provider, labelled with its name,
  holding the most recent success time across all of its boards

#### Scenario: A provider dead for weeks is visible as itself

- **WHEN** every board of one provider has been failing for weeks while the rest of the
  fleet is healthy
- **THEN** that provider's sample carries its weeks-old timestamp, unaffected by the other
  providers' freshness

#### Scenario: A provider that never succeeded publishes nothing

- **WHEN** no board of a provider has ever recorded a successful crawl
- **THEN** the exposition carries no sample for that provider, and no zero timestamp

#### Scenario: The exposition order is stable

- **WHEN** two collection passes measure the same providers
- **THEN** the samples appear in the same order in both expositions
