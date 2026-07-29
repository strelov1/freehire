## ADDED Requirements

### Requirement: The assistant is open to every signed-in user

Every assistant route SHALL serve any caller it can authenticate, and SHALL apply no
membership test beyond authentication. A signed-in user SHALL reach the assistant whatever
their `role` and whatever the state of their `beta_tester` flag; an unauthenticated caller
SHALL still be refused, and owner scoping SHALL still confine each caller to their own
conversations. The `/my/assistant` page and its account-nav entry SHALL be shown to every
signed-in user.

The `beta_tester` flag SHALL remain on the user account and SHALL remain exposed on the
authenticated user's profile (`/auth/me`), independent of `role`. The assistant simply
stops reading it — the flag outlives this consumer.

#### Scenario: A user with no special standing opens the assistant

- **WHEN** a signed-in user who is neither a moderator nor a beta tester requests the session list, creates a session, reads a transcript, or sends a turn
- **THEN** the request is served exactly as it would be for a beta tester

#### Scenario: The nav entry is shown to everyone signed in

- **WHEN** any signed-in user loads the account area
- **THEN** the "Agent" nav entry is present and `/my/assistant` renders the chat rather than a restricted-rollout notice

#### Scenario: Authentication is still required

- **WHEN** an assistant route is called with neither a cookie nor a Bearer credential
- **THEN** the request is refused and no session is created, read or advanced

## MODIFIED Requirements

### Requirement: The session API accepts a Bearer credential as well as the cookie

Every assistant session operation SHALL authenticate either the browser's session
cookie or an `Authorization: Bearer` credential — a session JWT or a full-scope
API key — resolving both to the same freehire user. A browser extension cannot
send hire's httpOnly cookie across origins, and holding a conversation from the
extension's side panel is why this widens.

Widening the carrier SHALL change nothing else: the owner scoping and the "a session you do
not own is missing, not forbidden" rule apply identically however the caller authenticated.

#### Scenario: A Bearer session JWT holds a conversation

- **WHEN** the session list, a transcript read, or a turn is requested with a valid session JWT as `Authorization: Bearer` and no cookie
- **THEN** the request is served as that user, subject to the same owner scoping as a cookie request

#### Scenario: An unauthenticated request is still refused

- **WHEN** an assistant route is called with neither a cookie nor a Bearer credential
- **THEN** the request is rejected and no session is created, read or advanced

## REMOVED Requirements

### Requirement: The assistant is gated to the beta-tester group

**Reason**: The gate was written when the assistant was free to us and cost the user their
own Claude subscription; #1165 moved the agent in-process, and the justification did not
survive the move. Meanwhile the product had already routed around it — the account nav
linked "Agent" for everyone and the tailoring workspace mounted the chat with its UI gate
switched off — so the only thing the gate still produced was a wall of `403`s for 358 of
360 accounts. The decision it encoded is hereby re-made rather than left to erode.

**Migration**: None for any caller. A signed-in user who previously received `403` now
receives the same response a beta tester received; no request that used to succeed changes
shape, and no stored session is affected. The `beta_tester` flag is not dropped — it stays
on the user model and on `/auth/me`, and granting or revoking it simply no longer affects
assistant access.

**Note**: this removes the gate, not the reason it was affordable. The assistant charges no
AI credits, so the spend it incurs was bounded by this gate alone. Metering assistant turns
is an owed follow-up, tracked outside this change.
