## ADDED Requirements

### Requirement: vouch.careers company crawl

The system SHALL provide a `vouch` source adapter that crawls one vouch.careers
company page into the catalogue. The board id is the URL company cuid, and the
adapter fetches `https://vouch.careers/companies/<board>`, decodes the Next.js RSC
flight, and extracts the inlined `listings` array. The crawl is keyless. The adapter
is board-based (it requires a board id) and appears in the source facet.

#### Scenario: Company page yields all live postings

- **WHEN** the adapter fetches a configured company board
- **THEN** it returns one `Job` per **live** listing (activated, not draft, not
  unlisted) in the page's `listings` array, with the posting's title, HTML
  description composed from its body fields, free-text location, apply/detail URL,
  work mode, and posted-at timestamp — all read from the single company page, with
  no per-posting detail fetch

#### Scenario: Stable dedup identity

- **WHEN** the adapter maps a listing to a `Job`
- **THEN** the `ExternalID` is the listing's stable vouch id (cuid), so re-crawling
  the same company dedups to the same catalogue row; a listing without an id is
  dropped rather than persisted with an empty key

#### Scenario: Company name falls back to the configured entry

- **WHEN** a listing carries a `company.name`
- **THEN** it is used as the job's company, otherwise the configured board company
  is used

### Requirement: Excludes non-live listings

The `listings` array can include draft, deactivated, or unlisted postings, so the
adapter SHALL emit only postings that are `activated` and neither `draft` nor
`unlisted` — a draft or hidden posting is not a live vacancy.

#### Scenario: Draft or unlisted posting skipped

- **WHEN** a listing is not activated, or is marked draft or unlisted
- **THEN** the adapter omits it from the returned jobs

### Requirement: Structured work mode from employment type

The listing's `employmentType` is a STRUCTURED set of tokens including
"Remote"/"Hybrid"/"On-site", so the adapter SHALL map it to the work-mode
vocabulary rather than inferring the arrangement from free text.

#### Scenario: Remote token maps to remote work mode

- **WHEN** a listing's `employmentType` contains "Remote"
- **THEN** the job's work mode is "remote"

### Requirement: Resilient flight parsing

The postings live in the Next.js RSC flight, so the adapter SHALL fail with an error
when the flight or its `listings` array is absent (a markup change must surface
loudly rather than silently empty the catalogue), while an empty `listings` array is
valid and yields no jobs.

#### Scenario: Missing flight payload errors

- **WHEN** the company page carries no flight payload or no `listings` array
- **THEN** `Fetch` returns an error rather than an empty success
