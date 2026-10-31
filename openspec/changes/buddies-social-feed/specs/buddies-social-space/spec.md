## ADDED Requirements

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
