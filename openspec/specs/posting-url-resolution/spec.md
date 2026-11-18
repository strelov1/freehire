# posting-url-resolution Specification

## Purpose
TBD - created by archiving change find-job-by-page-url. Update Purpose after archive.
## Requirements
### Requirement: A page URL resolves to the catalog posting stored under that URL

The system SHALL resolve the URL of a job page to a catalog posting when no source
identity can be recovered from the URL, by comparing the URL to `jobs.url` in a normalized
form. Only open postings (`closed_at IS NULL`) are eligible. A matched duplicate SHALL be
answered with the posting it duplicates while that posting is open, and with its own slug
otherwise. When several eligible postings share a normalized URL, the system SHALL prefer
a canonical one, then the most recently seen (`last_seen_at DESC`), then `id DESC`, and
SHALL answer with exactly one posting.

The normalized form of a URL is: lowercased, with the `http://` or `https://` scheme and a
leading `www.` removed, with the fragment removed, with trailing slashes removed from the
path, and with the query string reduced to its non-tracking parameters sorted
lexicographically (the `?` dropped when none remain). A tracking parameter is one whose
name starts with `utm_` or is in the tracking vocabulary shared with link import
(`gclid`, `fbclid`, `ref`, `source`, …). The same normalization SHALL be applied to the
stored URL and to the requested URL.

#### Scenario: An aggregator posting is resolved by its page URL

- **WHEN** `/api/v1/jobs/find` is called with the URL of a page that an open canonical
  posting stores as its `url`, and no source identity can be recovered from that URL
- **THEN** the response carries that posting's `public_slug`

#### Scenario: Tracking parameters do not prevent resolution

- **WHEN** the requested URL differs from the stored one only by tracking parameters,
  the order of its query parameters, fragment, `www.` prefix, scheme, letter case, or
  trailing slashes
- **THEN** the posting is resolved as if the URLs were identical

#### Scenario: A query that names a different posting is not resolved

- **WHEN** the requested URL shares its host and path with a stored posting's URL but its
  non-tracking query parameters differ (another posting id, a listing page, or no query)
- **THEN** the response is `{"data": null}`

#### Scenario: A closed or duplicate posting is not resolved

- **WHEN** the only postings matching the normalized URL are closed (`closed_at` set)
- **THEN** the response is `{"data": null}`
- **WHEN** the only open posting matching is a duplicate (`duplicate_of` set) of an open
  posting
- **THEN** the duplicate itself is not answered: the response carries the duplicated
  posting's `public_slug` (or the duplicate's own when the duplicated posting is closed)

#### Scenario: An unknown page stays unresolved

- **WHEN** no posting stores the requested URL and no source identity can be recovered
  from it
- **THEN** the response is `{"data": null}`

### Requirement: Source identity resolution takes precedence over the URL lookup

The system SHALL first attempt to recover the catalog identity `(source, external_id)`
from the requested URL and load the posting by that identity. The URL comparison SHALL be
attempted only when no identity is recovered. A posting found by identity SHALL be
answered with regardless of whether its stored `url` matches the requested URL.

#### Scenario: A recognised ATS URL is answered from its identity

- **WHEN** the requested URL is one an identity parser recognises, and a posting exists
  under that identity
- **THEN** the response carries that posting's `public_slug`, without consulting `jobs.url`

#### Scenario: A recognised URL with no catalog posting falls through

- **WHEN** the requested URL yields an identity but no posting exists under it
- **THEN** the URL comparison runs, and the response is `{"data": null}` unless a posting
  stores that URL

