## ADDED Requirements

### Requirement: Hiring-shaped Gmail sync

The system SHALL, for each connected user, read their mail via the Gmail API restricted to
mail that is hiring-shaped — sent from a curated ATS sender domain, OR carrying one of the
recognised application and interview phrasings — and MUST NOT ingest mail matching neither.

The phrasings SHALL cover the wordings employers actually use rather than one canonical
form of each. Measured against a live mailbox, the sync fetched 431 messages over 120 days
where a hiring-shaped query found 1151: the misses were near misses, an acknowledgement
reading "we've received your … application" where the list knew only "your application at",
and an invitation reading "interview invite" where it knew only "invite you to interview".

The sync SHALL NOT ingest messages the connected account itself sent, and SHOULD NOT fetch
them. The storage guard already exists; the fetch-side exclusion is what stops those
messages consuming the query's results and a body retrieval each.

#### Scenario: Mail from an ATS sender is ingested

- **WHEN** the sync worker runs and the mailbox holds mail from a configured ATS domain
- **THEN** that mail is fetched and stored

#### Scenario: Mail phrased as an application or an interview is ingested

- **WHEN** the mailbox holds a message from a domain on no list whose text carries a
  recognised application or interview phrasing
- **THEN** that mail is fetched and stored

#### Scenario: Mail that is neither is ignored

- **WHEN** the mailbox holds mail from an unrecognised sender with no recognised phrasing
- **THEN** that mail is never fetched or stored

#### Scenario: The candidate's own mail is neither fetched nor stored

- **WHEN** the mailbox holds a message the connected account sent
- **THEN** the query does not ask for it
- **AND** it is not stored, whether or not its text is hiring-shaped

## REMOVED Requirements

### Requirement: ATS-scoped Gmail sync

**Reason**: Replaced by "Hiring-shaped Gmail sync". The name and the rule had both drifted
from the code: it required that the sync "MUST NOT ingest non-ATS mail" and that non-ATS
mail is "never fetched or stored", which stopped being true when the multilingual phrase
clauses were added — mail from a domain on no list has been ingested for as long as those
have existed. Widening the phrasings makes the gap wider still, so the requirement is
restated around what the fetch is actually scoped to rather than corrected in place.

**Migration**: None. No behaviour is removed; the successor covers ATS senders as one of
its two admitted classes.
