# regional-ats-providers Specification

## Purpose

Smaller regional ATS providers (humanbit, selfrecruit, scalis, recrutei, pyjamahr, recruiterflow, staffy). Split out of source-ingest, which holds the rules every adapter shares.

## Requirements

### Requirement: humanbit is a registered provider

The system SHALL register a `humanbit` adapter so a HumanBit tenant's public job board
(`jobs.humanbit.ai/<board>`) can be crawled by board id. The board id SHALL be the tenant's
path segment. The listing page SHALL be fetched once per crawl to enumerate every open
posting (id, title, and the platform's own company display name), and each posting's own
detail page SHALL then be fetched to complete the structured fields (employment type,
remote flag, skills) the listing does not carry. Each posting's HTML description — a
`"$<id>"` reference into a flight's text rows — SHALL be resolved before being sanitized
into the job's description. A failure fetching one posting's detail SHALL mark only that
posting Unreadable, leaving every other posting unaffected; a listing fetch or decode
failure SHALL fail the whole `Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, structured employment type/remote flag/skills where
  the detail states them, and the company name the listing carries

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing page fails to fetch or its flight cannot be decoded
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: A description reference resolves to the flight's text row

- **WHEN** a posting's `description` field is a `"$<id>"` reference
- **THEN** the yielded job's description is the sanitized HTML of that id's text row, not
  the literal reference string

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

### Requirement: scalis is a registered provider

The system SHALL register a `scalis` adapter so a Scalis tenant's public job listing
(`<board>.scalis.ai`) can be crawled by board id. The board id SHALL be the tenant
subdomain. The listing SHALL be paged via `https://<board>.scalis.ai/jobs?page=N&limit=10&sortBy=SORT_BEST_MATCH`,
walked to exhaustion (a page whose result list is empty ends the walk; a page-fetch or
decode failure at any point fails the whole `Fetch`, never a silently truncated listing).
Each page's RSC-flight payload SHALL be decoded via the existing shared primitives, and
each posting's HTML description — a `"$<id>"` reference into the flight's text rows —
SHALL be resolved before being sanitized into the job's description. The adapter SHALL
yield the normalized job shape with `external_id` set to the posting's native id and
structured `employment_type`, `work_mode`, `skills`, and salary bounds populated from the
platform's own enums/fields wherever it states them.

#### Scenario: A single-page board is crawled

- **WHEN** a tenant's listing has 10 or fewer open postings
- **THEN** every posting on the first page is yielded, and the walk stops after that page
  (its next page's result list is empty)

#### Scenario: A multi-page board is crawled to exhaustion

- **WHEN** a tenant's listing spans multiple pages
- **THEN** the adapter walks pages in order and yields the union of every page's postings

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's first listing page has an empty result list
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A failed listing page fetch fails the whole board

- **WHEN** any page's fetch or flight decode fails, including a page after the first
- **THEN** `Fetch` returns an error rather than yielding a partial job set

#### Scenario: A description reference resolves to the flight's text row

- **WHEN** a posting's `descriptionHtml` field is a `"$<id>"` reference
- **THEN** the yielded job's description is the sanitized HTML of that id's text row, not
  the literal reference string

#### Scenario: Structured employment, work mode, skills, and salary are mapped

- **WHEN** a posting states a recognized `employment`/`workplace` enum value, a skills
  list, and/or a non-null salary bound
- **THEN** the yielded job carries the mapped `employment_type`/`work_mode`, the skills
  list, and the salary bounds with their stated currency

### Requirement: recrutei is a registered provider

The system SHALL register a `recrutei` adapter so a Recrutei tenant's public job board
(`jobs.recrutei.com.br/<board>`) can be crawled by board id. The board id SHALL be the
tenant's path segment. The listing SHALL be fetched once per crawl via one POST to the
tenant's `per-departments` endpoint to enumerate every open posting (id, title, company
name, location, and employment regime), and the sum of every department's item count SHALL
equal the response's declared total or the whole `Fetch` fails. Each posting's own detail
page SHALL then be fetched to complete its description and post date via the page's
schema.org ld+json block. A failure fetching one posting's detail SHALL mark only that
posting Unreadable, leaving every other posting unaffected; a listing fetch, decode, or
completeness-check failure SHALL fail the whole `Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, an employment type derived from the listing's
  Brazilian labor-regime field, the listing's own location, and the listing's company name

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing POST fails to fetch or its response cannot be decoded
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A listing whose declared total disagrees with its item count fails the whole board

- **WHEN** the response's `data.total` does not equal the sum of every department's item
  count
- **THEN** `Fetch` returns an error rather than yielding an unproven partial job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: A Brazilian labor regime maps to an employment type

- **WHEN** a listing item's `regime` is `CLT` or `Pessoa Jurídica`
- **THEN** the yielded job's employment type is `full_time` or `contract` respectively, and
  an ambiguous or unstated regime (`CLT ou PJ`, `Não informado`, or any other value) yields
  an empty employment type rather than a guess

#### Scenario: A listing item's location tolerates either a string array or a placeholder string

- **WHEN** a listing item's `location` field is a JSON array of strings
- **THEN** the yielded job's location is those parts joined
- **WHEN** a listing item's `location` field is instead the bare string `"Não informado"`
  ("not stated")
- **THEN** the yielded job's location is empty rather than carrying that literal placeholder
  text, and the whole board's `Fetch` still succeeds rather than failing on the type
  mismatch

### Requirement: pyjamahr is a registered provider

The system SHALL register a `pyjamahr` adapter so a PyjamaHR tenant's public job board
(`jobs.pyjamahr.com/<board>`) can be crawled by board id. The board id SHALL be the
tenant's path segment. The listing SHALL be fetched page by page via the response's own
`next` URL until it is null, and reaching a page-count safety ceiling while `next` is still
non-empty SHALL fail the whole `Fetch` rather than silently stopping. Each posting's own
detail page SHALL then be fetched to complete its description, employment type, work mode,
salary (only when the platform marks it visible), and skills. A failure fetching one
posting's detail SHALL mark only that posting Unreadable, leaving every other posting
unaffected; a listing fetch, decode, or safety-ceiling failure SHALL fail the whole
`Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, an employment type derived from the detail's job
  type, a work mode derived from the detail's workplace type or remote flag, and the
  configured company name

#### Scenario: The listing pages to exhaustion via its own next link

- **WHEN** a tenant's listing spans multiple pages
- **THEN** the adapter walks pages via the response's `next` URL and yields the union of
  every page's postings, stopping when `next` is null

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** any page's fetch or decode fails, including a page after the first
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: Reaching the page safety ceiling with more pages left fails the whole board

- **WHEN** the page walk reaches its safety ceiling while the response still names a `next`
  page
- **THEN** `Fetch` returns an error rather than yielding an unproven partial job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: Salary maps only when the platform marks it visible

- **WHEN** a posting's detail states `is_salary_visible` true with both a minimum and
  maximum bound and a recognized salary period
- **THEN** the yielded job carries those bounds with their currency and period; any other
  combination (invisible, a missing bound, or an unrecognized period) yields no salary
  fields at all

### Requirement: recruiterflow is a registered provider

The system SHALL register a `recruiterflow` adapter so a RecruiterFlow agency board's
public listing (`recruiterflow.com/<board>/jobs`) can be crawled by board id. The board id
SHALL be the tenant's path segment. The listing SHALL be read once per crawl from the
page's own embedded `window.jobsList` JavaScript object (department-grouped, no separate
network call), which SHALL name every open posting's id, title, location, employment type,
remote type, and last-opened date. Each posting's own detail page SHALL then be fetched
for its description via the page's schema.org ld+json block. A failure fetching one
posting's detail SHALL mark only that posting Unreadable, leaving every other posting
unaffected; a listing fetch or decode failure SHALL fail the whole `Fetch`.

#### Scenario: A board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls a configured tenant
- **THEN** every posting the embedded listing names is fetched for its detail and yielded
  with a title, sanitized HTML description, an employment type and work mode derived from
  the listing's own fields, the listing's own location, and the configured company name

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** a tenant's embedded listing carries no postings
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing page fails to fetch or its embedded `window.jobsList` cannot be
  decoded
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: Employment type and remote type map from the listing's own vocabulary

- **WHEN** a listing item's `employment_type` is `Full time`, `Part time`, or `Contract`,
  and its `remote_type` is `Remote`, `Hybrid`, or absent
- **THEN** the yielded job's employment type and work mode reflect those values, with an
  absent remote type yielding an empty work mode rather than a guess

### Requirement: staffy is a registered provider

The system SHALL register a `staffy` adapter as a boardless single-company source over
`jobs.wearestaffy.com`. The listing SHALL be fetched once per crawl from the platform's
static `/vacantes` page, and the count of distinct posting links found SHALL be verified
against the page's own declared total; a mismatch SHALL fail the whole `Fetch`. Each
posting's own detail page SHALL then be fetched to complete its description, location,
work mode, and seniority from its own fixed metadata fields. A failure fetching one
posting's detail SHALL mark only that posting Unreadable, leaving every other posting
unaffected; a listing fetch, decode, or completeness-check failure SHALL fail the whole
`Fetch`.

#### Scenario: The board's open postings are enumerated and hydrated

- **WHEN** the adapter crawls
- **THEN** every posting the listing names is fetched for its detail and yielded with a
  title, sanitized HTML description, a seniority level derived from the detail page's own
  label, a work mode derived from the detail page's own work-arrangement text, and the
  configured company name

#### Scenario: An empty board yields no jobs, not an error

- **WHEN** the listing carries no postings and declares a total of zero
- **THEN** `Fetch` returns an empty result rather than failing

#### Scenario: A listing fetch or decode failure fails the whole board

- **WHEN** the listing page fails to fetch or carries no parseable posting links
- **THEN** `Fetch` returns an error rather than yielding a partial or empty job set

#### Scenario: A listing whose declared total disagrees with its link count fails the whole board

- **WHEN** the number of distinct posting links found does not equal the page's own
  declared total
- **THEN** `Fetch` returns an error rather than yielding an unproven partial job set

#### Scenario: A failed posting detail fetch marks only that posting unreadable

- **WHEN** one posting's detail request fails while the listing itself succeeded
- **THEN** the adapter yields that posting as an Unreadable marker carrying its identity,
  and every other posting is unaffected

#### Scenario: A detail page with no parseable metadata block marks only that posting unreadable

- **WHEN** a posting's detail page answers successfully but carries no metadata block at
  all
- **THEN** the adapter yields that posting as an Unreadable marker rather than a job with
  empty structured fields or a mis-bounded description

#### Scenario: Seniority maps from a closed, confirmed vocabulary

- **WHEN** a posting's detail states a seniority label of `Junior`, `Jr`, `Semi senior`,
  `Ssr`, `Senior`, `Sr`, or `Staff` (case-insensitive)
- **THEN** the yielded job's seniority is `junior`, `middle`, `senior`, or `staff`
  respectively; a compound or otherwise unrecognized label yields an empty seniority
  rather than a guess
