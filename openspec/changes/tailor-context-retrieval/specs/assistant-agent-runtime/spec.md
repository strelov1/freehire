## MODIFIED Requirements

### Requirement: Malformed tool calls are reported to the model, not crashed on

Tool arguments SHALL be decoded strictly: unknown fields, missing required
fields, and type mismatches SHALL be rejected. A rejected call and a failing
tool SHALL both be returned to the model as a tool result describing the error,
so it can correct itself within the same turn. Such a failure SHALL count
against the turn's step cap and MUST NOT abort the turn or surface as a request
error.

Where the valid values are a short, caller-owned set — the ids of the roles an achievement may
attach to, the values an enum admits — the error SHALL carry them. A message that only says the
argument was wrong costs a round to discover what would be right, and that round comes out of the
step cap the same turn is trying to spend on work.

#### Scenario: A mis-shaped argument object is corrected in-turn

- **WHEN** the model calls a tool with an argument object that fails strict decoding
- **THEN** the turn continues with a tool result naming the decoding problem, and the model may retry the call

#### Scenario: A failing dependency does not kill the turn

- **WHEN** a tool fails because a backing service is unavailable
- **THEN** the error is returned to the model as that tool's result and the turn continues within its step cap

#### Scenario: A rejected id names the ids that would have been accepted

- **WHEN** an achievement is recorded against an employment id that is not one of the caller's
- **THEN** the error lists the caller's employments with their ids, so the retry needs no separate lookup
