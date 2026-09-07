## Purpose

Gives homepage visitors live, ongoing proof that the catalogue is actively
growing, by surfacing recently ingested jobs as a real-time feed rather than
a static, point-in-time count.

## ADDED Requirements

### Requirement: Feed Eligibility
Only canonical, non-duplicate, IT/tech job postings ingested through the
standard crawl pipeline SHALL be eligible to appear in the recent jobs feed.

#### Scenario: Duplicate posting is ingested
- **WHEN** a newly ingested posting is identified as a duplicate or repost of
  an existing posting
- **THEN** it does not appear in the recent jobs feed

#### Scenario: Non-IT posting is ingested
- **WHEN** a newly ingested posting is classified as not an IT/tech role
- **THEN** it does not appear in the recent jobs feed

#### Scenario: Posting from an out-of-scope source is ingested
- **WHEN** a posting is ingested through a source not covered by this feed
  (for example, Telegram-extracted vacancies)
- **THEN** it does not appear in the recent jobs feed

### Requirement: Role-Based Aggregation
When multiple eligible postings for the same role become available in the
same delivery cycle, the feed SHALL present them as a single aggregated
entry once their count reaches a defined threshold, instead of one entry per
posting.

#### Scenario: A handful of postings for the same role arrive together
- **WHEN** eligible postings for the same role arrive in the same delivery
  cycle in a quantity below the aggregation threshold
- **THEN** the feed presents each posting as its own individual entry

#### Scenario: A large burst of postings for the same role arrives together
- **WHEN** eligible postings for the same role arrive in the same delivery
  cycle in a quantity at or above the aggregation threshold
- **THEN** the feed presents them as one aggregated entry naming the role and
  the number of postings it represents, instead of one entry per posting

### Requirement: Live Delivery With Backlog on Connect
The feed SHALL be delivered to the homepage as a live stream that updates
without a page reload, and a client that connects SHALL immediately receive
the most recently produced entries rather than starting with an empty feed.

#### Scenario: Visitor loads the homepage
- **WHEN** a visitor's browser opens a connection to the recent jobs feed
- **THEN** it immediately receives the most recent entries already produced,
  if any exist

#### Scenario: New eligible posting arrives while a visitor is connected
- **WHEN** an eligible posting (or aggregated group) is produced while a
  visitor's browser is connected to the feed
- **THEN** the visitor's feed updates with the new entry without the visitor
  reloading the page

### Requirement: Entry Presentation
Every feed entry SHALL display the role title and a company logo (or a
placeholder when the company has none), and an aggregated entry SHALL make
clear that it represents postings from more than one company rather than
implying they all belong to the single company shown.

#### Scenario: Single-posting entry for a company without a logo
- **WHEN** a feed entry represents one posting from a company that has no
  logo on file
- **THEN** the entry displays a placeholder in place of the logo, and still
  displays the role title and company name

#### Scenario: Aggregated entry is displayed
- **WHEN** a feed entry represents an aggregated group of postings for the
  same role from multiple companies
- **THEN** the entry names the role, states how many postings it represents,
  and does not present the one company logo shown as representative of all
  of them

### Requirement: Public, Unauthenticated Access
The recent jobs feed SHALL be viewable by anonymous homepage visitors without
requiring sign-in.

#### Scenario: Anonymous visitor views the homepage
- **WHEN** a visitor who is not signed in loads the homepage
- **THEN** the recent jobs feed is visible and updates live for them

### Requirement: No Misleading Empty State
When there are no entries yet to show, the feed SHALL render nothing rather
than an empty placeholder that could be mistaken for a broken feature.

#### Scenario: No entries have been produced yet
- **WHEN** a visitor connects to the feed before any eligible entry has been
  produced
- **THEN** the homepage shows no recent-jobs section, rather than an empty or
  loading placeholder
