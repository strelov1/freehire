# source-ingest (delta)

## ADDED Requirements

### Requirement: Working Nomads is a registered boardless aggregator provider

The `workingnomads` provider SHALL crawl `workingnomads.com` through its public JSON feed
(`/api/exposed_jobs/`), a single flat array of postings with the body inline (no detail
call). Each posting maps to the normalized job shape with the employer taken from the
posting. The feed carries no numeric id field, so the adapter SHALL derive `ExternalID`
from the posting URL path (`/job/go/<id>/`); a posting whose URL yields no id SHALL be
dropped rather than persisted with an empty id.

#### Scenario: Working Nomads posting maps to a job with id from its URL

- **WHEN** a `workingnomads` posting carries URL `.../job/go/1663269/`, a title, a company name, an inline HTML description, and a `pub_date`
- **THEN** the job's `ExternalID` is `1663269`, its `Company` comes from the posting, and `Title`/`Description`/`PostedAt` come from that payload

#### Scenario: Posting with an unparseable URL is dropped

- **WHEN** a `workingnomads` posting has a URL with no `/job/go/<id>/` segment
- **THEN** the adapter drops it and does not emit a job with an empty `ExternalID`

### Requirement: Himalayas is a registered boardless aggregator provider

The `himalayas` provider SHALL crawl `himalayas.app` through its public JSON API
(`/jobs/api`), paginating by offset/limit over the API-reported `totalCount` up to a
defensive maximum-page cap. Each posting maps to the normalized job shape with the employer
taken from `companyName` and the canonical link from `applicationLink`.

#### Scenario: Himalayas is crawled across offset pages

- **WHEN** the `himalayas` provider crawls its boardless entry and the API reports a `totalCount` larger than one page
- **THEN** the adapter requests successive offset pages until the catalogue is exhausted or the defensive page cap is reached
- **AND** every posting becomes a job whose `Company` is its `companyName` and whose `ExternalID` is the posting's stable id (`guid`)

### Requirement: Remotive is a registered boardless aggregator provider

The `remotive` provider SHALL crawl `remotive.com` through its public JSON API
(`/api/remote-jobs`) with a **single** request per run — the API is rate-limited and serves
24h-delayed data, so the adapter SHALL NOT paginate or poll. Each posting maps to the
normalized job shape with the employer from `company_name`; Remotive lists only remote jobs,
so every job SHALL be marked remote.

#### Scenario: Remotive performs a single fetch and marks jobs remote

- **WHEN** the `remotive` provider crawls its boardless entry
- **THEN** the adapter makes exactly one request to the feed (no pagination loop)
- **AND** every returned posting becomes a remote job (`Remote` true, `WorkMode` "remote") with `Company` from `company_name` and `ExternalID` from the posting id

### Requirement: JustJoin is a registered boardless aggregator provider

The `justjoin` provider SHALL crawl `justjoin.it` through its public JSON API
(`api.justjoin.it/v2/user-panel/offers/by-cursor`), paginating by following `meta.next.cursor`
up to a defensive maximum-page cap. Each posting maps to the normalized job shape with the
employer from `companyName` and `WorkMode` derived from the structured `workplaceType`
(`remote`/`hybrid`/`office`). The list response carries no apply link, so the adapter SHALL
synthesize the canonical `URL` from the posting `slug` as `https://justjoin.it/job-offer/<slug>`.

#### Scenario: JustJoin posting maps to a job with synthesized URL and structured work mode

- **WHEN** a `justjoin` posting has slug `acme-senior-go`, `companyName` "Acme", and `workplaceType` "remote"
- **THEN** the job's `URL` is `https://justjoin.it/job-offer/acme-senior-go`, its `Company` is "Acme", and its `WorkMode` is "remote"

#### Scenario: JustJoin is crawled across cursor pages

- **WHEN** the API response includes a non-null `meta.next.cursor`
- **THEN** the adapter requests the next page with that cursor until the cursor is absent or the defensive page cap is reached
