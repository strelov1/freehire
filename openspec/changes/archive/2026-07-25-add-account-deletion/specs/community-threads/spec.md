## ADDED Requirements

### Requirement: Authored content survives its author's deletion

A thread or reply SHALL remain readable after its author's account is deleted.
Deleting an account SHALL NOT remove discussion other members contributed to.

- A thread whose author is gone SHALL keep its replies, including replies by
  members who still have accounts.
- A de-authored thread or reply SHALL still appear in listings and thread reads;
  the missing author SHALL NOT cause it to be filtered out.
- The author identity of de-authored content SHALL be rendered as an explicit
  deleted-member marker, distinct from both a live persona handle and the AI
  persona used for authorless system replies.

#### Scenario: Thread outlives its author

- **WHEN** a member who opened a thread with replies from others deletes their account
- **THEN** the thread and all its replies remain readable, and the thread still appears in its subject's listing

#### Scenario: De-authored content is not mistaken for the AI persona

- **WHEN** a client reads a thread or reply whose author's account was deleted
- **THEN** the author is presented as a deleted member, distinguishable from the AI persona

#### Scenario: The departed member's handle is gone

- **WHEN** a member's account is deleted
- **THEN** their persona handle no longer appears on any of their surviving threads or replies, and the handle carries no link back to them
