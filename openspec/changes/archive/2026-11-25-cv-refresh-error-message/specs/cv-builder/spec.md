## ADDED Requirements

### Requirement: A refused base-CV reset surfaces the server's specific reason
When resetting the base CV from a résumé is refused, the SPA SHALL show the server's specific reason for the refusal when one is available, rather than a generic failure message.

#### Scenario: A list-cap refusal is shown verbatim
- **WHEN** resetting the base CV from a résumé is refused because a role or project would exceed the maximum bullet count
- **THEN** the candidate sees the server's message naming the affected role or project and stating that no existing content was deleted

#### Scenario: A failure with no server message falls back to a generic one
- **WHEN** resetting the base CV from a résumé fails for a reason that carries no usable message (e.g. a network failure)
- **THEN** the candidate sees a generic message directing them to retry from a tailoring workspace
