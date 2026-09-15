## ADDED Requirements

### Requirement: HERP is a registered provider

The system SHALL register a `herp` adapter so a HERP-hosted careers catalogue
(`herp.careers/v1/<board>`) can be crawled by board id. The company's listing page SHALL be
fetched as HTML; the adapter SHALL collect every link matching `/v1/<board>/<jobID>` (a single
path segment after the board, excluding `/apply` and the platform's own reserved words — `top`,
its optional distinct landing page, linked from every listing and job page of a board that has
one) as a job, and every link matching
`/v1/<board>/requisition-groups/<uuid>` as a requisition group, whose OWN page SHALL be fetched
the same way and its job links added to the same set (one level of expansion; no further
nesting or pagination is expected). A link SHALL be matched by its resolved host and path
shape, never by a substring test, so an unrelated host embedding the job URL only inside its
own query string (a share-widget link) is never read as a job or a group. Each collected job
link SHALL be fetched and its `application/ld+json` `JobPosting` block decoded for `title`,
`description`, `datePosted`, and `jobLocation.address`; the adapter SHALL yield the normalized
job shape with `external_id` set to the link's final path segment. A failure fetching the
company page or any requisition-group page SHALL fail the whole `Fetch` call (this adapter
guarantees a full-or-none listing); a failure fetching one job's detail SHALL mark only that
posting Unreadable, leaving every other job unaffected.

#### Scenario: Direct job links are collected

- **WHEN** a board's listing page links directly to `/v1/<board>/<jobID>`
- **THEN** that job is fetched and yielded, without requiring a requisition group

#### Scenario: A requisition group is expanded one level

- **WHEN** a board's listing page links to `/v1/<board>/requisition-groups/<uuid>`
- **THEN** that group's own page is fetched and every job link found there is added to the
  same job set as the board's direct links

#### Scenario: The platform's own "top" landing-page link is not mistaken for a job

- **WHEN** a board's listing or job page carries a header link back to its own `/v1/<board>/top`
  landing page (a real, live shape on a subset of HERP boards)
- **THEN** that link is not collected as a job, since its detail page carries no `JobPosting`
  block and would otherwise be marked Unreadable on every crawl, permanently withholding that
  board's stale-job close

#### Scenario: A share-widget link is not mistaken for a job or a group

- **WHEN** a listing or group page contains a share link whose OWN host is not `herp.careers`
  but whose query string happens to embed a `herp.careers/v1/<board>/...` URL (e.g. a Twitter
  share button)
- **THEN** that link is not collected as a job or a group

#### Scenario: Job detail comes from the page's JobPosting ld+json block

- **WHEN** a collected job link is fetched
- **THEN** the adapter reads its `application/ld+json` `JobPosting` block and yields a job
  whose title, description, posted date, and location come from that block, and whose
  `external_id` is the link's final path segment

#### Scenario: A failed listing or group fetch fails the whole board

- **WHEN** the board's own listing page, or any requisition-group page it links to, fails to
  fetch
- **THEN** `Fetch` returns an error rather than yielding a partial job set

#### Scenario: A failed job detail fetch marks only that posting unreadable

- **WHEN** one job's detail request fails (not a platform-stated 404/410) while the rest of
  the board's listing succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity, and
  every other job it found is unaffected
