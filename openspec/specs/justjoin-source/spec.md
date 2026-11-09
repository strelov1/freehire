# justjoin-source Specification

## Purpose

How the `justjoin` (justjoin.it, Poland's IT board) and `rocketjobs` (rocketjobs.pl, the same
platform's board for every other trade) adapters crawl the Just Join IT platform. Both hosts'
robots.txt disallow the platform's JSON API, so the offer set comes from each site's
active-jobs sitemap and every fact from the offer page itself.

## Requirements

### Requirement: Just Join IT boards are boardless aggregator providers

The system SHALL provide two adapters on one implementation: `justjoin` for `justjoin.it`
(offer pages under `/job-offer/`) and `rocketjobs` for `rocketjobs.pl` (offer pages under
`/oferta-pracy/`). Each is boardless (one national board, no per-tenant id) and an aggregator
(it stays in the source facet and takes each offer's employer from the offer). Every
`justjoin` job SHALL carry `IsTechHint`, because every justjoin.it offer is filed under one
of its IT categories; `rocketjobs` jobs SHALL NOT.

#### Scenario: The IT board marks its jobs technical

- **WHEN** the `justjoin` adapter yields a job
- **THEN** the job carries `IsTechHint`, and a job from `rocketjobs` does not

### Requirement: The sitemap is the offer set

The adapter SHALL list offers from the site's active-jobs sitemap index
(`https://<host>/sitemaps/active-jobs.xml`) and every part it names, keeping only offer URLs
on the site's own host whose path is the offer prefix followed by a slug. The slug is the
offer's `ExternalID` (a multi-city offer is one sitemap entry under its parent slug) and the
canonical `URL` is `https://<host><offer prefix><slug>`. Duplicate slugs SHALL be listed once,
and offers SHALL be ordered by the sitemap's `lastmod`, most recent first.

The sitemap is the whole listing, so the crawl SHALL fail when the index or any part cannot
be read, when the index names no part or more than a defensive cap of parts, or when no offer
is found: a partial listing would let the sweep close the offers it did not read.

#### Scenario: A sitemap entry becomes an offer identity

- **WHEN** the justjoin.it sitemap names `https://justjoin.it/job-offer/acme-senior-go`
- **THEN** the offer's `ExternalID` is `acme-senior-go` and its `URL` is that address

#### Scenario: A foreign or malformed entry is skipped

- **WHEN** a sitemap entry is on another host or its path is not an offer slug
- **THEN** the adapter skips it

#### Scenario: An unreadable sitemap part fails the crawl

- **WHEN** one part of the sitemap index cannot be fetched
- **THEN** the crawl fails instead of yielding the offers of the other parts

### Requirement: Only unseen offers are read, within a per-run bound

The adapter SHALL receive a `seen(externalID)` predicate. For a seen offer it SHALL yield a
liveness-refresh job (`SeenRefresh`, no page request), so the pipeline refreshes the row's
last-seen and open state without rewriting its content. For unseen offers it SHALL read at
most a bounded number of pages a run, picked by `hydrationLanes`: the freshest offers plus a
backlog window that moves each hour, so offers the pipeline discards unsaved cannot starve
the older ones. Offers past the bound are deferred to later runs and logged.

Requests SHALL be paced (one a second across sitemap and offer pages) with bounded
concurrency, and page reads SHALL stop a margin before the run's deadline, leaving the
pipeline time to write what was read. An offer whose page fails SHALL be logged and skipped,
not abort the crawl; but a run that asked for pages and got no answer at all SHALL fail.

#### Scenario: An already-ingested offer is refreshed without a page request

- **WHEN** the sitemap names an offer whose slug is in the seen set
- **THEN** the adapter yields a liveness-refresh job for it and does not request its page

#### Scenario: One failed page does not abort the crawl

- **WHEN** the page of one unseen offer fails to load
- **THEN** the adapter logs it, skips that offer, and keeps reading the rest

#### Scenario: A run where no page answers fails

- **WHEN** the adapter requested offer pages and every one of them failed
- **THEN** the crawl returns an error

### Requirement: An offer page maps to a job

The adapter SHALL read the offer object from the page's server-rendered Next.js flight,
resolving a body given as a flight text reference. A page answering 404 or 410, or whose
offer says `isOfferActive` false (the sitemap names an expired offer for about a day), SHALL
count as a gone offer and yield nothing. A page that carries no offer object, or a body
reference the flight does not hold, SHALL be an error, not a gone offer. An offer with no
title, employer or body SHALL be dropped.

The job SHALL carry the title, the employer from `companyName`, the city, the country from
`countryCode` (Poland as fallback), `WorkMode` from `workplaceType`, the posted-at time from
`publishedAt`, and from the offer's structured fields: seniority from `experienceLevel`
(justjoin's `mid` meaning `middle`, unknown levels dropped), employment type from
`workingTime`, skills from `requiredSkills` canonicalized through the skill dictionary, and
the English level from `languages` when it is a CEFR level. The adapter SHALL NOT derive a
category from the platform's category, which is a stack tag rather than a role.

Pay is listed per contract type, once in its original currency and again converted. The
adapter SHALL read only the original-currency entries with a full range and a known period.
The structured salary SHALL take the employment contract's range when there is one, else the
first other contract type's, and the description SHALL begin with a "Wynagrodzenie" paragraph
stating every contract type's range with its gross or net basis.

#### Scenario: An offer page becomes a job

- **WHEN** an unseen offer's page carries an offer with title, `companyName` "Acme",
  `workplaceType` "remote" and an employment-contract salary
- **THEN** the job's `Company` is "Acme", its `WorkMode` is "remote", its salary is the
  employment contract's range, and its description starts with the pay paragraph

#### Scenario: An expired offer is gone

- **WHEN** an offer page answers 404 or its offer says `isOfferActive` false
- **THEN** the adapter yields no job for it and counts the page as answered

#### Scenario: A page without an offer object is an error

- **WHEN** an offer page answers but its flight carries no offer object
- **THEN** the adapter treats the page as failed, not as a gone offer
