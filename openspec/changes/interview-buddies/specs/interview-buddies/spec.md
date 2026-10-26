## ADDED Requirements

### Requirement: Opt-in buddy membership
The system SHALL let a signed-in account join and leave the buddy network explicitly. An account that has not joined SHALL NOT appear in any buddy list, feed or search, and SHALL NOT be able to read buddy lists, the feed or other members' profiles. Leaving SHALL close the member's conversations only while they are away: after they rejoin, their former conversation partners SHALL see their handle on the old conversation history again and MAY invite them again; blocking SHALL be the only permanent way to end contact.

#### Scenario: Joining creates a membership with a handle
- **WHEN** a signed-in account that is not a member joins
- **THEN** the system creates a membership with a newly minted anonymous buddy handle and the account can read similar members and the feed

#### Scenario: Non-member is refused buddy reads
- **WHEN** a signed-in account that has not joined requests similar members, the feed or a buddy profile
- **THEN** the system responds 403 and returns no member data

#### Scenario: Leaving hides the member and closes conversations
- **WHEN** a member leaves the network
- **THEN** the member no longer appears in any other member's similar list or feed, and every conversation they are part of becomes closed

#### Scenario: Rejoining keeps the handle
- **WHEN** a member who left joins again
- **THEN** the membership is restored with the same buddy handle it had before

#### Scenario: Rejoining restores the handle on old conversations
- **WHEN** member B left while in a conversation with A, with no block between them, and B joins again
- **THEN** A's conversation list shows B's handle on that conversation's history again, and A may invite B again; only a block keeps B hidden from A for good

#### Scenario: Unauthenticated access is refused
- **WHEN** a request without a session reaches any buddy endpoint
- **THEN** the system responds 401

### Requirement: Anonymous buddy profile
A buddy profile SHALL publish only values a dictionary resolved, numbers or dates, plus role titles verbatim, using the same projection as the Talent Network card. It SHALL NOT include the account's name, email, username, employer names, or any free text from the CV. The buddy handle SHALL be distinct from the account's Talent Network handle and username. Buddy pages SHALL NOT be listed in any sitemap and SHALL be marked non-indexable.

#### Scenario: Card carries no free text
- **WHEN** a member whose CV summary and role highlights name their employer is projected into a buddy card
- **THEN** the card contains none of the summary text, the highlights text or the employer name

#### Scenario: Handle differs from Talent Network handle
- **WHEN** an account is both a Talent Network member and a buddy member
- **THEN** its buddy handle is not equal to its Talent Network handle

#### Scenario: Member without a CV
- **WHEN** an account with no structured CV joins
- **THEN** the membership is created with an empty card and the profile response says the card is empty

### Requirement: Similar members ranking
The system SHALL list other active members ranked by a similarity score that adds points for the same category as the viewer, the same country, the same region, and each company both members applied to within the last 90 days. The list SHALL exclude the viewer, members who left, and any member either side has blocked. Each entry SHALL show the shared companies.

#### Scenario: Closer member ranks first
- **WHEN** member A shares category, country and two applied companies with the viewer, and member B shares only the category
- **THEN** A is listed before B

#### Scenario: Members with no overlap still appear
- **WHEN** a member shares nothing with the viewer
- **THEN** the member still appears, after every member with a positive score

#### Scenario: Blocked member is excluded both ways
- **WHEN** the viewer has blocked member C, or C has blocked the viewer
- **THEN** C does not appear in the viewer's similar list and the viewer does not appear in C's

#### Scenario: Old applications do not count
- **WHEN** the only company both members applied to was applied to more than 90 days ago by one of them
- **THEN** that company adds no points and is not shown as shared

### Requirement: Buddy feed of applications and stage moves
The system SHALL show members a feed of other active members' application events: an application, and a move into the interview or offer stage. Events for rejected, withdrawn, expired or accepted stages SHALL NOT appear. Each feed entry SHALL show the author's buddy card summary, the company, the job title when the posting still exists, the event type and the day it happened (no time of day). The feed SHALL offer a "similar" mode, which keeps recency order but restricts the feed to authors among the viewer's 200 most similar members, and an "all" mode, which is recency order over every active member. Events on a privately pasted posting SHALL NOT appear, and such a posting SHALL NOT count as a shared company. The viewer's own events and events of members either side has blocked SHALL NOT appear.

#### Scenario: Application appears in the feed
- **WHEN** an active member applies to a job
- **THEN** another member's "all" feed contains an entry for that company with the type "applied" and the day only

#### Scenario: Interview move appears
- **WHEN** an active member's application moves to the interview stage
- **THEN** the feed contains an entry with the type "interview" for that company

#### Scenario: Similar mode restricts authors, not order
- **WHEN** the viewer reads the "similar" feed
- **THEN** it contains only events by the viewer's 200 most similar members, newest first, and the "all" feed adds the events of every other active member

#### Scenario: Private posting never appears
- **WHEN** a member marks a privately pasted posting as applied
- **THEN** no other member's feed contains that event and its company is not counted as shared

#### Scenario: Rejection never appears
- **WHEN** a member's application moves to the rejected stage
- **THEN** no feed entry is produced for that move

#### Scenario: Events from before joining are included
- **WHEN** a member joins after having applied to jobs
- **THEN** their earlier applications that are not hidden appear in the feed

#### Scenario: Left member disappears from the feed
- **WHEN** a member leaves
- **THEN** none of their events appear in any feed

### Requirement: Member activity grid
A member's page SHALL show that member's last year as a per-day count, and SHALL count exactly the events the feed would list for that member to the viewer — nothing the feed excludes, and none of the tracker's own counted actions (follow-ups, other stage moves). Counts SHALL be at UTC day precision. A handle the viewer may not see SHALL answer not found, as the profile does.

#### Scenario: The grid counts what the feed lists
- **WHEN** a member has applications, interview and offer moves, rejections, follow-ups, a retracted event, a hidden application and an event on a private posting
- **THEN** the grid's total equals the number of that member's feed entries, day by day

#### Scenario: Blocked or own handle
- **WHEN** a viewer requests the grid of a member on either side of a block, or of their own handle
- **THEN** the response is not found

### Requirement: Per-application feed visibility
Every application SHALL be shown in the buddy feed by default. A member SHALL be able to hide a single application from the feed and show it again. Hiding SHALL remove all of that application's events from every feed immediately.

#### Scenario: Hiding removes the entries
- **WHEN** a member hides their application to a job
- **THEN** no feed contains any event of that application

#### Scenario: Showing again restores the entries
- **WHEN** the member shows the application again
- **THEN** its events appear in the feed again

### Requirement: Invitations
A member SHALL be able to invite another active member by sending a first message of 1 to 500 characters. There SHALL be at most one conversation per pair of members. A member SHALL be able to send at most 10 invitations per rolling 24 hours. Accepting and declining follow the `messaging` capability.

#### Scenario: Invitation creates a pending conversation
- **WHEN** member A invites member B with a message
- **THEN** a pending conversation exists between A and B holding that message, and B sees it as an incoming invitation

#### Scenario: Second invitation to the same member is refused
- **WHEN** A invites B while a conversation between them already exists that is pending, open or declined
- **THEN** the system responds 409 and creates nothing

#### Scenario: Re-inviting after a conversation closed by leaving
- **WHEN** a conversation between A and B was closed because one of them left, both are active members again with no block between them, and A invites B
- **THEN** the same conversation becomes pending with A as the inviter, keeps its earlier messages as read, holds the new message as unread for B, and counts toward A's daily limit

#### Scenario: A block keeps a closed conversation closed
- **WHEN** a closed conversation's pair has a block in either direction and either member invites the other
- **THEN** the system responds 403 without revealing the block, and the conversation stays closed

#### Scenario: Daily invitation limit
- **WHEN** A has sent 10 invitations in the last 24 hours and sends another
- **THEN** the system responds 429 with a message saying when the next invitation is possible

#### Scenario: Empty or oversized message is refused
- **WHEN** the invitation message is empty or longer than 500 characters
- **THEN** the system responds 400

#### Scenario: Inviting a non-member, a left member or a blocking member is refused
- **WHEN** A invites an account that is not an active member, or that has blocked A, or that A has blocked
- **THEN** the system responds 403 and creates nothing

### Requirement: Buddy conversations run on messaging
A buddy conversation SHALL be a `messaging` conversation of kind `buddy`, with one conversation per pair of members, and SHALL follow the `messaging` capability's rules for request and accept, messages and read state, blocks, reports and unread notices. Its counterpart SHALL be the other member's handle and card summary, and SHALL mark the other member unreachable once they have left the network or a block exists between the pair.

#### Scenario: Buddy chat lists the other member by handle
- **WHEN** a member lists their conversations
- **THEN** each buddy conversation names the other member by handle with their card summary, never by account details

#### Scenario: Left member is shown unreachable
- **WHEN** the other member has left the network
- **THEN** the conversation's counterpart is marked unreachable and carries no card

### Requirement: A block hides members from each other
A block between two accounts, made from any conversation, SHALL hide the two members from each other's similar list, feed and member pages, and SHALL stop either from inviting the other.

#### Scenario: Block hides both ways
- **WHEN** A blocks B
- **THEN** neither appears in the other's similar list or feed, neither can open the other's member page, and neither can invite the other

### Requirement: Account deletion removes buddy data
Deleting an account SHALL delete its membership and its hidden-application marks; its conversations, messages and blocks are removed as the `messaging` capability requires.

#### Scenario: Deleted account's conversation is gone
- **WHEN** A deletes their account
- **THEN** B no longer sees the conversation with A or any of its messages
