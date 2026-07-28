## MODIFIED Requirements

### Requirement: Company-scoped removal requires retiring the board

The system SHALL apply the company-scoped rules — non-technical category at a company without technical evidence, and unknown at a company with no evidence at all — only to companies whose board entries are removed from the source board files in the same step. Boards are re-crawled hourly on the unchanged dedup key, so a company-scoped deletion that leaves the board in place is undone within one crawl cycle; the ingest-time rejection covers only the title rule and cannot substitute for board retirement.

The board-retirement report SHALL name a board only where its postings were classified and none of them resolved as technical. A board no posting of which carries an `is_tech` verdict MUST be withheld from the report rather than named, because `is_tech` is tri-state and the absence of a technical signal from an unclassified posting is not evidence about the board. The report MUST state how many boards it withheld on that ground, so a shorter list is not read as a complete one.

#### Scenario: Company-scoped deletion without board retirement is refused

- **WHEN** a run would apply a company-scoped rule to a company whose board is still listed in the source board files
- **THEN** the run reports those companies and does not delete their jobs

#### Scenario: Retirement candidates are reported

- **WHEN** the operator requests the board-retirement report
- **THEN** the system lists the board-file entries whose postings were classified and none resolved as technical, identified by the board the write path namespaces into `external_id`

#### Scenario: An unclassified board is withheld, not retired

- **WHEN** no posting of a listed board carries an `is_tech` verdict either way
- **THEN** the board is absent from the retirement list and counted among the boards withheld for want of a classification

#### Scenario: The report accounts for what it held back

- **WHEN** the report withholds one or more boards for want of a classification
- **THEN** it states how many, so the operator cannot mistake the remaining list for the whole of what the catalogue offers
