## Why

`company-credential-registries` already proves the abstraction with two registers
(UK Skilled Worker, NL recognised sponsor); its own design doc deferred a third as
speculative until the shape was proven in production. It now has two live registers
and the exact "one registry entry" seam the abstraction was built for. The US H-1B
market is the single most-asked-about visa question among freehire's candidate base,
and USCIS publishes a public, authoritative dataset (the H-1B Employer Data Hub)
that answers it the same way GOV.UK and IND answer it for their countries — this
adds the missing US register using the existing machinery unchanged.

## What Changes

- New credential collection `us-h1b-sponsor`: a badge for employers with at least
  one USCIS-approved H-1B petition (initial or continuing) in the last 5 completed
  fiscal years, sourced from the public USCIS H-1B Employer Data Hub bulk CSV files.
- New self-fetching dataset source (`internal/collections/ush1bsponsor.go`) that
  discovers the most recent 5 fiscal-year CSV links from the USCIS archive index
  page, fetches and parses each, and fails the whole fetch if any year is
  unreachable or the index no longer lists at least 5 years.
- `internal/collections/register.go`: extend `countryAliases` with US spellings and
  `legalSuffixes` with US corporate forms (`Inc`, `Incorporated`, `Corp`,
  `Corporation`, `LLC`) so the existing name-matching guards work for US employer
  names — deliberately excludes the ambiguous `Co` token.
- `web/src/lib/credentials.ts`: one new badge-copy entry for `us-h1b-sponsor`,
  issuer `USCIS`, mirroring the UK/NL disclaimer pattern.
- No change to `cmd/import-collections`, the ambiguity guard, or the
  `/collections/[slug]` routes — all three are already generic over any
  `KindCredential` entry.
- Explicitly out of scope: DOL/OFLC LCA disclosure data (job-level wage/title data —
  a distinct future feature, not a credential fact about the employer), approval
  counts or fiscal-year lists in the UI (bare badge only, like UK/NL), and any
  route/tier gate (the Data Hub is H-1B-specific already, unlike GOV.UK's
  multi-route register).

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `company-credential-registries`: adds a third register requirement (US H-1B,
  sourced from the USCIS Employer Data Hub) alongside the existing UK and NL
  requirements. No existing requirement's behavior changes; this is an additive
  requirement following the same pattern.

## Impact

- **Affected code**: `internal/collections/collections.go` (registry entry),
  `internal/collections/register.go` (country aliases, legal suffixes), new
  `internal/collections/ush1bsponsor.go` and its test file,
  `internal/collections/register_test.go` (extend the both-credentials invariant
  test to three), `web/src/lib/credentials.ts` and its test file, regenerated
  `web/src/lib/generated/contracts.ts` (via `make gen-contracts`).
- **Affected systems**: `cmd/import-collections` picks up the new register on its
  next run with no code change; Meilisearch's `collections` filterable attribute
  gains one more possible value with no index-shape change.
- **External dependency**: `uscis.gov` availability and page/CSV format stability.
  The built-in fetch tooling used during design research got HTTP 403 from
  `uscis.gov` while a plain HTTP client with a standard browser User-Agent got 200 —
  worth confirming `cmd/import-collections`' outbound requests behave the same way
  production traffic already does for the UK/NL fetches (both also plain
  `net/http` calls) before assuming this needs any special handling.
