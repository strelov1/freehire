## ADDED Requirements

### Requirement: Incomplete-application status

The classifier SHALL recognise an `incomplete_application` status for emails
telling the caller their application was started but not finished (e.g. "your
application is incomplete", "action required to complete your application"), and
SHALL NOT auto-advance the application stage on this signal (an unfinished
application has not progressed).

#### Scenario: Incomplete application is its own status

- **WHEN** an email says the caller's application is incomplete and must be
  completed
- **THEN** it is classified as `incomplete_application`

#### Scenario: Incomplete application does not advance the stage

- **WHEN** an `incomplete_application` email is linked to an application
- **THEN** the application's stage is not automatically advanced
