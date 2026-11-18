## ADDED Requirements

### Requirement: Headless fault collection
When the applicable OTLP endpoint is configured, the system SHALL store unexpected
application failures as OpenTelemetry exception logs that can be queried without
opening a web interface. Each scenario below assumes that configuration; without
an endpoint, the optional fallback and local logging remain available as configured.

#### Scenario: Unexpected HTTP error
- **WHEN** an API handler returns an unexpected failure
- **THEN** a queryable exception includes service identity, severity and its error
- **AND** expected client errors and disconnects do not become application faults

#### Scenario: Worker terminates unsuccessfully
- **WHEN** a worker exits unsuccessfully or panics
- **THEN** the final failure is emitted before bounded exporter shutdown
- **AND** the worker retains its original nonzero exit or panic behavior

#### Scenario: Client application failure
- **WHEN** the web application or extension encounters its own unexpected error
- **THEN** configured collection emits a bounded exception event
- **AND** it excludes unrelated page errors and request credentials

### Requirement: Reuse operations infrastructure
The system SHALL reuse Vector and VictoriaLogs and provide terminal access and
configurable notifications using existing open-source tools.

#### Scenario: Operator investigates
- **WHEN** an operator connects using the documented SSH tunnel and CLI or HTTP API
- **THEN** they can filter and inspect stored errors and follow new events
- **AND** the storage query endpoint remains private

#### Scenario: Trigger threshold reached
- **WHEN** stored exception events exceed a configured rule threshold
- **THEN** the existing notification channel receives an alert
- **AND** a broken query or collector is not silently treated as no errors
