## ADDED Requirements

### Requirement: selfrecruit is a registered provider

The system SHALL register a `selfrecruit` adapter so a selfrecruit.ge tenant's public job
board (`<board>.selfrecruit.ge`) can be crawled by board id. The board id SHALL be the
tenant subdomain. The listing SHALL be paged via `https://<board>.selfrecruit.ge/vacancies/<offset>`
in steps of 10, walked to exhaustion (a page yielding no new posting link ends the walk;
a page-fetch failure at any point fails the whole `Fetch`, never a silently truncated
listing). The platform exposes no JSON API and no schema.org/ld+json markup, so each
collected posting link's detail page SHALL be fetched and its title and description
extracted from the page's own DOM structure. The adapter SHALL yield the normalized job
shape with `external_id` set to the detail link's UUID path segment. A failure fetching
one job's detail SHALL mark only that posting Unreadable, leaving every other job
unaffected.

#### Scenario: A single-page board is crawled

- **WHEN** a tenant's listing has 10 or fewer open postings
- **THEN** every posting link found on the first page is fetched and yielded, and the
  walk stops after that page (its next offset yields no new links)

#### Scenario: A multi-page board is crawled to exhaustion

- **WHEN** a tenant's listing spans multiple `/vacancies/<offset>` pages
- **THEN** the adapter walks pages in order and yields the union of every page's posting
  links

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing page carries no posting links
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A failed listing page fetch fails the whole board

- **WHEN** any page of the board's listing fails to fetch, including a page after the
  first
- **THEN** `Fetch` returns an error rather than yielding a partial job set

#### Scenario: Re-crawling the same tenant does not duplicate postings

- **WHEN** the same tenant is crawled twice with no change to its open postings
- **THEN** both crawls map every posting to the same `external_id`

#### Scenario: A failed job detail fetch marks only that posting unreadable

- **WHEN** one job's detail request fails while the rest of the board's listing succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other job it found is unaffected
