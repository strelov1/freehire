## MODIFIED Requirements

### Requirement: The frozen fit analysis is labelled as a snapshot of the base profile

The Job Match tab SHALL continue to present the cached LLM fit analysis beneath the live score,
and SHALL label it as kept current by the latest autopilot run rather than as a frozen snapshot,
alongside its existing recompute control.

A candidate reading two numbers beside each other needs to know what each one measures. Now that
an autopilot run refreshes the cached analysis itself, labelling it as still, silently frozen would
be actively wrong rather than merely uninformative.

#### Scenario: The fit analysis is labelled as current, not frozen

- **WHEN** the Job Match tab renders the cached fit analysis
- **THEN** it is labelled as kept current by the latest autopilot run, not as a snapshot of the base profile

#### Scenario: The two scores are visually distinct

- **WHEN** both the live job-match score and the cached fit score are shown
- **THEN** they are presented as separate blocks with their own headings, not as two rows of one table
