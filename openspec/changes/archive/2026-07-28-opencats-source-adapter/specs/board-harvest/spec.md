## MODIFIED Requirements

### Requirement: The harvest tool validates candidate boards against the live platform API

The `harvest-boards` host tool SHALL expand a board file (`sources/<provider>.yml`)
only with boards it has live-validated: each candidate board SHALL be probed
against the platform's own live surface — its official public API, or, for a
self-hosted platform that exposes no API, the portal's own served pages — and kept
only if that surface reports at least one open job, so the committed file is the
project's own validated fact set rather than a redistributed dataset. A candidate
that is absent, closed, or unreachable SHALL be skipped, never abort the run. A
kept board SHALL be appended to the provider's board file with the company name
the platform reports (or the board id when the platform exposes none),
de-duplicated against the boards already in the file.

#### Scenario: A candidate with open jobs is kept

- **WHEN** a candidate board is probed and the platform API reports one or more
  open jobs
- **THEN** the board is appended to `sources/<provider>.yml` with the reported
  company name (or the board id as a fallback)

#### Scenario: A candidate with no open jobs is skipped

- **WHEN** a candidate board is probed and the platform API reports zero jobs or
  is unreachable
- **THEN** the board is not appended and the run continues with the other
  candidates

#### Scenario: An already-known board is not duplicated

- **WHEN** a candidate board id already appears in `sources/<provider>.yml`
- **THEN** it is filtered out before probing and not appended again

#### Scenario: A self-hosted portal is validated against its own pages

- **WHEN** a candidate belongs to a platform that publishes no vendor API, and its
  portal page lists one or more open postings
- **THEN** the candidate is validated from that page and kept exactly as an
  API-validated candidate would be

### Requirement: A provider may supply its own candidate boards by discovery

The harvest tool SHALL allow a provider whose boards are not available as a seed
list to discover its candidate boards, by implementing an opt-in discovery
capability. Discovery SHALL draw candidates from the platform API where one
exists, and MAY draw them from a third-party index of publicly scanned hosts when
the platform is self-hosted and therefore has no tenant catalogue to enumerate.
When a provider supports discovery and the tool is run with no seed file, the tool
SHALL obtain the candidate boards from discovery instead of from a seed list;
every discovered candidate SHALL then pass through the same live-validation,
de-duplication, and append steps as a seeded candidate. A provider that does not
support discovery SHALL continue to require a seed file.

#### Scenario: Discovery supplies candidates when no seed is given

- **WHEN** the tool is run for a provider that supports discovery and no seed file
  is given
- **THEN** the candidate boards come from the provider's discovery, and each is
  live-validated, de-duplicated, and appended exactly as a seeded candidate would be

#### Scenario: A provider without discovery still needs a seed

- **WHEN** the tool is run for a provider that does not support discovery and no
  seed file is given
- **THEN** the tool reports a usage error and makes no changes

#### Scenario: A self-hosted platform discovers from a third-party index

- **WHEN** discovery runs for a self-hosted platform that has no tenant catalogue
- **THEN** the candidate hosts come from a third-party index of publicly scanned
  hosts, and each is live-validated before it can be appended

## ADDED Requirements

### Requirement: OpenCATS portals are discovered from a public scan index

The harvest tool SHALL support discovering OpenCATS boards from a public index of
scanned hosts, querying it by the signatures an install exposes — the stock page
title and the portal's URL routing — and unioning the results into one
de-duplicated candidate host list. Discovery SHALL drop hosts that cannot be a
company portal, namely bare IP addresses and the project's own documentation and
demo sites. Each surviving host SHALL be probed by requesting the portal's listing
and counting its posting links, and kept only when that count is at least one,
with the proposed company name read from the portal page title and falling back to
the host.

#### Scenario: Signatures are unioned into one candidate list

- **WHEN** discovery queries the index by page title and by URL routing, and a host
  appears in both result sets
- **THEN** that host appears exactly once in the candidate list

#### Scenario: A host serving postings is kept and named

- **WHEN** a candidate host's portal listing links one or more postings
- **THEN** the host is kept with the company name taken from its portal page title,
  or the host itself when the title yields no usable name

#### Scenario: A host serving no postings is skipped

- **WHEN** a candidate host is unreachable, serves no portal, or serves a portal
  with zero postings
- **THEN** it is not appended and the run continues with the other candidates

### Requirement: Candidates belonging to a covered sibling provider are excluded

Commercial CATS shares its URL scheme with its open-source descendant and is already
crawled under its own provider, so OpenCATS discovery SHALL exclude any candidate
host belonging to that sibling platform. Admitting such a host would create the same
posting under two providers, which the `(source, external_id)` dedup key cannot
detect.

#### Scenario: A commercial CATS host is rejected

- **WHEN** discovery surfaces a host on the commercial CATS platform
- **THEN** that host is excluded from the candidate list and never appended to
  `sources/opencats.yml`
