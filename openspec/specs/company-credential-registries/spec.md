# company-credential-registries Specification

## Purpose

Company tags drawn from authoritative public registers rather than editorial
judgement — currently the UK Skilled Worker sponsor licence, the Dutch
recognised-sponsor licence, and USCIS's approved-H-1B-petition history. Covers
each register's source and fetch strategy, the conservative matching policy
that decides when a company earns a credential, and how a credential is
presented so it is never mistaken for a promise about a specific role.
Distinct from the job-level `enrichment.visa_sponsorship` signal, which
reports what a posting says rather than what the employer is licensed to do.

## Requirements

### Requirement: A credential is a company fact sourced from an authoritative public register

The system SHALL model a **credential** as a collection whose membership comes from
an authoritative public register rather than from editorial judgement. A credential
SHALL be distinguished from an editorial collection by a `kind` on its registry
entry, and SHALL otherwise reuse the collection machinery unchanged — the same
company-level tag set, the same denormalization onto jobs, and the same search
facet. A credential SHALL state a fact about the **employer**, never about a
specific role.

The system SHALL NOT merge a credential with the job-level
`enrichment.visa_sponsorship` signal. The two answer different questions — "the
posting says they sponsor" versus "the employer is licensed to sponsor" — and SHALL
remain independently filterable, so neither is implied by the other.

#### Scenario: A credential and the job-level sponsorship signal filter independently

- **WHEN** a user filters by the `uk-skilled-worker-sponsor` credential
- **THEN** the feed is scoped by the employer's register membership only, and the
  `visa_sponsorship` filter remains separately selectable and unaffected

#### Scenario: A credential collection reuses the collection tag set

- **WHEN** a company earns a credential
- **THEN** the credential's slug is present in the company's collection set
  alongside any editorial collections it belongs to, and is denormalized onto its
  jobs like any other collection tag

### Requirement: The UK register is resolved dynamically and gated by sponsorship route

The system SHALL source the `uk-skilled-worker-sponsor` credential from the GOV.UK
"Register of Worker and Temporary Worker licensed sponsors". Because the register's
CSV URL embeds a snapshot date and is republished on a new URL each month, the
system SHALL resolve the current URL at fetch time: first from the GOV.UK Content
API for the publication, selecting the CSV attachment by its title, and failing
that by scraping the publication's HTML page for the CSV link. A resolve that
yields no URL SHALL be an error, not an empty result.

The register lists every licensed sponsor across all immigration routes, including
routes irrelevant to the catalogue's audience. The system SHALL retain every parsed
row together with its route, and SHALL grant the credential only when at least one
of a company's rows carries a work route — Skilled Worker, any Global Business
Mobility route, or Scale-up. A company present in the register solely under a
Temporary Worker or study route SHALL NOT receive the credential.

#### Scenario: The current CSV URL is resolved from the Content API

- **WHEN** the register is fetched and the GOV.UK Content API responds
- **THEN** the CSV attachment titled for the Worker and Temporary Worker register is
  selected and downloaded, without fetching the HTML page

#### Scenario: A Content API failure falls back to the HTML page

- **WHEN** the Content API is unavailable or returns a payload that cannot be parsed
- **THEN** the publication's HTML page is scraped for the CSV link and the download
  proceeds

#### Scenario: A register entry on a work route earns the credential

- **WHEN** a matched company has a register row whose route is `Skilled Worker`
- **THEN** the company receives the `uk-skilled-worker-sponsor` credential

#### Scenario: A register entry on a temporary route does not earn the credential

- **WHEN** a matched company's only register rows carry Temporary Worker routes
- **THEN** the company does not receive the credential, and the rows are still
  retained for the run

#### Scenario: An unresolvable register URL aborts the run

- **WHEN** neither the Content API nor the HTML fallback yields a CSV URL
- **THEN** the resolve fails with an error and no credential membership is written

### Requirement: The Netherlands register grants the recognised-sponsor credential

The system SHALL source the `nl-recognised-sponsor` credential from the IND public
register of recognised sponsors for the "work" residence purpose, parsed from its
single HTML table of organisation names and KvK numbers. The KvK number SHALL be
retained as record metadata for disambiguation. The register does not break down by
route, so no route gate applies. A parse that yields no rows SHALL be an error, not
an empty result.

#### Scenario: A recognised sponsor earns the credential

- **WHEN** a matched company appears in the IND recognised-sponsor register
- **THEN** the company receives the `nl-recognised-sponsor` credential

#### Scenario: An empty parse aborts rather than clearing membership

- **WHEN** the IND page is fetched but yields zero parsed rows
- **THEN** the resolve fails with an error and no company loses its existing
  credential

### Requirement: The US register grants the H-1B sponsor credential from historical approval data

The system SHALL source the `us-h1b-sponsor` credential from the USCIS H-1B
Employer Data Hub's bulk CSV files, published one file per completed fiscal year
at a stable archive index page. Because the set of published fiscal years grows
over time, the system SHALL discover the current set of year files at fetch time
rather than pinning literal year numbers, and SHALL combine the **5 most recent**
fiscal years found into one member list. A discovery that finds fewer than 5
year files SHALL be an error, not a smaller result — a shrunk index is
indistinguishable from a genuine drop in coverage and MUST NOT silently reduce the
window.

Fetching each of the 5 year files SHALL be all-or-nothing: if any one year's file
is unreachable or fails to parse, the whole fetch SHALL fail and no membership
SHALL be written for this credential on that run, leaving the previous run's
membership intact.

Within the combined rows, the system SHALL grant the credential only to an
employer with at least one row whose approvals (initial or continuing) sum to 1 or
more across the 5-year window. An employer whose every row in the window sums to
zero approvals SHALL NOT receive the credential, even if it has denied petitions
on record. This filtering SHALL happen while building the member list, before
company matching — unlike the UK register's route gate, which reads a matched
row's field after matching, because the approval threshold is a fact about the
source row rather than about which company (if any) it matches.

The register does not publish a route or scheme breakdown — every listed petition
is H-1B — so no route gate applies, unlike the UK register.

#### Scenario: The most recent 5 fiscal years are combined into one member list

- **WHEN** the archive index page lists 15 fiscal-year files
- **THEN** only the 5 most recent are fetched and combined; older years are not
  fetched

#### Scenario: An index page listing fewer than 5 years aborts the run

- **WHEN** the archive index page lists fewer than 5 fiscal-year files
- **THEN** the resolve fails with an error and no credential membership is written

#### Scenario: A single unreachable year aborts the whole fetch

- **WHEN** 4 of the 5 target fiscal-year files fetch successfully and 1 fails
- **THEN** the resolve fails with an error, and the previous run's membership for
  this credential is left unchanged

#### Scenario: An employer with only approvals in the window earns the credential

- **WHEN** an employer's rows across the 5-year window sum to at least one initial
  or continuing approval
- **THEN** the employer's row is retained as a candidate for the `us-h1b-sponsor`
  credential

#### Scenario: An employer with only denials in the window does not earn the credential

- **WHEN** an employer's rows across the 5-year window sum to zero approvals,
  regardless of any denials on record
- **THEN** the employer's rows are dropped before company matching and it cannot
  receive the credential from this run

### Requirement: US employer names disambiguate by tax identifier and match by US geography

The system SHALL retain each parsed row's last-4-digit tax identifier (`Tax ID`) as
record metadata and SHALL use it as the ambiguity guard's identity field, the same
role the UK register's town and the Dutch register's KvK number play: two rows
sharing a normalized employer name but different tax identifiers SHALL be treated
as a naming collision and SHALL grant the credential to no company; rows sharing
both a name and a tax identifier — including an employer's rows for different
worksite cities or different fiscal years within the window — SHALL be treated as
one organisation.

The system SHALL apply the existing conservative country-matching guards
(legal-suffix stripping, geography gate, single-token rule, country comparison)
with the United States recognised as a matchable country: the country-comparison
guard SHALL recognise `US` by ISO code or by a spelled-out US name, and the
legal-suffix guard SHALL recognise US corporate forms (`Inc`, `Incorporated`,
`Corp`, `Corporation`, `LLC`) as strippable trailing suffixes, in addition to the
UK/NL forms it already recognises.

#### Scenario: Same-named employers with different tax identifiers are a collision

- **WHEN** two organisations in the register share a normalized name but carry
  different `Tax ID` values
- **THEN** no company receives the credential from that name

#### Scenario: One employer's multi-city, multi-year rows are not mistaken for a collision

- **WHEN** the register lists one employer's rows across several worksite cities
  and several fiscal years within the window, all sharing the same `Tax ID`
- **THEN** every one of those rows survives the ambiguity guard

#### Scenario: A US corporate suffix is stripped before matching

- **WHEN** the register lists `ACME ROBOTICS INC` and the catalogue holds the
  company `acme-robotics`
- **THEN** the names match after suffix stripping and normalization

#### Scenario: A spelled-out US headquarters country is recognised

- **WHEN** a single-token register name matches a company whose headquarters
  country is stored as `United States` rather than `US`
- **THEN** the company receives the credential, the comparison having recognised
  the name as denoting the same country

### Requirement: Canada, Ireland and Denmark grant credentials from their official employer lists

The system SHALL source three further credentials, each gated on the company having
open jobs in that country (`RequireCountry`):

- `ca-lmia-employer` from ESDC's positive-LMIA employers list on open.canada.ca. The
  quarterly English XLSX files SHALL be discovered from the dataset's CKAN metadata at
  fetch time and the **4 most recent** combined. Only rows in the High Wage, Global
  Talent or Permanent Resident streams SHALL earn the credential.
- `ie-permit-employer` from DETE's annual "employment permits issued to companies"
  workbooks, discovered through the statistics index and each year's page, combining
  the **3 most recent** years (the current, partial year and the two before it).
- `dk-fast-track-company` from SIRI's list of companies certified for the Danish
  Fast-track scheme, a single HTML table keyed by CVR number.

Each fetch SHALL be all-or-nothing, and fewer files than the window SHALL be an
error, as for the US register.

#### Scenario: A Canadian agricultural LMIA does not earn the credential

- **WHEN** an employer's only LMIA rows are in the Primary Agriculture or Low Wage stream
- **THEN** it does not receive `ca-lmia-employer`

#### Scenario: One unreachable Irish year aborts the whole fetch

- **WHEN** one of the 3 target years' workbook cannot be fetched
- **THEN** the resolve fails and the previous run's `ie-permit-employer` membership is
  left unchanged

### Requirement: Credential matching is conservative and never assigns on a coincidence of names

The system SHALL match a register entry to a catalogue company by normalized name,
and SHALL apply the following guards before granting a credential. All guards are
cumulative; failing any one of them SHALL leave the company untagged rather than
tagged on a guess.

- **Legal-suffix stripping.** Before normalization, a trailing legal-form suffix
  SHALL be stripped as a whole word from the **end** of the register name (for
  example `Ltd`, `Limited`, `PLC`, `LLP`, `CIC`, `B.V.`, `N.V.`). A suffix word
  appearing anywhere other than the end SHALL NOT be stripped.
- **Geography gate.** The company SHALL have a geographic link to the register's
  country. For a normalized name of two or more tokens, the register's country
  present in the company's countries SHALL suffice.
- **Single-token rule.** For a name whose normalized slug is a single unpunctuated
  token, the company's headquarters country SHALL denote the register's country. A
  single-token name is too weak a signal on its own: a multinational with a local
  office would otherwise inherit a credential from an unrelated local business of
  the same name. A name that normalizes to several slug segments — including one
  punctuated into segments, such as `Booking.com` or `Rolls-Royce` — is specific
  enough to fall under the geography gate alone.
- **Country comparison.** A stored country value SHALL be compared whole and
  case-insensitively, and SHALL be recognised by its ISO code or by a spelled-out
  name for the same country. The headquarters column has more than one writer and
  not all of them normalize, so the guard SHALL NOT depend on a particular upstream
  spelling; a substring SHALL NOT count as a match.
- **Ambiguity guard.** A normalized name shared by more than one *organisation* in
  a register SHALL be treated as identifying none of them, and SHALL grant the
  credential to no company. Organisations SHALL be told apart by a register-specific
  identity field (the UK register's town, the Dutch register's KvK number) — a
  shared name alone is not a collision, because the UK register lists a single
  organisation once per sponsorship route it holds, and those rows must survive for
  the route gate to read them. Where a register publishes no such identity field,
  same-named rows SHALL be treated as one organisation and the geography guards
  SHALL carry the decision.

#### Scenario: A legal suffix is stripped from the end of a register name

- **WHEN** the register lists `ACME ROBOTICS LIMITED` and the catalogue holds the
  company `acme-robotics`
- **THEN** the names match after suffix stripping and normalization

#### Scenario: A suffix word inside a name is not stripped

- **WHEN** the register lists `LIMITED BRANDS INC`
- **THEN** only the trailing corporate suffix is considered, and the leading
  `Limited` is retained as part of the name

#### Scenario: A multi-token name matches on the country facet alone

- **WHEN** a two-token register name matches a company that has open jobs in the
  register's country
- **THEN** the company receives the credential

#### Scenario: A single-token name is rejected without a matching headquarters

- **WHEN** a single-token register name matches a company that has open jobs in the
  register's country but whose headquarters country is elsewhere
- **THEN** the company does not receive the credential

#### Scenario: A spelled-out headquarters country is recognised

- **WHEN** a single-token register name matches a company whose headquarters country
  is stored as `United Kingdom` rather than `GB`
- **THEN** the company receives the credential, the comparison having recognised the
  name as denoting the same country

#### Scenario: A country name is not matched by substring

- **WHEN** a company's headquarters country is stored as a longer string that merely
  contains a country name
- **THEN** the guard does not treat it as a match

#### Scenario: A single-token name is accepted with a matching headquarters

- **WHEN** a single-token register name matches a company whose headquarters country
  equals the register's country
- **THEN** the company receives the credential

#### Scenario: An ambiguous name is assigned to nobody

- **WHEN** a normalized register name appears for two organisations with different
  identity fields (different towns)
- **THEN** no company receives the credential from that name, regardless of any
  other guard passing

#### Scenario: One organisation's per-route rows are not mistaken for a collision

- **WHEN** a register lists one organisation on several routes, so its name repeats
  across rows sharing the same identity field
- **THEN** every one of those rows survives the ambiguity guard and reaches the
  route gate

### Requirement: A credential is presented so it is never read as a promise about a role

The system SHALL render a company's credential as a badge on the job card and on the
company page, and SHALL accompany it with copy that states what the credential does
and does not mean: the employer holds the licence, which is not a commitment to
sponsor the role being viewed. The badge SHALL identify the issuing register so a
user can verify it independently.

In the job-search filter, credentials SHALL be presented as a group distinct from
editorial collections, so a legal qualification is not read as an editorial theme.

#### Scenario: A badge carries its disclaimer

- **WHEN** a job card renders for a company holding the `uk-skilled-worker-sponsor`
  credential
- **THEN** the badge names the credential and exposes copy clarifying that the
  licence belongs to the employer and is not a promise of sponsorship for that role

#### Scenario: Credentials are a distinct filter group

- **WHEN** a user opens the job-search filter
- **THEN** credential options appear under their own group heading, separate from
  the editorial collection options
