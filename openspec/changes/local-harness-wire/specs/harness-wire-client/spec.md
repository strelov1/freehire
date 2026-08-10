## ADDED Requirements

### Requirement: A harness opens the wire with the stored API key

A harness client SHALL connect to hire's browser-tool relay in the harness role,
authenticating with the `fhk_…` API key the user already holds, and SHALL NOT ask
for a new credential, scope, or login flow.

#### Scenario: Connects with the credential already on disk

- **WHEN** a harness client starts and `~/.freehire/creds.json` holds an API key
- **THEN** it opens the wire as the harness end of that user's channel

#### Scenario: No credential

- **WHEN** no API key is configured
- **THEN** the client says so and names the command that fixes it, rather than
  failing on a socket error

#### Scenario: The channel is the key's owner's

- **WHEN** a harness connects
- **THEN** it can reach only the browser of the user the key belongs to, never
  another user's

### Requirement: A call the browser cannot answer is reported, not hung

Every call SHALL end in an answer or a named failure within a bounded time. The
client SHALL wait longer than the extension's own waiting on a widget, so a slow
widget is never reported as a failure that did not happen.

#### Scenario: No browser attached

- **WHEN** a harness issues a call while no extension end is connected
- **THEN** the client reports that the browser is not connected and names what the
  user does about it

#### Scenario: A widget takes time to answer

- **WHEN** a primitive waits on a widget re-rendering
- **THEN** the client is still waiting when the answer arrives, and reports the
  real outcome

#### Scenario: Nothing answers at all

- **WHEN** no answer arrives within the client's budget
- **THEN** the call ends as a timeout naming the tool, and the harness is free to
  continue with other work

### Requirement: An unrecognised outcome is surfaced verbatim

The extension owns the vocabulary of outcomes and may extend it. A client SHALL
pass an outcome it does not recognise through unchanged, and SHALL NOT map it onto
a familiar one.

#### Scenario: A status the client has never seen

- **WHEN** a primitive answers with an outcome the client does not know
- **THEN** the harness sees that outcome as-is, so drift appears as an unfamiliar
  word rather than as a wrong result

### Requirement: A displaced harness does not fail silently

The relay keeps one harness per user and the newest connection replaces the
previous one. A client that is displaced SHALL notice and either reconnect or say
it was displaced — it SHALL NOT continue as though still attached.

#### Scenario: A second harness connects

- **WHEN** another harness (or a one-shot command) joins the same user's channel
- **THEN** the displaced client stops claiming to be attached

#### Scenario: A long-running client survives a drop

- **WHEN** the connection is lost while the host is still running
- **THEN** the client reconnects, so the next call does not fail on a dead socket

### Requirement: Shutting down does not strand the browser

A harness SHALL close its end on shutdown, and SHALL NOT leave an in-flight call
without an outcome.

#### Scenario: Interrupted mid-call

- **WHEN** the user interrupts a harness while a call is in flight
- **THEN** the call ends with a stated outcome and the connection is closed
