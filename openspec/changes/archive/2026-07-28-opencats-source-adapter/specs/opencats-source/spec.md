## ADDED Requirements

### Requirement: Self-hosted OpenCATS portal crawl

The system SHALL crawl a self-hosted OpenCATS career portal over HTTPS, reading the portal's
"show all" listing for one configured board and returning one `Job` per open posting. The
adapter SHALL be registered under provider key `opencats` in the source registry. The adapter
SHALL be read-only: it never submits an application or registers a candidate.

#### Scenario: Listing yields one job per posting

- **WHEN** the adapter fetches a board whose portal listing links several open postings
- **THEN** it requests `https://<board>/index.php?m=careers&p=showAll` and returns one `Job`
  per posting, each carrying the posting's native numeric id as `ExternalID`

#### Scenario: An unreachable listing fails the board

- **WHEN** the portal listing cannot be fetched
- **THEN** the adapter returns an error for that board, so the run counts it as a board
  failure rather than an empty success

### Requirement: Board identity is host plus optional path prefix

The board SHALL be the portal root: a host, optionally followed by a path prefix, because
installs differ in where the portal is mounted. The adapter SHALL build every portal URL by
appending to that root, so both a root-mounted portal (`atscareers.g4s.com`) and a nested one
(`careers.boomit.pt/careers`) are addressable without a per-install code path.

#### Scenario: Root-mounted portal

- **WHEN** the board is `atscareers.g4s.com`
- **THEN** the listing is fetched from `https://atscareers.g4s.com/index.php?m=careers&p=showAll`

#### Scenario: Portal nested under a path prefix

- **WHEN** the board is `careers.boomit.pt/careers`
- **THEN** the listing is fetched from
  `https://careers.boomit.pt/careers/index.php?m=careers&p=showAll`

### Requirement: Postings are identified by routing, not by markup

Installs customise the portal template freely — CSS classes, column order, and column count
differ between them — so the adapter SHALL identify postings by the portal's routing
invariants alone. A posting SHALL be recognised by a link of the form
`index.php?m=careers&p=showJob&ID=<n>`, whose captured `<n>` is the posting's `ExternalID` and
whose anchor text is the job title. The adapter SHALL NOT depend on CSS classes, on the
position of a listing column, or on the number of listing columns.

#### Scenario: A rewritten template is parsed identically

- **WHEN** two boards serve listings with different markup, column counts, and CSS classes,
  but both link postings as `index.php?m=careers&p=showJob&ID=<n>`
- **THEN** both yield the same set of postings, with ids and titles taken from those links

#### Scenario: Duplicate links collapse to one posting

- **WHEN** a listing links the same posting id more than once (for example a title link and a
  separate "apply" link)
- **THEN** the adapter returns that posting once

### Requirement: Non-posting portal links are excluded

A portal listing SHALL contribute only real postings. Links that use the posting route but do
not represent an open position — notably a general-application entry such as "Can't find what
you're looking for? Apply here" — SHALL NOT be returned as jobs.

#### Scenario: General-application entry is skipped

- **WHEN** a listing contains a general-application link alongside real postings
- **THEN** only the real postings are returned

### Requirement: Location and description come from the detail page

Listing rows are positional and differ per install, so the adapter SHALL read each posting's
location and description from its detail page rather than from the listing row. Description
HTML SHALL pass through the shared sanitiser before it is stored. Detail pages SHALL be
fetched concurrently through the shared detail-fetch helper.

#### Scenario: Detail supplies location and sanitised description

- **WHEN** a posting's detail page is fetched
- **THEN** the returned `Job` carries the location and the description read from that page,
  with any embedded script stripped by the sanitiser

#### Scenario: A failed detail page skips only that posting

- **WHEN** one posting's detail page cannot be fetched or carries no usable id
- **THEN** that posting is skipped and every other posting on the board is still returned
