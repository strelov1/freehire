## ADDED Requirements

### Requirement: A turn outlives a reader that stopped reading

A turn SHALL NOT be cancelled because writing to its client failed. The system
SHALL treat an unwritable stream as a reader that is not listening, never as an
instruction to abandon the work, and SHALL carry the turn through to its own
end under the bounds it already has: the server-owned step cap and the model
call timeout. Every message the turn produces SHALL be persisted whether or not
anyone is reading, so the session holds the whole turn and not the part that
happened to be delivered.

#### Scenario: A backgrounded tab does not stop the work

- **WHEN** the client stops reading mid-turn because its tab was backgrounded or its connection dropped
- **THEN** the turn keeps running to its own end, and every message and tool result it produces is stored on the session

#### Scenario: An unattended run finishes and files its report

- **WHEN** an unattended tailoring run loses its reader after editing the CV but before filing its report
- **THEN** the run still reaches its report, and the report is stored on the CV

### Requirement: Cancelling a turn is an explicit act

The system SHALL offer an owner-scoped endpoint that cancels the in-flight turn
of a named session, and that endpoint SHALL be the only way a caller stops a
turn. Cancellation SHALL stop the turn before its next model call and SHALL NOT
start new tool work. Work already committed by a tool SHALL remain committed, and
the partial transcript SHALL be persisted so the session is resumable. A session
that has no turn running SHALL report success rather than an error, so a client
may cancel without first proving that there is something to cancel.

#### Scenario: The user stops a running turn

- **WHEN** the owner asks to cancel a session whose turn is running
- **THEN** the turn stops before its next model call, the work already committed stands, and the transcript up to that point is stored

#### Scenario: Cancelling an idle session is not an error

- **WHEN** the owner asks to cancel a session with no turn in flight
- **THEN** the request succeeds and nothing is changed

#### Scenario: A stranger cannot stop someone else's turn

- **WHEN** a caller who does not own the session asks to cancel it
- **THEN** the session is reported as missing and the turn continues untouched

### Requirement: One turn at a time within a session

A session SHALL run at most one turn at a time. A message that arrives while a
turn is in flight SHALL wait for that turn rather than run beside it, and the
client SHALL be told it is waiting rather than left to guess. The wait SHALL be
bounded in time. At most one message SHALL wait: a further message arriving
while one is already waiting SHALL be refused, because a queue a client can grow
without limit is a way to hold the process open.

#### Scenario: A second message waits its turn

- **WHEN** the user sends a message while a turn is still running in that session
- **THEN** the client is told the message is queued, and the message runs as its own turn once the running one ends

#### Scenario: A third message is refused

- **WHEN** a message arrives while one turn is running and another is already waiting
- **THEN** the request is refused and the running and waiting turns are unaffected

### Requirement: A returning client is shown what it missed

A client that lost the stream SHALL be able to recover the turn's outcome by
re-reading the session, and the system SHALL serve the full transcript including
anything produced while nothing was reading. A stream that ended without its
terminal event SHALL NOT be presented as a failed turn — a reader that stopped
reading is not a turn that failed, and showing it as one misreports the state of
the user's own CV.

#### Scenario: Returning to a backgrounded tab

- **WHEN** the user returns to a tab whose stream was interrupted while a turn ran
- **THEN** re-reading the session shows the messages the agent produced in the meantime

#### Scenario: An interrupted stream is not an error

- **WHEN** a stream ends without its terminal event
- **THEN** the turn is not rendered as errored

## REMOVED Requirements

### Requirement: A turn stops when the caller goes away

**Reason**: The requirement rested on the assumption that an unwritable stream
means the caller left. On a phone it usually means the tab was backgrounded for a
moment, so the rule threw away live work — an unattended tailoring pass lost its
report after twenty-five committed CV edits. It also made a deliberate stop
indistinguishable from a blinking network, because both reached the server only
as a failed write.

**Migration**: Replaced by "A turn outlives a reader that stopped reading" and
"Cancelling a turn is an explicit act". A client that stopped a turn by dropping
its connection MUST now call the cancel endpoint; dropping the connection leaves
the turn running to its step cap.
