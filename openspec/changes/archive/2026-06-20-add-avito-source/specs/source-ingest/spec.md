## ADDED Requirements

### Requirement: avito is a registered single-company boardless provider

The system SHALL register an `avito` adapter so Avito's career site (`career.avito.com`) can be
listed in a board file as a single-company boardless source (its config entry carries no `board`,
and the provider remains in the source facet). The adapter SHALL enumerate every vacancy from the
public, keyless sitemap index `https://career.avito.com/sitemap.xml`: it SHALL fetch the index,
fetch each referenced sub-sitemap, and keep every `<loc>` that matches a vacancy-detail URL
(`/vacancies/<category>/<id>/`, a numeric trailing id), deduplicating by that numeric id. Each
vacancy's posting SHALL come from its own detail-page fetch, fanned out under the shared
bounded-concurrency detail pool. The adapter SHALL yield the normalized job shape with
`external_id` set to the numeric vacancy id from the URL, `url` set to the vacancy page URL,
`company` set to the configured company, `title`/`description`/`posted_at` parsed from the page's
`JobPosting` ld+json, and `description` HTML-sanitized.

#### Scenario: The board is enumerated from the sitemap index and vacancy pattern

- **WHEN** a board file lists a boardless entry with provider `avito`
- **THEN** the adapter fetches `https://career.avito.com/sitemap.xml`, fetches each sub-sitemap,
  keeps only `<loc>` values matching `/vacancies/<category>/<id>/`, deduplicates by the numeric id,
  and yields each vacancy as the normalized job shape with `external_id` set to that id and `url`
  set to the vacancy page URL

#### Scenario: A loc without a parseable vacancy id is skipped

- **WHEN** a sitemap `<loc>` is a category, team, or landing page with no trailing numeric vacancy
  id
- **THEN** the adapter does not treat it as a vacancy and does not fetch it as a detail page

#### Scenario: A failed sitemap fetch is a board error; a failed detail skips just that vacancy

- **WHEN** the sitemap-index request fails
- **THEN** the adapter returns an error for the board
- **WHEN** a single vacancy's detail-page request fails or the page carries no `JobPosting` ld+json
- **THEN** the adapter skips that one vacancy and still yields the rest

### Requirement: avito derives location and remote from the page title, not the JSON-LD city

The adapter SHALL derive the display location from the page `<title>` suffix (`… в городе <city>`),
falling back to the JSON-LD `addressLocality` when that suffix is absent, because Avito's
`JobPosting.jobLocation.addressLocality` reports the company HQ city (`Москва`) even for remote
roles. The adapter SHALL mark a vacancy remote when the derived location or the title matches a
remote signal (the existing `isRemote` check, which matches the Russian stem "удал"), so "Удалённая
работа" roles are classified remote rather than as on-site Moscow roles.

#### Scenario: A remote role is classified remote despite a Moscow JSON-LD city

- **WHEN** a vacancy page's `<title>` ends with "в городе Удалённая работа" while its `JobPosting`
  ld+json reports `addressLocality` "Москва"
- **THEN** the adapter sets the job location to "Удалённая работа" and marks the job remote

#### Scenario: An on-site role keeps its city

- **WHEN** a vacancy page's `<title>` ends with "в городе Москва"
- **THEN** the adapter sets the job location to "Москва" and does not mark the job remote
