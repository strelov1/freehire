## ADDED Requirements

### Requirement: Inbox email carries its application link and status

The inbox email read shape SHALL expose the email's resolved application (`job_id`), any pending suggestion (`suggested_job_id`), the classified `status_signal`, and the `link_source`, so the SPA can render linkage and status without a second lookup.

#### Scenario: Linked email exposes its application and status

- **WHEN** the SPA loads an email that has been classified and linked
- **THEN** the read model includes the linked application reference, the status signal, and the link source

#### Scenario: Unclassified email exposes null linkage

- **WHEN** the SPA loads an email that has not yet been classified
- **THEN** the linkage and status fields are null and the email still renders normally

### Requirement: Reading pane renders link confirmation and application link

The inbox reading pane SHALL render an inline link-confirmation chip for a suggested match ("confirm / not this") and a link to the application when the email is linked.

#### Scenario: Suggested match shows a confirmation chip

- **WHEN** the open email has a `suggested_job_id` and no `job_id`
- **THEN** the reading pane shows an inline chip naming the suggested application with confirm and reject actions

#### Scenario: Linked email links to its application

- **WHEN** the open email has a `job_id`
- **THEN** the reading pane shows a link to that application's detail page

#### Scenario: Unlinked email offers manual linking

- **WHEN** the open email has neither a `job_id` nor a `suggested_job_id`
- **THEN** the reading pane offers a manual "link to application" affordance
