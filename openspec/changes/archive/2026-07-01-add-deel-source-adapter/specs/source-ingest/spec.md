## ADDED Requirements

### Requirement: deel is a registered provider

The system SHALL register a `deel` adapter so Deel ATS career pages
(`jobs.deel.com/<orgSlug>`) can be listed in a board file. The adapter SHALL treat the
configured `board` value as the org's URL slug and enumerate that org's postings from a single
request, `GET https://jobs.deel.com/<board>`, by parsing the Next.js flight payload embedded in
the page (the concatenated, JS-string-decoded bodies of the page's `self.__next_f.push` chunks).
The adapter SHALL read the postings from the payload's `jobPostings` array and SHALL resolve
each posting's `richtextDescription` `"$N"` reference to the corresponding text row
(`N:T<byte-length>,<html>`) in the same payload, yielding it as the job `description` after HTML
sanitization. The adapter SHALL NOT issue a per-posting detail request. The adapter SHALL yield
the normalized job shape (at least title, url, description, and the platform's native posting
id), with `external_id` set to the posting `id` (the UUID used in the public job URL) and `url`
set to `https://jobs.deel.com/<board>/job-details/<id>/overview`. The job `company` SHALL come
from the payload's `careerPageSettings.preferredOrganizationName` when present, falling back to
the configured company name. The job `location` SHALL come from the posting's job-location names
when present and MAY be empty otherwise. Because Deel exposes no structured workplace-type field,
the adapter SHALL leave the structured work mode empty and infer the remote flag from the shared
location heuristic.

#### Scenario: Deel board is enumerated from the embedded flight payload

- **WHEN** a board file lists an entry with provider `deel` and an org URL slug as the board
- **THEN** the adapter requests `https://jobs.deel.com/<slug>` once and yields each posting from
  the page's `jobPostings` payload as the normalized job shape, with `external_id` set to the
  posting `id` and `url` set to `https://jobs.deel.com/<slug>/job-details/<id>/overview`, issuing
  no per-posting detail request

#### Scenario: Description is resolved from a `$N` reference and sanitized

- **WHEN** a posting's `richtextDescription` is a `"$N"` reference to a `N:T<byte-length>,<html>`
  text row in the same payload
- **THEN** the adapter resolves the reference to that row's HTML (sliced by its declared byte
  length) and yields it as the job `description`, sanitized (active content stripped, structure
  such as paragraphs and lists kept) and decoded as correct UTF-8

#### Scenario: Company name comes from the career-page settings

- **WHEN** the payload's `careerPageSettings.preferredOrganizationName` is present
- **THEN** the yielded jobs use it as the company name; when it is absent the adapter falls back
  to the configured company name

#### Scenario: An empty or jobless board yields no jobs without error

- **WHEN** a board's page carries an empty `jobPostings` payload
- **THEN** the adapter yields zero jobs and returns no error, so the board is simply skipped

#### Scenario: A malformed payload is an error, not silent bad data

- **WHEN** the page does not contain a decodable `jobPostings` payload
- **THEN** the adapter returns an error for that board rather than yielding zero jobs silently,
  so a markup change surfaces instead of quietly emptying the catalogue
