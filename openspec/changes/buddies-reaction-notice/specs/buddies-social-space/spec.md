## ADDED Requirements

### Requirement: The author hears about a reaction
A reaction SHALL put one notice in the reacted-to event's author's notification center per reacting member, naming the reaction and the news, never the member. Changing or re-giving that member's reaction SHALL NOT add another.

#### Scenario: A member changes their mind
- **WHEN** a member reacts congrats to an offer, then switches to luck
- **THEN** the author has exactly one notice from that member's reaction
