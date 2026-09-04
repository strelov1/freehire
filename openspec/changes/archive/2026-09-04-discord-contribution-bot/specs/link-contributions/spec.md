## ADDED Requirements

### Requirement: Contribute a board from Discord

The system SHALL let a user who has linked their Discord identity submit a link via the bot's
`/contribute <url>` command: the interaction resolves the invoking Discord identity to its user
and runs the same intake sequence as every other surface, replying with the outcome — including a
link to the posting when the vacancy could be imported. A `/contribute` invocation from a Discord
identity not linked to any user SHALL import nothing and SHALL prompt the user to link their
account first, identical to the Telegram bot's behavior for an unlinked chat.

#### Scenario: A readable vacancy is imported and linked back

- **WHEN** a linked user runs `/contribute` with a link to a vacancy that can be imported
- **THEN** the vacancy is imported and the bot replies with a link to the posting

#### Scenario: Novel board is recorded and rewarded

- **WHEN** a linked user runs `/contribute` with a supported board link for a board not yet
  crawled
- **THEN** the board is recorded, the user's AI-credits reward is credited, and the bot confirms
  the new board

#### Scenario: Second link on the same board earns no reward

- **WHEN** a linked user runs `/contribute` for a board they already contributed
- **THEN** no AI credits are credited and the bot says the board is already known

#### Scenario: Unlinked identity is prompted to link

- **WHEN** `/contribute` is invoked by a Discord identity not linked to any user
- **THEN** nothing is imported or recorded, and the bot replies prompting the user to link their
  account on the site first
