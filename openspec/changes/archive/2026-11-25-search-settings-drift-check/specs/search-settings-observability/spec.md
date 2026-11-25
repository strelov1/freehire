## Purpose

Reports the gap between what a binary's Meilisearch client code expects the live
jobs/companies index settings to declare and what the live index actually declares, on
a schedule, so a deploy-ordering mistake is visible before a caller hits it.

## ADDED Requirements

### Requirement: Live index settings are compared against what the binary expects

The system SHALL compare the live jobs index's and the live companies index's declared
sortable attributes, filterable attributes, and embedders against the set this binary's
own index-settings code expects, and SHALL report one entry per gap: an attribute or
embedder the binary may request that the live index has not declared. The comparison
SHALL name the index and the specific attribute or embedder for each gap.

#### Scenario: Live settings match what the binary expects

- **WHEN** the live jobs and companies indexes declare every sortable attribute, filterable attribute, and embedder this binary's settings code expects
- **THEN** the comparison reports no gaps

#### Scenario: A sortable attribute is missing from the live index

- **WHEN** the binary's expected settings include a sortable attribute the live jobs index does not declare
- **THEN** the comparison reports a gap naming the jobs index and that attribute

#### Scenario: A filterable attribute is missing from the live index

- **WHEN** the binary's expected settings include a filterable attribute the live index does not declare
- **THEN** the comparison reports a gap naming the index and that attribute

#### Scenario: An embedder is missing from the live index

- **WHEN** the binary's expected settings include an embedder the live jobs index does not declare
- **THEN** the comparison reports a gap naming the jobs index and that embedder

### Requirement: The comparison is one-directional

The system SHALL NOT report a gap for an attribute or embedder the live index declares
that the binary's expected settings do not include. Only an attribute or embedder the
binary may request that the live index lacks is a gap — a live index still declaring a
retired attribute is a harmless, distinct state that never breaks a request.

#### Scenario: The live index declares a retired attribute

- **WHEN** the live index declares a sortable attribute the binary's expected settings no longer include
- **THEN** the comparison reports no gap for that attribute

### Requirement: The check runs on a schedule and publishes what it finds

The system SHALL run this comparison on a recurring schedule, independent of any
request path, and SHALL publish the number of gaps found as a metric a monitoring
system can alert on. A run that finds gaps SHALL NOT be treated as the check's own
failure — the check succeeded at observing a state it does not control.

#### Scenario: A scheduled run finds no gaps

- **WHEN** a scheduled run's comparison reports no gaps
- **THEN** the published metric reports zero, and the run reports success

#### Scenario: A scheduled run finds gaps

- **WHEN** a scheduled run's comparison reports one or more gaps
- **THEN** the published metric reports the gap count, and the run still reports success — the gap is the observed condition, not a failure of the observer

### Requirement: The check is inert without its required configuration

The system SHALL perform no comparison and open no connection to Meilisearch when it
has nowhere to publish its result or no way to authenticate to Meilisearch.

#### Scenario: No metrics destination configured

- **WHEN** the check runs with no configured destination for its published metric
- **THEN** it performs no comparison and exits successfully

#### Scenario: No Meilisearch credential configured

- **WHEN** the check runs with a metrics destination configured but no Meilisearch credential
- **THEN** it performs no comparison and exits successfully
