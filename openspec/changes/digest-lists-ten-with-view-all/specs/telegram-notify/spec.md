## MODIFIED Requirements

### Requirement: Telegram digest sender

The system SHALL send a subscription digest to a linked chat via the Telegram Bot
API as the `telegram` channel's `Notifier` implementation. A send failure SHALL be
reported to the caller so the delivery retry/dead-letter policy applies.

The message SHALL itemize at most ten jobs — the same bound the email channel
applies, so the two channels never disagree about a digest's shape — and SHALL
additionally stop short of that bound when the next job line would carry the message
past Telegram's own message-length limit, because an oversized message is rejected
deterministically and would dead-letter the whole batch. Whatever the listing omits
SHALL be summarized as a "+ N more" tail that links to the digest's matched-jobs
page, `<origin>/my/notifications/<id>/jobs`, falling back to
`<origin>/my/notifications` when the digest carries no notification id.

#### Scenario: Send a digest

- **WHEN** the worker delivers a `telegram` digest for a linked user
- **THEN** the sender posts one message to the user's `chat_id` and reports success or failure to the delivery loop

#### Scenario: A digest of more than ten jobs lists ten and links to the rest

- **WHEN** a digest of 67 matched jobs carrying notification id 42 is rendered for Telegram
- **THEN** the message itemizes 10 jobs and ends with a "+ 57 more" tail linking to `<origin>/my/notifications/42/jobs`

#### Scenario: A message that would overflow is truncated further

- **WHEN** ten job lines would carry the message past Telegram's message-length limit
- **THEN** the message itemizes fewer than ten and the tail's count covers every omitted job
