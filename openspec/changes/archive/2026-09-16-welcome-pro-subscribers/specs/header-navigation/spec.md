## ADDED Requirements

### Requirement: Paying-tier badge on the desktop profile icon

On viewports where the standalone signed-in profile icon is shown (desktop, `≥640px`), the
system SHALL display a small "PRO" or "ULTRA" badge on that icon when the signed-in
account's tier is `pro` or `ultra` respectively, and SHALL show no badge when the tier is
`free`.

#### Scenario: Pro subscriber sees a PRO badge

- **WHEN** a signed-in account whose tier is `pro` views any page on a desktop viewport
- **THEN** the header's profile icon shows a "PRO" badge

#### Scenario: Ultra subscriber sees an ULTRA badge

- **WHEN** a signed-in account whose tier is `ultra` views any page on a desktop viewport
- **THEN** the header's profile icon shows an "ULTRA" badge

#### Scenario: Free account sees no badge

- **WHEN** a signed-in account whose tier is `free` views any page on a desktop viewport
- **THEN** the header's profile icon shows no tier badge

#### Scenario: Signed-out visitor

- **WHEN** a signed-out visitor views any page on a desktop viewport
- **THEN** the header shows the sign-in control with no tier badge
