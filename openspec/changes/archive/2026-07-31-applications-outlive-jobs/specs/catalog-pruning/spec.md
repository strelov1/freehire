## ADDED Requirements

### Requirement: Removal MUST NOT destroy a user's application

A prune run SHALL leave every tracked application standing when it deletes the posting the
application was made against, clearing the application's link to that posting and nothing else.
The run MAY continue to remove the views, saves, dismissals and votes recorded against the
posting: those are marks on inventory, and losing them costs a bookmark.

The distinction is the point. The worker's existing note — that a user's saved job goes with the
posting and this is an accepted cost of the campaign — weighed a bookmark and, through one shared
cascade, silently applied the same verdict to applications. An application carries a date, a
stage, free-text notes and a mail history; it is a record of something a person did, and no
catalogue-hygiene campaign has the standing to delete it.

#### Scenario: A pruned posting leaves the application intact

- **WHEN** a run deletes a posting that a user had applied to
- **THEN** the user's application survives with its date, stage, notes and follow-up mark
- **AND** the application no longer names a posting

#### Scenario: A pruned posting still takes bookmarks with it

- **WHEN** a run deletes a posting that a user had only viewed, saved, dismissed or voted on
- **THEN** those marks are removed with the posting

#### Scenario: Removal does not change what the aggregates say about the employer

- **WHEN** a run deletes a posting that a user had applied to and the employer had replied to
- **THEN** the company's served response rate and median reply time are the same as before the run

#### Scenario: A cap is not a licence

- **WHEN** a run deletes its full capped batch
- **THEN** no application anywhere is deleted as part of that batch
