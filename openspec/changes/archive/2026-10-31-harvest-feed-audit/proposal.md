## Why

Harvesting excludes known boards and only proves that a board returns jobs. It cannot find a structured feed that could simplify an existing parser or expose fields lost by switching to it. Live research for issue #3990 found usable Scalis XML and Teamtailor RSS, with meaningful differences and exceptions.

## What Changes

- Add a read-only `harvest-boards -audit-feeds` mode for Scalis and Teamtailor.
- Audit existing catalog boards or an explicit seed list, preserving the normal harvest path.
- Report the observed schema, canonical IDs, body comparison, field availability, request counts and failures as JSON lines.
- Bound boards, pagination, request rate and per-board runtime. Never automatically switch an adapter.

## Capabilities

### New Capabilities

- `harvest-feed-audit`: inspect documented feed candidates and compare them with the current adapter.

### Modified Capabilities

None.

## Impact

`cmd/harvest-boards` only; no database schema, dependencies, public API or ingestion behavior changes.
