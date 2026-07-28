## MODIFIED Requirements

### Requirement: Reject a board already in the catalogue

The system SHALL NOT record a contribution for a board it already crawls, and SHALL NOT
award AI credits for one, because the board needs no onboarding. The submitted link SHALL
still be served by the intake sequence — the vacancy is imported if it can be read — and
the caller SHALL be told which company is already tracked.

#### Scenario: A board we already crawl is not queued

- **WHEN** a user submits a link for a board that already has jobs in the catalogue
- **THEN** no contribution row is recorded, no credits are awarded, and the answer names
  the company already being crawled

#### Scenario: A vacancy on a crawled board is still imported

- **WHEN** the submitted link is a readable vacancy on an already-crawled board that the
  catalogue does not yet carry
- **THEN** the vacancy is imported and its posting is returned to the caller

### Requirement: Contribute a board from Telegram

The system SHALL let a user who has linked their Telegram chat submit a link by sending it
to the bot: the webhook resolves the chat to its user and runs the same intake sequence as
every other surface, replying with the outcome — including a link to the posting when the
vacancy could be imported. A message with no link SHALL draw no reply; a link from a chat
not linked to any user SHALL prompt the user to link their account first.

#### Scenario: A readable vacancy is imported and linked back

- **WHEN** a linked user sends a link to a vacancy that can be imported
- **THEN** the vacancy is imported and the bot replies with a link to the posting

#### Scenario: Novel board is recorded and rewarded

- **WHEN** a linked user sends a supported board link for a board we do not crawl
- **THEN** the board is recorded, the user's AI-credits reward is credited, and the bot
  confirms the new board

#### Scenario: Second link on the same board earns no reward

- **WHEN** a linked user sends another link for a board they already contributed
- **THEN** no AI credits are credited and the bot says the board is already known

#### Scenario: Ordinary chatter is ignored

- **WHEN** a linked user sends a message with no link
- **THEN** the bot does not reply

#### Scenario: Unlinked chat is prompted to link

- **WHEN** a link arrives from a chat not linked to any user
- **THEN** the bot replies prompting the user to link their account on the site first

### Requirement: My contributions view

The system SHALL let an authenticated user list their own contributions, newest first,
each carrying its canonical URL, status, the surface it was submitted from, and — for a
recognized board — its source and board slug; a review-queue row carries no source or
board. The list SHALL be scoped to the caller and never reveal another user's
contributions.

#### Scenario: User lists their own contributions

- **WHEN** an authenticated user requests their contributions
- **THEN** the response contains only that user's contributions, newest first, each with
  its status and originating surface

#### Scenario: A review-queue submission is listed without a board

- **WHEN** an authenticated user who submitted an unrecognized link requests their
  contributions
- **THEN** that row appears with status `review` and no source or board

## REMOVED Requirements

### Requirement: Contribution submissions are rate-limited per user

**Reason**: Superseded, not dropped. Submission no longer has its own endpoint — every
surface enters through the unified intake sequence, whose rate limiting is specified by
`posting-import-by-url` ("Import requests are authenticated and rate limited"). Keeping a
second, separately-worded limit for the same budget would let the two drift.

**Migration**: The per-user hourly budget, its `429` response, and its "no fetch before
refusing" guarantee are unchanged in behaviour; they are now specified once, on the intake
endpoint.
