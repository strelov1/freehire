## ADDED Requirements

### Requirement: The posting's dates read as icons, not sentences

The job detail page SHALL state when a posting went up and when its words last moved as an
icon and a compact relative time, not as the words "Posted" and "Updated" spelled out. The
compact form SHALL be produced by the platform's own short relative-time formatting, so it
follows the reader's locale rather than being abbreviated by hand.

Each icon SHALL carry the word it replaces in its accessible name and the exact timestamp
in its tooltip. An icon that only a sighted reader can decode states the fact to nobody
else.

#### Scenario: The dates render compactly

- **WHEN** a posting went up within the last day and has since been updated
- **THEN** the header shows a clock icon with the compact age, and a refresh icon with the compact update age
- **AND** neither the word "Posted" nor the word "Updated" is drawn on screen

#### Scenario: The words survive for assistive tech

- **WHEN** a screen reader reaches either date
- **THEN** it announces the word the icon replaced — "Posted" or "Updated" — before the time

#### Scenario: The exact instant stays reachable

- **WHEN** the reader hovers either date
- **THEN** the tooltip names the field and its full date and time

#### Scenario: An unchanged posting states its date once

- **WHEN** a posting's update time renders to the same label as its posting time
- **THEN** only the posting date renders, as it does today

### Requirement: The posting date is stated once per page

The job detail page SHALL NOT restate the posting's date beside the reality badge. The
badge's age chip and the posting date sit on the same line, so the contrast between them —
a long-open role whose source rewrites its date every crawl — is already visible to the
reader; spelling it out a second time printed the identical phrase twice on one line.

#### Scenario: No duplicate phrase on the provenance line

- **WHEN** a posting is long-open and its source date reads much fresher than its true age
- **THEN** the reality badge shows only its age chip and its remaining evidence
- **AND** the posting date appears exactly once on the line, in the dates group

#### Scenario: The reality badge keeps its other evidence

- **WHEN** the reality signal carries evidence beyond the posting date
- **THEN** that evidence still renders beside the badge, and the badge's tooltip is unchanged
