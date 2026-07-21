# company-name-backfill Specification

## Purpose
TBD - created by archiving change add-company-name-backfill. Update Purpose after archive.
## Requirements
### Requirement: Slug-like company selection

The worker SHALL operate only on companies whose stored name is still slug-like — a single lowercase token with no whitespace and no uppercase letters (e.g. `lbresearch`, `gs1ca`, `afcb`) — and SHALL restrict work to companies that have at least one live (open) job. Companies with a human-readable name, and dead or empty boards, SHALL be skipped.

#### Scenario: Slug-like name with live jobs is selected
- **WHEN** a company's stored name is a single lowercase token and the company has one or more open jobs
- **THEN** the worker attempts to resolve a real display name for it

#### Scenario: Human-readable name is skipped
- **WHEN** a company's stored name already contains a space or an uppercase letter (e.g. `AFC Bournemouth`)
- **THEN** the worker leaves the company untouched and does not attempt resolution

#### Scenario: Slug-like name with no live jobs is skipped
- **WHEN** a company's stored name is slug-like but no open job references it
- **THEN** the worker does not attempt resolution for that company

### Requirement: Native-source name resolution per ATS

The worker SHALL resolve a candidate display name from the ATS's own native source for that company's board: the careers-page HTML `<title>` for title-bearing ATSes (BambooHR, Lever, Ashby, Pinpoint), or an ATS API field where exposed (Greenhouse board `name`, Workday). The worker SHALL NOT derive a name by prettifying or guessing from the slug itself.

#### Scenario: Careers-page title yields a name
- **WHEN** the board's careers page returns a title of the form `Jobs at {Name} | {Name} Careers`
- **THEN** the worker extracts `{Name}` as the candidate display name

#### Scenario: API field yields a name
- **WHEN** the ATS exposes an organization/board display name via its API (e.g. Greenhouse board `name`)
- **THEN** the worker uses that field as the candidate display name

#### Scenario: No name is invented from the slug
- **WHEN** no native source name is available for a board
- **THEN** the worker emits no candidate name and leaves the company untouched

### Requirement: Name acceptance gate

Before applying a resolved name the worker SHALL decode HTML entities in the candidate, reject empty, test-board, and recruiter-style titles, and require a confidence match — the decoded candidate SHALL share a token or an acronym with the slug. A candidate that fails any check SHALL be discarded and the company left untouched.

#### Scenario: HTML entities are decoded
- **WHEN** a candidate name contains HTML entities (e.g. `Bob&#39;s Red Mill`, `Aspire Allergy &amp; Sinus`)
- **THEN** the applied name is the decoded form (`Bob's Red Mill`, `Aspire Allergy & Sinus`)

#### Scenario: Low-confidence candidate is rejected
- **WHEN** the decoded candidate shares no token or acronym with the slug (e.g. slug `kempinski` → `Elena - Meta Recruitment`)
- **THEN** the candidate is discarded and the company name is left unchanged

#### Scenario: Test/empty title is rejected
- **WHEN** the candidate is empty or a test-board artifact (e.g. `Joe's Test Platform`)
- **THEN** the candidate is discarded and the company name is left unchanged

### Requirement: Idempotent application and propagation

Applying a resolved name SHALL update the ingested company name so that the derived `companies` catalogue reconciles through the existing sync path, and SHALL be idempotent — a second run over the same data resolves nothing new because previously fixed names are no longer slug-like.

#### Scenario: Applied name propagates to the catalogue
- **WHEN** the worker applies a resolved name to a company's ingested jobs
- **THEN** the `companies` catalogue reflects the new name after the existing `SyncCompaniesFromJobs` + `DeleteOrphanCompanies` reconciliation, with no manual DB edit

#### Scenario: Re-run is a no-op
- **WHEN** the worker runs again after a successful backfill with no new slug-like companies
- **THEN** it resolves and applies nothing

### Requirement: Run-once worker contract

The capability SHALL be delivered as a standalone run-once-and-exit worker (`cmd/backfill-company-names`) that requires `DATABASE_URL`, reuses the existing sources HTTP client for outbound fetches, and reports a summary of resolved, skipped, and low-confidence counts so operators can review coverage.

#### Scenario: Worker runs to completion and exits
- **WHEN** the worker is invoked with a valid `DATABASE_URL`
- **THEN** it processes eligible companies, prints a summary of resolved / skipped / rejected counts, and exits

#### Scenario: Coverage is observable, not silently capped
- **WHEN** the worker finishes a run
- **THEN** its summary distinguishes companies it fixed from those it left untouched and why (no trustworthy source vs. low confidence)

