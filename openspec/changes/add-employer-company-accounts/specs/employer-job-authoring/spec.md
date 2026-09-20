## Purpose

Lets a verified employer publish, edit, and close vacancies for their own company directly,
attributed to that company alone, with no moderator step in the ordinary path.

## ADDED Requirements

### Requirement: A verified employer creates a vacancy for their own company

The system SHALL allow a user with an active employer account to create a vacancy through a
dedicated employer endpoint. The created job MUST be stored with `source = 'employer'`,
`external_id` equal to the supplied apply URL, and `created_by` set to the acting user. The
system SHALL derive geography, skill tags, the public slug, and the company slug from the
supplied fields using the same deterministic dictionaries every other manual write path
uses. `url` and `title` are required; the company identity is never taken from the request
(see the following requirement).

#### Scenario: An active employer creates a vacancy

- **WHEN** a user with an active employer account submits `{ "url": "...", "title": "..." }`
- **THEN** the system stores a job with `source='employer'`, `external_id` equal to the URL,
  `created_by` set to that user, and responds with the created vacancy

#### Scenario: A non-employer or unverified account cannot use this endpoint

- **WHEN** a user with no employer account, or a pending/revoked one, calls the employer
  create endpoint
- **THEN** the request is refused and no job is created

### Requirement: A vacancy's company identity is fixed to the employer's claimed company

The system SHALL attribute every vacancy an employer creates to the exact company name and
slug recorded on their employer account at claim time, never to company text supplied in
the create/edit request. This SHALL hold for every vacancy the employer creates or edits, so
an employer account's postings always resolve to one stable `company_slug`.

#### Scenario: The stored company matches the claimed company regardless of request content

- **WHEN** an active employer creates a vacancy
- **THEN** the stored job's company name and `company_slug` equal the company recorded on
  that employer's account, not any company value the request might otherwise imply

### Requirement: Re-creating under the same URL by its own owner reopens the vacancy

When an employer submits a create request whose URL matches a vacancy already owned by that
same employer account, the system SHALL treat it as an idempotent update of that vacancy
rather than creating a duplicate, and SHALL clear `closed_at` if the vacancy was closed.

#### Scenario: Re-submitting the same URL updates the existing vacancy

- **WHEN** an employer submits a create request whose URL matches a vacancy they already own
- **THEN** the existing vacancy's content is updated rather than a new one being created

#### Scenario: Re-submitting the same URL reopens a closed vacancy

- **WHEN** an employer submits a create request whose URL matches a vacancy they own that is
  currently closed
- **THEN** the vacancy's `closed_at` is cleared

### Requirement: A colliding URL from a different employer is refused, not taken over

When a create request's URL already identifies a vacancy owned by a *different* employer
account, the system SHALL refuse the request with a conflict response and MUST NOT alter
the existing vacancy's content or ownership.

#### Scenario: A different employer cannot take over a vacancy via a matching URL

- **WHEN** employer account B submits a create request whose URL already identifies a
  vacancy created by employer account A
- **THEN** the request is refused, and vacancy A's content and `created_by` are unchanged

### Requirement: A verified employer edits their own vacancy

The system SHALL allow a user with an active employer account to edit the title, location,
remote flag, description, and posted-at date of a vacancy they own, addressed by its public
slug. The system SHALL re-derive geography, skill tags, and other dictionary-driven facets
from the edited content. The vacancy's URL/external identity and its company attribution
SHALL NOT be editable through this action.

#### Scenario: An employer edits their own vacancy's content

- **WHEN** an active employer submits an edit to a vacancy they own
- **THEN** the specified fields are updated and dictionary-derived facets are recomputed
  from the new content

#### Scenario: The URL and company are not affected by an edit

- **WHEN** an active employer edits their own vacancy
- **THEN** the vacancy's URL/external identity and its company/`company_slug` are unchanged

### Requirement: A verified employer cannot edit or close another account's vacancy

The system SHALL scope every employer edit and close action to vacancies whose
`created_by` is the acting employer's own user id and whose `source` is `'employer'`. An
attempt to edit or close a vacancy created by a different employer account, by a moderator,
or by an automated source SHALL be refused as not found.

#### Scenario: An employer cannot edit a moderator-authored vacancy

- **WHEN** an active employer attempts to edit a vacancy created by a moderator
- **THEN** the request is refused as not found, and the vacancy is unchanged

#### Scenario: An employer cannot edit another employer's vacancy

- **WHEN** employer account B attempts to edit or close a vacancy created by employer
  account A
- **THEN** the request is refused as not found, and vacancy A is unchanged

### Requirement: A verified employer closes their own vacancy

The system SHALL allow a user with an active employer account to close a vacancy they own.
Closing SHALL be soft: it MUST set `closed_at` and record `closed_reason = 'employer_closed'`
without deleting the row, consistent with every other closing mechanism in the catalogue.

#### Scenario: An employer closes their own vacancy

- **WHEN** an active employer closes a vacancy they own
- **THEN** the vacancy's `closed_at` is set, `closed_reason` is `'employer_closed'`, and the
  row still exists and remains reachable at its public slug

### Requirement: A verified employer lists their own vacancies

The system SHALL let a user with an active employer account list every vacancy they have
published through this capability, newest first, including vacancies they have closed. The
list SHALL never include a vacancy created by another employer account, a moderator, or an
automated source.

#### Scenario: An employer's list contains only their own vacancies

- **WHEN** an active employer lists their vacancies
- **THEN** the response contains every vacancy that employer created through this
  capability, and none created by any other account or source

#### Scenario: A closed vacancy still appears in the list

- **WHEN** an active employer has closed one of their own vacancies
- **THEN** it still appears in their vacancy list
