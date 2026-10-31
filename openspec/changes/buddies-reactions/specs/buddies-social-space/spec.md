## ADDED Requirements

### Requirement: Members react to feed events
A member SHALL be able to give one reaction — congrats, luck, rooting, same or insight — to an event their own buddy feed shows, replace it with another, or remove it. An event the member's feed does not show SHALL answer not found. The feed SHALL show each event's reaction counts by kind and the viewer's own reaction, counting only reactions from active members not blocked with the viewer.

#### Scenario: Changing a reaction
- **WHEN** a member reacted luck to an interview and then chooses congrats
- **THEN** the event counts one congrats from them and no luck

#### Scenario: Reacting across a block
- **WHEN** a member reacts to an event by someone who blocked them
- **THEN** the request answers not found
