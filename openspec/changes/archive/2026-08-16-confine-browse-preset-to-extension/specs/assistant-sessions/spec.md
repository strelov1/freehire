## MODIFIED Requirements

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
