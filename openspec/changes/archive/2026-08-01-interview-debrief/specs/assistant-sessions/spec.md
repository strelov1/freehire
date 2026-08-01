## MODIFIED Requirements

### Requirement: The session list spans every conversation the user can return to

The session list SHALL contain every conversation of the caller's except a tailoring
one, most recently active first — general chats, experience interviews, browsing
sessions, rehearsals and debriefs alike. A conversation begun in the extension's side
panel is one the candidate can pick up at their desk, so it belongs in the same rail;
only tailoring sessions stay out, because each is reached through the CV that owns it.

A rehearsal and a debrief bind to a vacancy rather than to nothing, and are still listed:
what keeps a conversation out of the rail is having its own way in, not having a binding.

#### Scenario: A browsing session appears in the rail

- **WHEN** the caller has held a conversation from the extension's side panel and requests the session list
- **THEN** that conversation is in the response alongside their general chats

#### Scenario: A debrief appears in the rail

- **WHEN** the caller has debriefed an interview and requests the session list
- **THEN** that debrief is in the response, named after the vacancy it was held against

#### Scenario: A tailoring conversation is still absent

- **WHEN** the caller has a CV-tailoring conversation and requests the session list
- **THEN** it is absent from the response
