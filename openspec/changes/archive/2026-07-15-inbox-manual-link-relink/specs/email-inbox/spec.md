## ADDED Requirements

### Requirement: Manual application linking in the inbox

The inbox SHALL let the caller manually link an email that has no application
link and no active suggestion to one of their tracked applications. Choosing an
application SHALL link the email to it and mark the link as manual.

The picker SHALL list the caller's tracked applications and SHALL let the caller
filter them by text. When the caller tracks no applications, the picker SHALL say
so rather than present an empty menu.

#### Scenario: Linking an unlinked email

- **WHEN** the caller opens the "Link to application" picker on an unlinked email
  and selects one of their tracked applications
- **THEN** the email becomes linked to that application
- **AND** the row shows "Linked to <company>" with an Unlink control

#### Scenario: No tracked applications

- **WHEN** the caller opens the picker but tracks no applications
- **THEN** the picker states that there is nothing to link to instead of showing
  an empty list

### Requirement: Undo an unlink

Immediately after the caller unlinks an email, the inbox SHALL offer an Undo that
re-links the email to the application it was just unlinked from. The Undo SHALL
apply only to the email that was just unlinked and SHALL be discarded when the
caller acts on a different email or establishes a new link.

#### Scenario: Undo restores the previous link

- **WHEN** the caller unlinks an email and then chooses Undo
- **THEN** the email is re-linked to the same application it was unlinked from

#### Scenario: Undo does not leak across emails

- **WHEN** the caller unlinks one email and then selects a different email
- **THEN** the different email does not offer to undo the first email's unlink
