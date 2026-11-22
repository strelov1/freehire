## ADDED Requirements

### Requirement: A colleague joins a company by an emailed invitation
The owner and managers of an active company account SHALL be able to invite a work email
address to the company's team. The invitation SHALL grant recruiter access through a
single-use link valid for seven days. The signed-in account SHALL have a confirmed,
case-insensitively matching email to accept. Existing links retain their original access
and acceptance conditions until expiration, replacement or consumption.
Inviting an address again SHALL replace its link. A company SHALL hold at most twenty open
invitations.

#### Scenario: Inviting and joining
- **WHEN** a manager invites `dev@acme.test` and the recipient follows the link while signed in with that confirmed email
- **THEN** they become a recruiter for the company acting under `dev@acme.test`

#### Scenario: A different or unverified email tries to join
- **WHEN** the signed-in email differs from the recruiter invite or has not been confirmed
- **THEN** the request is refused and the invitation remains available to its recipient

#### Scenario: A link works once
- **WHEN** an invitation link that was already accepted is followed again
- **THEN** it is refused as invalid

#### Scenario: A resend replaces the link
- **WHEN** an address is invited twice
- **THEN** only the second link works

#### Scenario: An expired link
- **WHEN** an invitation link is followed after seven days
- **THEN** it is refused as invalid

#### Scenario: A public webmail address
- **WHEN** a member invites an address at a public email provider
- **THEN** the request is refused

### Requirement: A user acts for at most one company
A user who claimed a company SHALL NOT join another's team, and a member SHALL NOT claim a
company.

#### Scenario: A claimant follows an invitation
- **WHEN** the claimant of one company follows another company's invitation
- **THEN** the request is refused with a conflict

#### Scenario: A member claims a company
- **WHEN** a member of one company's team claims another company
- **THEN** the claim is refused

### Requirement: Every member has access to the company inbox
A member SHALL see the company's vacancies and manage their delivered applications.
The owner and managers SHALL also be able to publish and manage vacancies, edit the
company profile, and manage the team. Existing members SHALL retain those permissions
as managers. A member SHALL NOT apply to a vacancy of their own company.

#### Scenario: A recruiter opens the company workspace
- **WHEN** a recruiter opens the workspace
- **THEN** they can review and download company applications, record notes and move stages
- **AND** requests to manage the team, profile or vacancy publication are refused

#### Scenario: A member sees a colleague's vacancy and its applications
- **WHEN** a member opens the vacancies and the applicant inbox
- **THEN** they see the vacancies the claimant published and the applications to them

#### Scenario: A member applies to their own company
- **WHEN** a member applies to a vacancy their company's team published
- **THEN** the application is refused as not found

### Requirement: Members can be removed; the claimant cannot
The owner and managers SHALL be able to remove another member. Any member SHALL be able
to leave. The claimant SHALL NOT be removable. A removed member's vacancies SHALL stay with the team, handed to the claimant, and
the removed member SHALL lose access at once.

#### Scenario: Removing a member
- **WHEN** a member is removed
- **THEN** they can no longer reach the company, and the vacancies they published now belong to the claimant

#### Scenario: A removed member's invitations stop working
- **WHEN** a member who sent an invitation is removed
- **THEN** that invitation's link no longer works

#### Scenario: A member deletes their freehire account
- **WHEN** a member deletes their freehire account
- **THEN** the vacancies they published now belong to the claimant and keep taking applications

#### Scenario: Removing the claimant
- **WHEN** a member tries to remove the claimant
- **THEN** the request is refused

### Requirement: A revoked account refuses its members
When a company account is revoked, every member SHALL be refused like its claimant, and no
invitation to it SHALL be accepted.

#### Scenario: Joining a revoked account
- **WHEN** someone follows an invitation to a revoked account
- **THEN** it is refused


### Requirement: Authorized teammates can read company activity
The company SHALL record stage and private note changes, invitation and withdrawal
events, joins and membership removals. The owner and managers SHALL see company activity.
Recruiters SHALL see application activity. Entries SHALL omit the contents of notes,
candidate messages and CVs. Events for a deleted application SHALL be deleted with it.

#### Scenario: An employer saves private notes
- **WHEN** a member changes an application's private notes
- **THEN** the company records an event for the application without the note text
