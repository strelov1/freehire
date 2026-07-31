## ADDED Requirements

### Requirement: A stored CV changes only through a committed revision

Every write to a stored CV's document, title or template SHALL go through the revision-committing
path, whatever the entry point — the editor's autosave, the template picker, the key-authenticated
patch endpoint, an agent tool, or seeding a tailored copy. No other code path may write the stored
document, and that restriction MUST be held by the package's own visibility rather than by
convention, so a new caller cannot reach the table without going through the commit.

Sanitization is unchanged and still runs before persistence: a revision records what was asked
for, and the sanitizer decides what is stored.

#### Scenario: Whole-document save becomes a revision

- **WHEN** the editor saves a changed document in full
- **THEN** the difference from the stored state is derived as operations, committed as a revision, and sanitized before persistence

#### Scenario: Changing the template is a revision

- **WHEN** the candidate picks a different template
- **THEN** the change is committed as a revision addressing the template, and appears in the feed like any other edit

#### Scenario: Seeding a tailored copy opens the feed

- **WHEN** a tailored copy is created from the base CV
- **THEN** its feed begins with a revision attributed to the system
