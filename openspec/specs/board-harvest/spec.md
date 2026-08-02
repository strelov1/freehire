# board-harvest Specification

## Purpose
TBD - created by archiving change gupy-board-discovery. Update Purpose after archive.
## Requirements
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

Live jobs alone do not make a board the right board. When the seed names the employer a
candidate is expected to belong to, and the platform reports a company name of its own, the
two SHALL agree — compared after normalizing case, punctuation and legal-form suffixes — or
the candidate SHALL be rejected and counted separately from an unreachable one, so a
mismatch is visible as a mismatch rather than as an absent board. A seed that names no
expected employer SHALL be validated on live jobs alone, and a platform that reports no name
of its own SHALL keep taking the seed's name as its label.

A seed entry MAY instead name the ATS-native id of a posting the candidate board is expected
to contain. When it does, and the platform's probe reads the ids of the board's live
postings, the board SHALL be kept only if that id is among them, and rejected otherwise —
counted separately from an unreachable candidate, as a name mismatch is. An expected id
SHALL take precedence over a name comparison for the same candidate, since it identifies the
board by evidence rather than by resemblance. An expected id on a provider whose probe reads
no posting ids SHALL leave validation unchanged rather than rejecting the candidate, so
supplying one is never worse than omitting it.

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

#### Scenario: A live board owned by a different employer is rejected

- **WHEN** the seed expects a candidate to belong to one employer, and the platform reports
  open jobs under a different company name
- **THEN** the board is not appended, and the run reports it as a name mismatch rather than
  as a skipped or unreachable candidate

#### Scenario: Legal-form suffixes and punctuation do not break agreement

- **WHEN** the seed's expected employer and the platform's reported name differ only in
  case, punctuation, or a legal-form suffix
- **THEN** the names are treated as agreeing and the board is kept

#### Scenario: A seed without an expected employer is unaffected

- **WHEN** a seed entry names no expected employer
- **THEN** the candidate is validated on live jobs alone, exactly as before

#### Scenario: A board containing the expected posting is kept

- **WHEN** a seed entry names an expected posting id and the platform reports a live
  posting with that id on the candidate board
- **THEN** the board is appended exactly as a live-validated candidate would be

#### Scenario: A board that does not contain the expected posting is rejected

- **WHEN** a seed entry names an expected posting id and the candidate board's live
  postings do not include it
- **THEN** the board is not appended, and the run reports it as an id mismatch rather than
  as a skipped or unreachable candidate

#### Scenario: An expected id is not weakened by a name comparison

- **WHEN** a seed entry names both an expected posting id and an expected employer, and the
  platform reports the posting under a company name that does not match the seed's
- **THEN** the expected id decides the outcome and the board is kept

#### Scenario: An expected id on a provider that reports no posting ids is inert

- **WHEN** a seed entry names an expected posting id for a provider whose probe does not
  read the ids of live postings
- **THEN** the candidate is validated exactly as it would be without the expected id

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

### Requirement: Gupy boards are discovered from the global jobs feed

The harvest tool SHALL support discovering Gupy boards from Gupy's global jobs
feed. Discovery SHALL page the feed (across all companies, not filtered to any job
category) collecting each posting's distinct numeric `companyId`, stopping when a
page returns no postings or a bounded maximum is reached. Each discovered
`companyId` SHALL be validated by querying the company's feed for its open-job
count and its reported career-page name, kept only when it has at least one open
job, with the name falling back to the `companyId` when the feed reports none.

#### Scenario: Gupy discovery collects distinct companies from the feed

- **WHEN** Gupy discovery pages the global jobs feed and the pages name several
  companies, some more than once
- **THEN** each distinct `companyId` is collected once as a candidate board

#### Scenario: Gupy discovery stops at an empty page

- **WHEN** a Gupy feed page returns no postings
- **THEN** discovery stops paging and returns the companies collected so far

#### Scenario: A discovered Gupy company is validated and named

- **WHEN** a discovered `companyId` is probed and its feed reports open jobs and a
  career-page name
- **THEN** the board is kept with that name; a company whose feed reports no name
  falls back to the `companyId`, and one with no open jobs is skipped

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

