## ADDED Requirements

### Requirement: A colleague joins a company by an emailed invitation
Anyone acting for an active company account SHALL be able to invite a work email address to
the company's team. The address SHALL receive a single-use link valid for seven days.
Following it while signed in SHALL make the user a member acting under the invited address.
Inviting an address again SHALL replace its link. A company SHALL hold at most twenty open
invitations.

#### Scenario: Inviting and joining
- **WHEN** a member invites `dev@acme.test` and the recipient follows the link while signed in
- **THEN** they become a member of the company's team acting under `dev@acme.test`

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

### Requirement: Every member acts as the claimant does
A member SHALL see and edit every vacancy the company's team published, see every
application to them and move it through the pipeline, edit the company profile, and manage
the team. A member SHALL NOT apply to a vacancy of their own company.

#### Scenario: A member sees a colleague's vacancy and its applications
- **WHEN** a member opens the vacancies and the applicant inbox
- **THEN** they see the vacancies the claimant published and the applications to them

#### Scenario: A member applies to their own company
- **WHEN** a member applies to a vacancy their company's team published
- **THEN** the application is refused as not found

### Requirement: Members can be removed; the claimant cannot
Any member SHALL be able to remove another member or leave. The claimant SHALL NOT be
removable. A removed member's vacancies SHALL stay with the team, handed to the claimant, and
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
- **THEN** the request is refused as invalid

### Requirement: A revoked account refuses its members
When a company account is revoked, every member SHALL be refused like its claimant, and no
invitation to it SHALL be accepted.

#### Scenario: Joining a revoked account
- **WHEN** someone follows an invitation to a revoked account
- **THEN** it is refused
