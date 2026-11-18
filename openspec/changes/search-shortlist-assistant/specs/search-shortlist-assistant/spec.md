## ADDED Requirements

### Requirement: Shortlist the current search
The system SHALL let a signed-in candidate open the assistant from the standalone job search, preserving all supported effective constraints and caller-specific filters. It SHALL return at most five grounded recommendations and SHALL NOT silently widen unsupported or degraded constraints.

#### Scenario: Existing filtered search
- **WHEN** the candidate starts shortlisting from a filtered jobs list
- **THEN** the assistant searches within the captured constraints and explains concrete reasons and concerns for each selected vacancy

#### Scenario: Unsupported constraint
- **WHEN** the search cannot enforce a captured constraint
- **THEN** the assistant reports the condition and does not recommend a widened result as compliant

### Requirement: Durable conversation context
The system SHALL persist the initial search and session-local feedback independently of bounded model history, scoped to the owner. Later page filter edits SHALL NOT change that search automatically.

#### Scenario: Reopen after feedback
- **WHEN** the owner reopens or reloads a conversation after correcting its selection
- **THEN** the assistant retains the original search and the saved selection preferences

#### Scenario: Another account requests context
- **WHEN** a different account requests or updates that conversation
- **THEN** the system returns the same missing response as for a nonexistent session

### Requirement: Existing assistant lifecycle
The entry SHALL reuse current authentication, metering, cancellation, and retry. It SHALL NOT require a saved profile or uploaded CV to provide an initial shortlist.

#### Scenario: No candidate profile
- **WHEN** a signed-in candidate without a saved profile asks for a shortlist
- **THEN** the assistant uses the search and stated preferences and asks at most one consequential clarification when necessary

#### Scenario: Close and reopen
- **WHEN** the candidate closes and reopens the panel
- **THEN** the same conversation resumes without an implicit cancellation or duplicate opening
