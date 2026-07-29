## ADDED Requirements

### Requirement: The header menu repeats what is opened from anywhere

The header menu SHALL carry the account sections a user reaches from outside the account shell —
the mail inbox, the agent, and the tailoring list — alongside its own links. These three are opened
in the middle of other work, so requiring a trip through the account shell to reach them costs a
navigation for no reason. The remaining sections stay in the account navigation only.

#### Scenario: The agent and the tailoring list are one click from anywhere

- **WHEN** a signed-in user opens the header menu from any page
- **THEN** it lists the inbox, the agent and the tailoring section among its account links

### Requirement: The tailoring section is named for its purpose

The account navigation SHALL name the section holding vacancy-bound CVs **Tailor**. Every CV it
lists is aimed at one posting; the earlier name described the CV-building tool the section grew out
of rather than what a user comes here to do.

#### Scenario: The section reads as Tailor

- **WHEN** the account navigation is rendered
- **THEN** the `/my/cvs` section is labelled "Tailor"
