## ADDED Requirements

### Requirement: The tailoring agent requests confirmation through a dedicated tool, not free text

Under the `tailor` preset, the agent SHALL request the candidate's confirmation of an
unbanked claim by calling `request_confirmation` with the exact claim text and a short
question, rather than writing the request as prose in its reply. `request_confirmation`
SHALL be registered only for the `tailor` preset and SHALL have no side effect: it
returns `{"status": "awaiting_candidate_response"}` and writes nothing.

#### Scenario: The agent asks via the tool

- **WHEN** the tailoring agent needs the candidate to confirm a claim that has no
  citable evidence
- **THEN** it calls `request_confirmation` with the claim text and a question, and
  writes no separate free-text confirmation request for the same claim in that turn

#### Scenario: The tool is unavailable outside tailoring

- **WHEN** a session runs under any preset other than `tailor`
- **THEN** `request_confirmation` is not among the tools offered to the model

### Requirement: A confirmation request renders as the claim text plus Yes/No buttons

The client SHALL render a `request_confirmation` tool call as the claim text together
with two actions, **Yes** and **No**, instead of the generic collapsed tool-call
rendering every other tool receives.

#### Scenario: The claim and buttons are shown

- **WHEN** an assistant message contains a `request_confirmation` tool call
- **THEN** the claim text is shown in full alongside **Yes** and **No** actions

### Requirement: Confirming sends the claim text verbatim as an ordinary message

Activating **Yes** SHALL send the claim text, unmodified, as a message from the
candidate, taking the same path a typed message takes. Activating **No** SHALL send a
fixed decline message. Neither action SHALL call any endpoint other than the ordinary
message-send path — no dedicated confirmation endpoint exists.

#### Scenario: Yes replays the claim verbatim

- **WHEN** the candidate activates **Yes** on a confirmation request
- **THEN** the exact claim text is sent as their next chat message, unchanged in any
  way, through the same path a typed message takes

#### Scenario: No declines without replaying the claim

- **WHEN** the candidate activates **No** on a confirmation request
- **THEN** a fixed decline message is sent as their next chat message, and the claim
  text is not sent

#### Scenario: The reply becomes citable through the existing evidence check

- **WHEN** the candidate's replayed claim text lands in the transcript and the agent
  retries recording it
- **THEN** the existing verbatim-quote provenance check (unchanged by this capability)
  evaluates that transcript message like any other candidate message, with no
  confirmation-specific bypass
