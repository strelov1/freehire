## ADDED Requirements

### Requirement: Portfolio project URL is retained on import

When a structured résumé's portfolio project is imported into the experience bank, the system SHALL persist the project's outbound URL on that employment (alongside its kind and name). An empty URL MUST remain empty; import MUST NOT invent a link. Re-import FillBlanks semantics apply: an empty banked link MAY be filled from a later extract; a non-empty banked link MUST NOT be overwritten.

#### Scenario: Project link survives import

- **WHEN** a structured résumé lists a portfolio project with a name and a non-empty URL
- **THEN** the banked project employment stores that URL

#### Scenario: Missing link stays empty

- **WHEN** a structured résumé lists a portfolio project with no URL
- **THEN** the banked project employment is created without a URL and no fabricated link is stored

#### Scenario: A user-edited link is not overwritten on re-import

- **WHEN** a banked project already has a non-empty URL and a later résumé extract carries a different URL for the matched project
- **THEN** the banked URL is unchanged

### Requirement: Banked projects are project-shaped for CV seed

A projection used to compose a CV seed SHALL expose banked `project`-kind employments as portfolio projects (name, optional URL, publishable-atom highlights), not as job rows in the work-history list. Job-kind employments remain the work-history source. An atom that is not publishable (`agent_inferred`) MUST NOT appear as a project highlight. The fit-analysis professional projection MAY continue to flatten places into a single work-history shape; this requirement binds the CV-seed composition path.

#### Scenario: A banked project carries name, link, and publishable bullets

- **WHEN** the bank holds a project employment with a URL and publishable atoms
- **THEN** the CV-seed project projection includes that name, URL, and those claims as highlights

#### Scenario: Unpublishable atoms stay off the project

- **WHEN** a banked project has only `agent_inferred` atoms
- **THEN** the CV-seed project projection includes the project identity but no highlights from those atoms

#### Scenario: Jobs are not emitted as projects

- **WHEN** the bank holds only job-kind employments
- **THEN** the CV-seed project projection is empty
