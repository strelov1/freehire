## Purpose

Crawls one werecruit employer board (a locale-scoped tenant career site) through the platform's
own embedded listing data, hydrates each posting's description, and recognizes the platform's
public URL shape so a pasted or harvested apply link can be onboarded as a board.

## ADDED Requirements

### Requirement: Locale-scoped listing crawl

The system SHALL crawl a werecruit board by requesting the tenant's listing page under the
board's configured locale, and SHALL return one `Job` per posting the page's embedded listing
data carries. The adapter SHALL be registered under provider key `werecruit`.

#### Scenario: Fetch returns the board's postings

- **WHEN** the adapter fetches a board whose locale is one the tenant is actually configured for
- **THEN** it returns one `Job` per posting the tenant's listing page embeds

#### Scenario: An unconfigured locale returns no postings

- **WHEN** the board's locale is one the tenant is NOT configured to publish in
- **THEN** the adapter returns an empty result, not an error — the platform itself answers an
  empty listing rather than a partial or error response

### Requirement: Locale is part of the board identity

The board SHALL be `<locale>/<tenant>`, both segments required, matching the platform's own
public URL shape. The locale SHALL NOT be dropped or folded, because a tenant not configured for
a given locale answers it with an empty listing rather than the tenant's full catalogue.

#### Scenario: Well-formed board is accepted

- **WHEN** a board is `"en/idiap"`
- **THEN** the adapter requests the tenant's listing under the `en` locale segment

#### Scenario: Malformed board is rejected

- **WHEN** a board is missing the locale, the tenant, or both
- **THEN** the adapter returns an error identifying the expected `<locale>/<tenant>` form, and
  issues no request

### Requirement: Posting normalization with hydrated description

The adapter SHALL map each listed posting onto the catalogue's `Job` shape using the listing's
own fields (native id, title, location, post date), and SHALL fetch each posting's own page to
obtain its description body, since the listing does not carry it. The job's company SHALL be the
board's configured company.

#### Scenario: A posting maps to a Job with a hydrated description

- **WHEN** a posting is listed with a native id, a title, a place, and a post date, and its own
  page carries a description
- **THEN** the resulting `Job` carries that id as `ExternalID`, the title, a location built from
  the place, the post date as `PostedAt`, the sanitized description from the posting's own page,
  and the board's configured company as `Company`

### Requirement: Public URL recognition

The system SHALL recognize `careers.werecruit.io/<locale>/<tenant>/…` as a werecruit board URL,
resolving it to provider `werecruit` and board `<locale>/<tenant>`.

#### Scenario: A posting link resolves to its board

- **WHEN** a URL is `https://careers.werecruit.io/en/idiap/offers/some-role-abc123`
- **THEN** it resolves to provider `werecruit`, board `"en/idiap"`

#### Scenario: A URL with no tenant segment is not recognized

- **WHEN** a URL on the host carries only a locale segment or no path at all
- **THEN** it is not recognized as a board — no false board is produced
