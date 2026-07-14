## ADDED Requirements

### Requirement: Application detail endpoint with linked emails

The system SHALL expose `GET /api/v1/me/tracking/:slug` returning the caller's application for that job slug together with its linked emails (each with `status_signal`, sender, subject, and received time), gated the same way as the rest of the inbox surface.

#### Scenario: Detail returns the application and its emails

- **WHEN** the caller requests the detail endpoint for a job slug they track
- **THEN** the response contains the application interaction and the list of emails linked to it, most recent first

#### Scenario: Application with no emails returns an empty list

- **WHEN** the caller requests an application that has no linked emails
- **THEN** the response returns the application with an empty emails list, not an error

#### Scenario: Untracked slug is not found

- **WHEN** the caller requests a slug they do not track
- **THEN** the endpoint responds 404

### Requirement: Application detail page lists linked emails

The SPA SHALL provide a per-application detail page at `/my/tracking/[slug]` that shows the job, the caller's interaction, and the application's linked emails with per-email status badges.

#### Scenario: Detail page renders linked emails with status badges

- **WHEN** the caller opens `/my/tracking/[slug]` for an application with linked emails
- **THEN** the page lists those emails each showing its status signal as a badge

### Requirement: Classified email may advance the application stage

A high-confidence classified email SHALL be allowed to advance its linked application's `stage` forward only, consistent with the monotonic-forward rule; a lower-confidence or backward signal SHALL be surfaced as a suggestion instead of mutating the stage.

#### Scenario: Forward advancement is reflected in tracking

- **WHEN** a high-confidence email advances an application's stage
- **THEN** the tracking surfaces show the application at the new forward stage

#### Scenario: Suggestion is offered rather than applied

- **WHEN** a classified email's stage change is low-confidence or backward
- **THEN** the tracking surface offers the stage change as a suggestion the caller can apply, and the stored stage is unchanged until they do
