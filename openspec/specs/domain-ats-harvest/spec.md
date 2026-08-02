# domain-ats-harvest Specification

## Purpose
TBD - created by archiving change add-domain-ats-harvest. Update Purpose after archive.
## Requirements
### Requirement: ATS detection from page HTML

The system SHALL provide a pure function that detects a supported ATS board from a
page's HTML, returning the provider and board slug. It SHALL recognise Greenhouse
(`boards.greenhouse.io/<slug>` and the `embed/job_board?for=<slug>` variant), Lever
(`jobs.lever.co/<slug>`), and Ashby (`jobs.ashbyhq.com/<slug>`). The detected slug
SHALL be validated to a sane shape (lowercase alphanumeric and hyphens); a match
that does not yield such a slug SHALL be treated as no detection. When several
providers' links are present, the function SHALL return a single deterministic
result. The function SHALL perform no I/O.

#### Scenario: Direct Greenhouse board link

- **WHEN** HTML contains `https://boards.greenhouse.io/acme`
- **THEN** detection returns provider `greenhouse` and slug `acme`

#### Scenario: Greenhouse embed link

- **WHEN** HTML contains `boards.greenhouse.io/embed/job_board?for=acme`
- **THEN** detection returns provider `greenhouse` and slug `acme` (not `embed`)

#### Scenario: Lever and Ashby links

- **WHEN** HTML contains `https://jobs.lever.co/acme` (or `https://jobs.ashbyhq.com/acme`)
- **THEN** detection returns provider `lever` (or `ashby`) and slug `acme`

#### Scenario: No ATS link

- **WHEN** HTML contains no supported ATS URL
- **THEN** detection returns ok = false

### Requirement: Unmatched-company extraction from collection datasets

The system SHALL provide an extract step that reads the collection datasets (using
the dataset URLs from the collections registry as the single source of truth for
their locations), parses each company's name and website, and emits only the
companies whose normalized-name slug is **absent** from a supplied set of existing
company slugs. A company with no website SHALL be omitted. The output SHALL pair
each emitted company's name with its website so the resolve step can fetch it.

#### Scenario: A company already in the catalogue is dropped

- **WHEN** a dataset company's normalized-name slug is present in the supplied
  company-slug set
- **THEN** it is not emitted by the extract step

#### Scenario: An unmatched company with a website is emitted

- **WHEN** a dataset company's slug is absent from the set and it has a website
- **THEN** it is emitted with its name and website

#### Scenario: A company without a website is omitted

- **WHEN** a dataset company has no website
- **THEN** it is not emitted (there is nothing to follow)

### Requirement: Website-to-board resolution writes per-provider seeds

The system SHALL provide a resolve step that, for each input company, fetches a
small fixed set of candidate career pages (the homepage, common careers/jobs paths,
and a careers/jobs link discovered on the homepage) through the shared HTTP client,
detects the ATS board each page belongs to, and stops at the first page that yields a
board. Detection SHALL first scan the page for the URL shapes that name a platform, and
where that yields nothing SHALL additionally recognise the platforms whose board *is* the
careers host and which therefore link out to no ATS domain at all — resolving them to that
host. A page that both links a board and is served by such a platform SHALL resolve to the
linked board. It
SHALL accumulate the detected slugs per provider and write one seed file per
provider (the input format the existing `harvest-boards` consumes). The run SHALL
be best-effort: a failure fetching or parsing one company (timeout, bot-block,
missing careers page, JS-only page) SHALL be logged and skipped without aborting
the run. Resolution SHALL NOT write to `sources/*.yml` directly — the per-provider
seeds feed `harvest-boards`, which validates each slug against the provider API
before any board is committed.

#### Scenario: A resolved company contributes to its provider's seed

- **WHEN** a company's careers page links to `jobs.lever.co/acme`
- **THEN** `acme` appears in the lever seed file

#### Scenario: A linked board wins over the host serving the page

- **WHEN** a company's careers site is served by a platform whose tenant host is the board,
  and the page also links to a different ATS board
- **THEN** the linked board is detected, not the host

#### Scenario: A career site that is itself the board is resolved

- **WHEN** a company's careers site is served by a platform whose tenant host is the
  board, and the page links to no external ATS domain
- **THEN** the platform and the host are detected as the board and contribute to that
  provider's seed

#### Scenario: A failed company does not abort the run

- **WHEN** one company's site times out or has no detectable ATS
- **THEN** it is skipped and logged, and the remaining companies are still processed

#### Scenario: Seeds feed the existing validation step

- **WHEN** the resolve step finishes
- **THEN** it has written per-provider seed files (no `sources/*.yml` writes), which
  `harvest-boards <provider> <seed>` then validates and commits

### Requirement: Candidate board slugs are derived offline when no page yields a board

When a company's career pages yield no board and the input supplies the ATS-native
identifier of one of that company's live postings, the resolve step SHALL derive candidate
board slugs from what it already knows about the company — its domain, its platform profile
slug, and its name — and SHALL emit each candidate into the seed of every provider the
identifier's shape is consistent with, carrying the identifier as the expected posting id.
Derivation SHALL perform no I/O and SHALL be bounded to a small fixed number of slugs per
company. An identifier whose shape narrows to no provider SHALL yield no candidates, and a
company with no identifier SHALL be skipped exactly as it is today.

Proposing a slug is safe only because the seed carries the identifier that will confirm it:
`harvest-boards` keeps such a candidate only when the platform reports a live posting with
that id, so a slug that happens to belong to some other employer is rejected on the
evidence rather than on a name resemblance.

#### Scenario: A JS-only careers page still yields candidates

- **WHEN** none of a company's career pages yields a board, and the input carries the
  ATS-native id of one of its postings
- **THEN** candidate slugs derived from the company's domain, profile slug and name are
  emitted with that id as the expected posting id

#### Scenario: Candidates go only to providers the id shape is consistent with

- **WHEN** the identifier's shape is consistent with a subset of the supported providers
- **THEN** the candidate slugs are emitted into exactly those providers' seeds and no others

#### Scenario: An unrecognised identifier shape yields nothing

- **WHEN** the identifier's shape narrows to no provider
- **THEN** no candidate slugs are emitted for that company

#### Scenario: A company without an identifier is unaffected

- **WHEN** no board is found and the input carries no ATS-native identifier
- **THEN** the company is skipped and logged exactly as before, with no candidates emitted

#### Scenario: Derivation costs no requests

- **WHEN** candidate slugs are derived for a company
- **THEN** no HTTP request is made for the derivation itself

