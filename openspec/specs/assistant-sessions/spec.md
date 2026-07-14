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
full prior transcript. The page SHALL detach from the current session and
re-attach to the selected one from the start of its journal, folding the
replayed events through the existing chat reducer.

#### Scenario: Selecting an older session shows its messages

- **WHEN** the user clicks a session that already contains a multi-message exchange
- **THEN** the chat pane clears and repaints that session's full user/assistant/tool history in order, and further messages continue that conversation

#### Scenario: Switching away and back preserves the conversation

- **WHEN** the user switches from session A to session B and later back to A
- **THEN** session A's transcript is shown again in full, reconstructed from the backend journal

#### Scenario: Switching mid-turn is safe

- **WHEN** the user switches sessions while a turn is streaming in the current one
- **THEN** the page ends/abandons the in-flight turn cleanly for the old session and does not interleave its frames into the newly selected session

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

The agent backend SHALL scope the session list to the authenticated caller and
SHALL expose deletion. `GET /sessions` SHALL return only sessions owned by the
caller. `DELETE /sessions/{id}` SHALL, for an owned session, close it if live
and remove its metadata so it no longer appears in the list nor accepts an
attach; for a session the caller does not own it SHALL fail without side effects.

#### Scenario: List is filtered by owner

- **WHEN** `GET /sessions` is called with a valid session cookie
- **THEN** the response contains every session whose `created_by` is the caller and no session owned by anyone else

#### Scenario: Delete an owned session

- **WHEN** `DELETE /sessions/{id}` is called for a session the caller owns
- **THEN** the backend closes it if it is live, removes its `session_meta` row, and returns success; a subsequent list omits it and an attach to it is rejected as not owned

#### Scenario: Delete a non-owned or unknown session

- **WHEN** `DELETE /sessions/{id}` names a session the caller does not own or that does not exist
- **THEN** the backend returns an error status and makes no change

### Requirement: The assistant is gated to the beta-tester group

Access to the assistant SHALL be restricted to members of a beta-tester group,
represented by a `beta_tester` flag on the user account that is independent of
`role` (a user may be both a moderator and a beta tester, or either alone). The
`/my/assistant` page and its account-nav entry SHALL be shown only to beta
testers; a non-beta user (including a moderator without the flag) SHALL NOT see
the nav entry and SHALL be stopped at the page. The flag SHALL be exposed on the
authenticated user's profile (`/auth/me`) so the client can gate the UI.
Membership is granted out-of-band (manual SQL); no self-service grant exists.

#### Scenario: Beta tester sees and can open the assistant

- **WHEN** a signed-in user whose account has `beta_tester = true` loads the account area
- **THEN** the "Agent" nav entry is present and the `/my/assistant` page renders the chat

#### Scenario: Non-beta user cannot access the assistant

- **WHEN** a signed-in user without the beta-tester flag (including a plain moderator) loads the account area
- **THEN** the "Agent" nav entry is absent and visiting `/my/assistant` directly shows the restricted-rollout notice instead of connecting

#### Scenario: The flag is independent of role

- **WHEN** the authenticated user's profile is fetched
- **THEN** it reports `beta_tester` separately from `role`, so granting beta access does not change the user's role and vice versa

