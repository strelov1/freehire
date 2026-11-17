## MODIFIED Requirements

### Requirement: A page URL resolves to the catalog posting stored under that URL

The system SHALL resolve the URL of a job page to a catalog posting when no source
identity can be recovered from the URL, by comparing the URL to `jobs.url` in a normalized
form. Only open, canonical postings (`closed_at IS NULL` and `duplicate_of IS NULL`) are
eligible. When several eligible postings share a normalized URL, the system SHALL answer
with the most recently seen one (`last_seen_at DESC`, then `id DESC`), and SHALL answer
with exactly one posting.

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

- **WHEN** the only postings matching the normalized URL are closed (`closed_at` set) or
  suppressed (`duplicate_of` set)
- **THEN** the response is `{"data": null}`

#### Scenario: An unknown page stays unresolved

- **WHEN** no posting stores the requested URL and no source identity can be recovered
  from it
- **THEN** the response is `{"data": null}`
