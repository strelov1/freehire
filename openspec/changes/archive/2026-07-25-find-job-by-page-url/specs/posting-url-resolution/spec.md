## ADDED Requirements

### Requirement: A page URL resolves to the catalog posting stored under that URL

The system SHALL resolve the URL of a job page to a catalog posting when no source
identity can be recovered from the URL, by comparing the URL to `jobs.url` in a normalized
form. Only open, canonical postings (`closed_at IS NULL` and `duplicate_of IS NULL`) are
eligible. When several eligible postings share a normalized URL, the system SHALL answer
with the most recently seen one (`last_seen_at DESC`, then `id DESC`), and SHALL answer
with exactly one posting.

The normalized form of a URL is: lowercased, with the `http://` or `https://` scheme and a
leading `www.` removed, with the query string and fragment removed, and with trailing
slashes removed. The same normalization SHALL be applied to the stored URL and to the
requested URL.

#### Scenario: An aggregator posting is resolved by its page URL

- **WHEN** `/api/v1/jobs/find` is called with the URL of a page that an open canonical
  posting stores as its `url`, and no source identity can be recovered from that URL
- **THEN** the response carries that posting's `public_slug`

#### Scenario: Tracking parameters do not prevent resolution

- **WHEN** the requested URL differs from the stored one only by query string, fragment,
  `www.` prefix, scheme, letter case, or trailing slashes
- **THEN** the posting is resolved as if the URLs were identical

#### Scenario: A closed or duplicate posting is not resolved

- **WHEN** the only postings matching the normalized URL are closed (`closed_at` set) or
  suppressed (`duplicate_of` set)
- **THEN** the response is `{"data": null}`

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
