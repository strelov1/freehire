## ADDED Requirements

### Requirement: The session API accepts a Bearer credential as well as the cookie

Every assistant session operation SHALL authenticate either the browser's session
cookie or an `Authorization: Bearer` credential — a session JWT or a full-scope
API key — resolving both to the same freehire user. A browser extension cannot
send hire's httpOnly cookie across origins, and holding a conversation from the
extension's side panel is why this widens.

Widening the carrier SHALL change nothing else: the rollout gate, the owner
scoping, and the "a session you do not own is missing, not forbidden" rule apply
identically however the caller authenticated.

#### Scenario: A Bearer session JWT holds a conversation

- **WHEN** the session list, a transcript read, or a turn is requested with a valid session JWT as `Authorization: Bearer` and no cookie
- **THEN** the request is served as that user, subject to the same rollout gate and owner scoping as a cookie request

#### Scenario: An unauthenticated request is still refused

- **WHEN** an assistant route is called with neither a cookie nor a Bearer credential
- **THEN** the request is rejected and no session is created, read or advanced

#### Scenario: A Bearer caller outside the rollout is still refused

- **WHEN** a valid Bearer credential belongs to a user who is neither a moderator nor a beta tester
- **THEN** the request is refused, exactly as the same user would be through the cookie

### Requirement: The session list spans every conversation the user can return to

The session list SHALL contain the caller's general chats and their browsing
conversations alike, most recently active first. A conversation begun in the
extension's side panel is one the candidate can pick up at their desk, so it
belongs in the same rail; only tailoring sessions stay out, because each is
reached through the CV that owns it.

#### Scenario: A browsing session appears in the rail

- **WHEN** the caller has held a conversation from the extension's side panel and requests the session list
- **THEN** that conversation is in the response alongside their general chats

#### Scenario: A tailoring conversation is still absent

- **WHEN** the caller has a CV-tailoring conversation and requests the session list
- **THEN** it is absent from the response
