# buddies-social-space Specification

## Purpose
TBD - created by archiving change buddies-reaction-notice. Update Purpose after archive.
## Requirements
### Requirement: The author hears about a reaction
A reaction SHALL put one notice in the reacted-to event's author's notification center per reacting member, naming the reaction and the news, never the member. Changing or re-giving that member's reaction SHALL NOT add another.

#### Scenario: A member changes their mind
- **WHEN** a member reacts congrats to an offer, then switches to luck
- **THEN** the author has exactly one notice from that member's reaction

### Requirement: Members react to feed events
A member SHALL be able to give one reaction — congrats, luck, rooting, same or insight — to an event their own buddy feed shows, replace it with another, or remove it. An event the member's feed does not show SHALL answer not found. The feed SHALL show each event's reaction counts by kind and the viewer's own reaction, counting only reactions from active members not blocked with the viewer.

#### Scenario: Changing a reaction
- **WHEN** a member reacted luck to an interview and then chooses congrats
- **THEN** the event counts one congrats from them and no luck

#### Scenario: Reacting across a block
- **WHEN** a member reacts to an event by someone who blocked them
- **THEN** the request answers not found

### Requirement: A job link in a chat message is previewed
A chat message that links to one of the site's job pages SHALL show that job under the message — company, title and location, linking to the job — for at most two links per message. A link that does not resolve to a job SHALL show no preview.

#### Scenario: Sharing a posting
- **WHEN** a member sends a message containing https://freehire.me/jobs/staff-go-x1
- **THEN** the message shows that posting's card under its text

### Requirement: Feed posts link to the posting
Each buddy feed entry SHALL carry the public address and location of the posting its event names, when that posting exists and is not private. The feed SHALL link an interview, an offer or an application to that posting, and to the company when the posting is gone.

#### Scenario: Interview links to the job
- **WHEN** a member moved to interview on a posting that still has a page
- **THEN** their feed post links to that job's page

### Requirement: One member's applications on one day read as one post
The feed SHALL show two or more applications by the same member on the same day as a single post naming every posting, and SHALL group posts under their day.

#### Scenario: Four applications in a day
- **WHEN** a member applied to four postings yesterday
- **THEN** the feed shows one post under "Yesterday" listing the four postings

