# test-coverage-reporting Specification

## Purpose
TBD - created by archiving change pr-tests-and-coverage. Update Purpose after archive.
## Requirements
### Requirement: Go coverage reports for unit and integration suites

The project SHALL provide a documented way to run the Go unit suite and the `integration`-tagged suite with coverage profiles (package-level HTML or coverprofile artifacts). CI SHALL generate those profiles on pull requests and retain them as build artifacts. The build MUST NOT fail solely because overall coverage is below a numeric threshold in this change.

#### Scenario: Local unit coverage

- **WHEN** a developer runs the documented Go unit-coverage command
- **THEN** a coverage profile is written that includes packages exercised by `go test ./...`

#### Scenario: CI uploads Go coverage

- **WHEN** CI runs the backend job on a pull request
- **THEN** unit (and, when run, integration) coverage artifacts are available on the workflow run

### Requirement: Web Vitest coverage report

The web package SHALL provide a documented Vitest coverage run that reports line coverage for exercised modules. CI SHALL run or attach that report on pull requests as an artifact. The build MUST NOT fail solely because coverage is below a numeric threshold in this change.

#### Scenario: Local web coverage

- **WHEN** a developer runs the documented web coverage command
- **THEN** a coverage report is produced for the Vitest suite

#### Scenario: CI uploads web coverage

- **WHEN** CI runs the web job on a pull request
- **THEN** a Vitest coverage artifact is available on the workflow run

