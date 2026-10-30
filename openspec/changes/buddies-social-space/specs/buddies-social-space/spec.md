## ADDED Requirements

### Requirement: Buddies space
The system SHALL serve interview buddies at `/buddies` for signed-in accounts, with a feed at `/buddies`, the similar-members list at `/buddies/network`, conversations at `/buddies/chats` and a member at `/buddies/<handle>`. A signed-out visitor SHALL be sent to sign-in and returned to the page they asked for. Every page in the space SHALL be marked non-indexable. An account that has not joined SHALL see the join screen on every page of the space.

#### Scenario: Signed-out visitor
- **WHEN** a signed-out visitor opens `/buddies/network`
- **THEN** they are redirected to sign-in with a return address of `/buddies/network`

#### Scenario: Non-member
- **WHEN** a signed-in account that has not joined opens `/buddies/chats`
- **THEN** the join screen is shown instead of the conversations

### Requirement: Old addresses redirect
The system SHALL permanently redirect `/my/buddies` to `/buddies`, `/my/buddies?c=<id>` to `/buddies/chats?c=<id>`, and `/my/buddies/<handle>` to `/buddies/<handle>`. The unread-message notification SHALL link to `/buddies/chats?c=<id>`.

#### Scenario: Notification link from an old email
- **WHEN** a member opens `/my/buddies?c=42`
- **THEN** they land on `/buddies/chats?c=42`

### Requirement: Generated member avatar
Every place that draws a buddy SHALL draw the same generated avatar: two initials taken from the handle on a colour derived only from the handle. It SHALL NOT use any photo, including the Talent Network one.

#### Scenario: Same member, same picture
- **WHEN** the handle `amber-heron-42` is drawn in the feed and in a conversation row
- **THEN** both show the initials `AH` on the same colour

### Requirement: Member profile shows the year in numbers
A member's profile SHALL show their country and how many applications, interview moves and offer moves they shared in the last year, counting exactly the events the activity grid counts.

#### Scenario: Counts match the grid
- **WHEN** a member shared 3 applications and 1 interview move this year, and hid one more application
- **THEN** the profile shows 3 applications, 1 interview and 0 offers, and the grid counts 4
