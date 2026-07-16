## ADDED Requirements

### Requirement: Djinni listing crawl over embedded JSON-LD

The system SHALL provide a `djinni` source adapter that crawls the anonymous listing
`https://djinni.co/jobs/?page=N`. Each listing page embeds a single
`<script type="application/ld+json">` block whose payload is a JSON array of `JobPosting`
objects carrying the full posting; the adapter SHALL parse that array and yield one `Job`
per `JobPosting` — no per-posting detail request is made, because the listing already
carries the description.

The adapter is **boardless** (djinni.co is one site with no per-tenant board) and an
**aggregator** (one crawl enumerates postings from many companies; it stays in the source
facet and takes each posting's company from the feed). The configured board file entry's
`board` value is not used to shape a URL.

#### Scenario: A listing page maps to jobs

- **WHEN** the adapter reads a listing page whose JSON-LD block is an array of `JobPosting`
  objects
- **THEN** it yields one `Job` per element carrying the posting's title, description (the
  `description` field, sanitized), company (from `hiringOrganization.name`), location/country
  (from `applicantLocationRequirements.address.addressCountry`), work mode (from
  `jobLocationType`), employment type (from `employmentType`), posted-at timestamp (from
  `datePosted`), the numeric `identifier` as the `ExternalID`, and the `url` as the canonical
  detail URL

#### Scenario: A page whose JSON-LD is a single object, not an array

- **WHEN** a listing page's JSON-LD block is a single `JobPosting` object rather than an array
- **THEN** the adapter still yields that one `Job` (it accepts both an array and a lone object)

### Requirement: Djinni pages the listing to the end of the feed

The adapter SHALL page the listing from `page=1` upward, fetching each page while FOLLOWING
redirects and observing the final URL. A past-the-end page 302-redirects to the bare listing
(`/jobs/`, which re-serves page 1 and is therefore NOT empty), so the adapter SHALL stop when
the resolved final URL no longer carries the requested `page=N` marker — the reliable
end-of-feed signal. A page that returns without a redirect but carries no `JobPosting` SHALL
also stop the crawl. Pagination SHALL be bounded by a page cap as a backstop against a
pathological feed, so a crawl can never loop unboundedly.

#### Scenario: Crawl stops when a past-the-end page redirects to the bare listing

- **WHEN** the adapter requests the page past the last populated one and the request redirects
  to `/jobs/` (whose content is page 1, not empty)
- **THEN** the adapter detects the missing page marker in the final URL, stops paging, and
  returns only the earlier pages' jobs — it does NOT re-ingest the redirected page-1 content

#### Scenario: Crawl stops on a non-redirected empty page

- **WHEN** a page returns without a redirect and carries no `JobPosting`
- **THEN** the adapter stops paging and returns the accumulated jobs

#### Scenario: Page cap bounds a pathological feed

- **WHEN** the listing never signals the end within the configured page cap
- **THEN** the adapter stops at the cap rather than paging without bound

### Requirement: Djinni survives a datacenter rate-limit mid-crawl

Djinni rate-limits a fast page burst from a datacenter IP with a `403`. The adapter SHALL pace
its sequential page requests to stay under that limit, and SHALL treat a page fetch that fails
partway through the crawl as a partial success: it keeps the postings already collected (the
freshest pages, since Djinni orders by recency) and stops, rather than discarding the whole
crawl. The adapter SHALL fail the board (return an error) ONLY when the FIRST page fails, so a
successful crawl never returns an empty set that would let the unseen-sweep close the catalogue.

#### Scenario: A mid-crawl 403 keeps the pages already collected

- **WHEN** page 1 maps successfully and a later page fails with a fetch error (e.g. a rate-limit
  `403`)
- **THEN** the adapter returns the jobs from the pages it did fetch, with no error, and does not
  request further pages

#### Scenario: A first-page failure fails the board

- **WHEN** the very first page fails with a fetch error (no jobs collected yet)
- **THEN** the adapter returns an error so the board is counted failed and cooled down — an
  empty successful crawl (which the unseen-sweep would act on) is never returned

### Requirement: Djinni drops a posting it cannot key or address

The adapter SHALL drop a `JobPosting` that lacks a usable `identifier` (no dedup key), a
usable `url` (no canonical address), or a company name (which would break the company slug),
rather than yielding an unusable `Job`. A single unusable posting SHALL NOT abort the page or
the crawl.

#### Scenario: Posting without id, url, or company is dropped

- **WHEN** a `JobPosting` in the array has no `identifier`, no `url`, or no
  `hiringOrganization.name`
- **THEN** the adapter drops that posting and continues mapping the rest of the page

### Requirement: Djinni postings inherit aggregator ATS suppression

The system SHALL register `djinni` as an aggregator (present in
`sources.AggregatorProviders()`) so that a Djinni posting duplicating a first-party ATS
posting is suppressed by the existing reindex suppression pass under its unchanged rules
(same `company_slug`, matching normalized title, compatible country; the ATS posting stays
canonical). The adapter itself SHALL NOT implement any cross-source dedup — it only supplies
the company, title, and country the suppression pass keys on.

#### Scenario: A Djinni copy of an ATS job is suppressed at reindex

- **WHEN** a company has an open first-party ATS posting and an open Djinni posting with the
  same normalized title and compatible country, and the reindex suppression pass runs
- **THEN** the Djinni posting is marked `duplicate_of` the ATS posting and the ATS posting
  stays canonical, exactly as for any other aggregator source

#### Scenario: A Djinni-exclusive posting is not suppressed

- **WHEN** a Djinni posting has no matching first-party ATS twin (a role posted only on
  Djinni, even for a company otherwise crawled via an ATS)
- **THEN** the posting is not suppressed and remains in search, embedding, and enrichment
