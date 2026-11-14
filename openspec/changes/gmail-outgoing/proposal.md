## Why

The inbox discards the candidate's replies, so a future digest cannot describe both sides of a hiring conversation. Sync must capture outgoing replies without treating them as employer decisions.

## What Changes

- Store Gmail direction and recipients and expose them to inbox readers.
- Backfill sent replies in existing hiring threads once and discover new sent replies even when no new inbound anchor arrives.
- Exclude outgoing messages from classification, employer-reply events and automatic stage changes.

## Capabilities

### New Capabilities

- `gmail-outgoing`: Read-only synchronization of sent messages in known hiring conversations.

### Modified Capabilities

None.

## Impact

Gmail sync, SQL schema and queries, inbox API and web rendering, tests and mail documentation. No additional OAuth scope, sending capability, hosted-mail transport or model calls.
