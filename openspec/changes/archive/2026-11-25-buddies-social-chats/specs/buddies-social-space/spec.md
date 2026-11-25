## ADDED Requirements

### Requirement: A job link in a chat message is previewed
A chat message that links to one of the site's job pages SHALL show that job under the message — company, title and location, linking to the job — for at most two links per message. A link that does not resolve to a job SHALL show no preview.

#### Scenario: Sharing a posting
- **WHEN** a member sends a message containing https://freehire.me/jobs/staff-go-x1
- **THEN** the message shows that posting's card under its text
