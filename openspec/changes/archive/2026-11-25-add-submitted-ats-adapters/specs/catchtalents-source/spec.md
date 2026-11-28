## ADDED Requirements

### Requirement: Recognize Catch Talents employer boards

The system SHALL resolve employer board and posting URLs to the numeric company ID on the public Catch Talents host.

#### Scenario: A posting identifies the employer

- **WHEN** a user submits a Catch Talents posting URL with a company ID
- **THEN** the system resolves the employer board using that company ID

#### Scenario: The central index has no company ID

- **WHEN** a user submits the site-wide Catch Talents index
- **THEN** the system leaves the employer board unspecified

### Requirement: Crawl complete employer listings

The adapter SHALL fetch the complete company listing and fetch details for unseen postings.

#### Scenario: A listing ends early

- **WHEN** the listing ends before its reported total has been reached
- **THEN** the adapter fails the crawl
