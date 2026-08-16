# assistant-sessions Specification

## Purpose
TBD - created by archiving change assistant-multi-session. Update Purpose after archive.
## Requirements
### Requirement: The assistant page lists the caller's held sessions

The `/my/assistant` page SHALL show a sidebar listing the signed-in user's
existing agent sessions, newest first, so the user can see and return to prior
conversations instead of only the one spawned on load.

#### Scenario: Sidebar shows the caller's sessions on load

- **WHEN** a moderator opens `/my/assistant` and the backend has two sessions owned by them
- **THEN** the sidebar lists both sessions with a human label, and the most recently active one is opened in the chat pane

#### Scenario: The list never shows another user's sessions

- **WHEN** the page requests the session list
- **THEN** it calls the owner-scoped backend list and displays only sessions whose owner is the caller — never a session created by a different user

#### Scenario: Empty state

- **WHEN** the caller has no prior sessions
- **THEN** the page creates one fresh session, opens it, and the sidebar shows that single entry

### Requirement: Starting a new chat

The page SHALL let the user start a new conversation without losing existing
ones. Creating a new chat SHALL create a fresh backend session, add it to the
top of the sidebar, and make it the active pane; existing sessions remain in the
list.

#### Scenario: New chat keeps prior sessions

- **WHEN** the user clicks "New chat" while a session with history is open
- **THEN** a new empty session is created and becomes active, and the previously open session is still listed in the sidebar and reopenable

#### Scenario: New chat is the active input target

- **WHEN** a new chat has just been created
- **THEN** a message the user sends is delivered to the new session, not the previously active one

### Requirement: Switching between sessions replays history

Selecting a session in the sidebar SHALL make it the active pane and repaint its
full prior transcript. The page SHALL load that session's stored transcript from
the freehire API and fold it through the existing chat reducer, so replayed
history and live turn events render identically.

#### Scenario: Selecting an older session shows its messages

- **WHEN** the user clicks a session that already contains a multi-message exchange
- **THEN** the chat pane clears and repaints that session's full user/assistant/tool history in order, and further messages continue that conversation

#### Scenario: Switching away and back preserves the conversation

- **WHEN** the user switches from session A to session B and later back to A
- **THEN** session A's transcript is shown again in full, reconstructed from the stored messages

#### Scenario: Switching mid-turn is safe

- **WHEN** the user switches sessions while a turn is streaming in the current one
- **THEN** the page ends the in-flight turn cleanly for the old session and does not interleave its events into the newly selected session

### Requirement: Deleting a session

The user SHALL be able to delete a session from the sidebar. Deleting SHALL
remove it from the caller's list permanently via the backend, and the deleted
session SHALL no longer be attachable.

#### Scenario: Delete removes the session from the list

- **WHEN** the user deletes a session that is not currently open
- **THEN** it disappears from the sidebar and does not reappear on reload

#### Scenario: Deleting the active session

- **WHEN** the user deletes the session currently open in the chat pane
- **THEN** the page switches to another session in the list (or creates a fresh one if none remain) and the deleted session is gone

#### Scenario: Delete is owner-guarded

- **WHEN** a delete is requested for a session the caller does not own
- **THEN** the backend rejects it and the session is not removed

### Requirement: Owner-scoped session backend contract

The freehire API SHALL own assistant sessions and SHALL scope every session
operation to the authenticated caller. Creating a session SHALL persist it for
that caller. Listing SHALL return only sessions owned by the caller, newest
first. Reading a session's transcript SHALL return only that caller's session's
messages. Deleting SHALL remove an owned session and its transcript so it no
longer appears in the list, cannot be read, and cannot accept a further turn.
An operation naming a session the caller does not own or that does not exist
SHALL fail without side effects and MUST NOT reveal whether the session exists.

#### Scenario: List is filtered by owner

- **WHEN** the session list is requested with a valid session cookie
- **THEN** the response contains every chat owned by the caller and no session owned by anyone else

#### Scenario: A tailoring conversation is not a chat

- **WHEN** the caller has a CV-tailoring conversation and requests the session list
- **THEN** it is absent from the response, and the chat sidebar therefore never offers it

#### Scenario: Delete an owned session

- **WHEN** a delete is requested for a session the caller owns
- **THEN** the session and its stored messages are removed and success is returned; a subsequent list omits it and a turn sent to it is rejected

#### Scenario: Delete a non-owned or unknown session

- **WHEN** a delete names a session the caller does not own or that does not exist
- **THEN** the request fails, makes no change, and does not distinguish the two cases

#### Scenario: Reading another user's transcript is refused

- **WHEN** a transcript read names a session owned by a different user
- **THEN** the request fails and no message content is returned

### Requirement: The session API accepts a Bearer credential as well as the cookie

Every assistant session operation SHALL authenticate either the browser's session
cookie or an `Authorization: Bearer` credential — a session JWT or a full-scope
API key — resolving both to the same freehire user. A browser extension cannot
send hire's httpOnly cookie across origins, and holding a conversation from the
extension's side panel is why this widens.

Widening the carrier SHALL change nothing else: the owner scoping and the "a
session you do not own is missing, not forbidden" rule apply identically however
the caller authenticated.

#### Scenario: A Bearer session JWT holds a conversation

- **WHEN** the session list, a transcript read, or a turn is requested with a valid session JWT as `Authorization: Bearer` and no cookie
- **THEN** the request is served as that user, subject to the same owner scoping as a cookie request

#### Scenario: An unauthenticated request is still refused

- **WHEN** an assistant route is called with neither a cookie nor a Bearer credential
- **THEN** the request is rejected and no session is created, read or advanced

### Requirement: The session list spans every conversation the user can return to

The session list SHALL contain every conversation of the caller's except a tailoring
or a browsing one, most recently active first — general chats, experience
interviews, rehearsals and debriefs alike. Tailoring sessions stay out because each
is reached through the CV that owns it; browsing sessions stay out because their
only useful reading — `read_current_page` — works solely over the extension's own
Bearer-JWT connection, so surfacing them on the website would list a conversation
that degrades to an ordinary chat the moment it is opened there.

A rehearsal and a debrief bind to a vacancy rather than to nothing, and are still
listed: what keeps a conversation out of the rail is having its own way in (the
extension, for browsing; the CV, for tailoring), not having a binding.

#### Scenario: A browsing session appears in the rail

- **WHEN** the caller has held a conversation from the extension's side panel and requests the session list
- **THEN** that conversation is absent from the response; it remains reachable from the extension, which holds its id directly rather than listing it

#### Scenario: A debrief appears in the rail

- **WHEN** the caller has debriefed an interview and requests the session list
- **THEN** that debrief is in the response, named after the vacancy it was held against

#### Scenario: A tailoring conversation is still absent

- **WHEN** the caller has a CV-tailoring conversation and requests the session list
- **THEN** it is absent from the response

### Requirement: The assistant is open to every signed-in user

Every assistant route SHALL serve any caller it can authenticate, and SHALL apply
no membership test beyond authentication. A signed-in user SHALL reach the
assistant whatever their `role` and whatever the state of their `beta_tester`
flag; an unauthenticated caller SHALL still be refused, and owner scoping SHALL
still confine each caller to their own conversations. The `/my/assistant` page and
its account-nav entry SHALL be shown to every signed-in user.

The `beta_tester` flag SHALL remain on the user account and SHALL remain exposed
on the authenticated user's profile (`/auth/me`), independent of `role`. The
assistant simply stops reading it — the flag outlives this consumer.

#### Scenario: A user with no special standing opens the assistant

- **WHEN** a signed-in user who is neither a moderator nor a beta tester requests the session list, creates a session, reads a transcript, or sends a turn
- **THEN** the request is served exactly as it would be for a beta tester

#### Scenario: The nav entry is shown to everyone signed in

- **WHEN** any signed-in user loads the account area
- **THEN** the "Agent" nav entry is present and `/my/assistant` renders the chat rather than a restricted-rollout notice

#### Scenario: Authentication is still required

- **WHEN** an assistant route is called with neither a cookie nor a Bearer credential
- **THEN** the request is refused and no session is created, read or advanced

#### Scenario: The flag is independent of role

- **WHEN** the authenticated user's profile is fetched
- **THEN** it reports `beta_tester` separately from `role`, so granting beta access does not change the user's role and vice versa

### Requirement: A session id is unguessable

A session's id SHALL be random, not sequential. Access is confined by ownership
on every operation, so the id is not a capability — but a countable id would
publish how many conversations the platform has created, and would turn a single
missing owner check on any future endpoint into bulk extraction rather than one
unusable request. An id that is not well-formed SHALL be reported as a missing
session, so "not a session" and "not yours" stay one answer.

#### Scenario: Two sessions get unrelated ids

- **WHEN** the same caller creates two conversations in a row
- **THEN** their ids are independently random, so neither reveals the other nor how many exist

#### Scenario: A malformed id is missing, not invalid

- **WHEN** a request names a session id that is not well-formed
- **THEN** it is refused as not found, indistinguishable from a session the caller does not own

### Requirement: A conversation has its own address

Each chat SHALL be addressable by a URL carrying its id, so it can be
bookmarked, reopened later, and reached with the browser's Back button.
Selecting a chat SHALL navigate to that chat's address, adding a history entry;
entering the assistant without an id SHALL open the newest chat (or start one)
and replace the address with that chat's own, so landing there is not itself a
step in the user's history.

#### Scenario: Switching chats changes the address

- **WHEN** the user selects a different chat in the sidebar
- **THEN** the address becomes that chat's URL, and pressing Back returns to the chat they came from

#### Scenario: A saved link reopens its chat

- **WHEN** the user opens a previously saved chat URL
- **THEN** that chat is opened and its transcript is repainted

#### Scenario: A dead link explains itself

- **WHEN** the URL names a conversation the caller cannot open on this surface — deleted, another user's, a tailoring conversation that belongs to a CV, or a browsing conversation held from the extension
- **THEN** the page says the chat is unavailable and offers a way back to the chat list, rather than silently opening a different conversation or opening it here with reduced capability

