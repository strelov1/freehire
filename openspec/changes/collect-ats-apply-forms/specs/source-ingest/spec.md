## ADDED Requirements

### Requirement: An adapter may yield an application form with a job

An adapter SHALL be able to yield, together with a normalized job, the
application form the platform published for that posting, when the platform's
list endpoint already carries it. The ingest write path SHALL persist such a form
alongside the job.

Yielding a form SHALL remain optional: an adapter that yields none SHALL keep
working unchanged, and the absence of a form SHALL NOT fail the posting, the
board, or the run. An adapter SHALL NOT issue an extra request in order to yield
a form — a form that costs a request is captured after ingest, not during it.

#### Scenario: A yielded form is persisted with the job

- **WHEN** an adapter yields a job carrying an application form
- **THEN** ingest writes the job and stores the form against it

#### Scenario: An adapter that yields no form is unaffected

- **WHEN** an adapter yields a job carrying no application form
- **THEN** ingest writes the job exactly as it does today and stores no form
