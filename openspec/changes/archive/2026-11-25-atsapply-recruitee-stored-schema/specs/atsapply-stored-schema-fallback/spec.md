## Purpose

Lets an auto-apply attempt reach field resolution for an ATS provider whose application form was captured during ingest rather than fetched live, instead of parking before resolution ever runs.

## ADDED Requirements

### Requirement: A stored form is read when no live schema fetcher exists

When submitting an auto-apply attempt for a posting whose ATS provider has no registered
live schema fetcher, the system SHALL attempt to read that job's previously captured
application form from storage before parking the attempt. When a stored form exists, it
SHALL be used as the schema for field resolution, exactly as a live-fetched schema would be.

#### Scenario: A provider with no live fetcher has a stored form

- **WHEN** an auto-apply attempt is submitted for a job whose provider has no live schema fetcher, and that job's application form was already captured into storage during ingest
- **THEN** the stored form is used as the schema, and field resolution (including any LLM-drafted answers) runs against it exactly as it would for a live-fetched schema

### Requirement: Absence of both a live fetcher and a stored form still parks honestly

When no live schema fetcher is registered for the provider and no form has been captured
into storage for the job either, the attempt SHALL park with the same reason it does today
— this requirement changes what is tried, not what happens when nothing is found.

#### Scenario: Neither a live fetcher nor a stored form is available

- **WHEN** an auto-apply attempt is submitted for a job whose provider has no live schema fetcher, and no form has been captured into storage for that job
- **THEN** the attempt parks with the same reason it reports today, and no field resolution runs

### Requirement: A resolved schema for a provider with no submit path still parks, but with precise diagnostics

Reaching field resolution for a provider that can neither be filled via the browser DOM
path nor submitted via the cloud fallback SHALL NOT cause a submission to be attempted.
When such an attempt's resolved plan is incomplete, the system SHALL report exactly which
fields could not be resolved, the same way it already does for any other provider with an
incomplete plan. When such an attempt's resolved plan is complete, the system SHALL park it
with the same "submission not yet implemented for this provider" reason used today.

#### Scenario: A resolvable-but-unsubmittable provider's incomplete plan reports its gaps

- **WHEN** field resolution runs, using a stored form, for a provider with no fill or submit path, and some required fields cannot be resolved
- **THEN** the attempt parks and names the specific unresolved fields, rather than reporting an undifferentiated "not implemented" reason

#### Scenario: A resolvable-but-unsubmittable provider's complete plan still parks

- **WHEN** field resolution runs, using a stored form, for a provider with no fill or submit path, and every required field resolves
- **THEN** the attempt still parks, with the same "submission not yet implemented for this provider" reason shown for this provider today
