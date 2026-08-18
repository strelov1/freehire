## ADDED Requirements

### Requirement: A company slug SHALL be derived with one trailing legal form stripped

The system SHALL derive a company slug via `normalize.CompanySlug(name)`, which applies
`normalize.Slug` after dropping exactly one trailing legal-form token from the name's
whitespace-separated fields. Only the LAST field is considered, so a form word inside a name is
kept. A name consisting only of a legal form SHALL be slugged unstripped rather than reduced to
the empty slug. The legal-form token set SHALL live in `internal/normalize` as the single
definition, and `collections.RegisterSlug` SHALL delegate to it rather than keeping its own.

Comparison is on the token's ASCII letters only, so the punctuated and bare spellings of one
form (`B.V.`, `BV`, `Ltd.`) are the same token.

#### Scenario: A trailing legal form is stripped

- **WHEN** a job is ingested for the company `RingCentral, Inc.`
- **THEN** its `company_slug` is `ringcentral`, not `ringcentral-inc`

#### Scenario: A punctuated legal form is stripped

- **WHEN** a job is ingested for the company `Booking B.V.`
- **THEN** its `company_slug` is `booking` — the strip compares the token's letters, so `B.V.`
  matches the `bv` form even though `normalize.Slug` would render it `b-v`

#### Scenario: A form word inside a name is kept

- **WHEN** a job is ingested for the company `Limited Brands`
- **THEN** its `company_slug` is `limited-brands` — only a TRAILING field is a candidate

#### Scenario: A name that is only a legal form is not erased

- **WHEN** a job is ingested for the company `Limited`
- **THEN** its `company_slug` is `limited`, because an empty slug silently matches nothing
  while a visibly odd company row can be found and fixed

#### Scenario: Only one form is stripped

- **WHEN** a job is ingested for the company `Acme Holdings Ltd`, where `holdings` is not in
  the token set
- **THEN** its `company_slug` is `acme-holdings` — `ltd` is stripped, and the strip does not
  recurse

#### Scenario: The register matcher and the catalogue agree by construction

- **WHEN** `collections.RegisterSlug` and `normalize.CompanySlug` are given the same name
- **THEN** they return the same slug, because one calls the other — there is no second
  legal-form list that could drift

### Requirement: A frozen alias registry SHALL map spelling variants to one canonical slug

The system SHALL store, in a `company_slug_aliases` table, the mapping from a retired company
slug to the canonical slug it was merged into. Each row SHALL carry `alias_slug` (primary key),
`canonical_slug`, `folded_key` (the alias's slug with hyphens removed, indexed), `reason`
(`legal_form` or `spelling`), and `created_at`.

The table SHALL NOT be derived from `jobs` or `companies`. A canonical decision must outlive
the company row that motivated it: `DeleteOrphanCompanies` removes a `companies` row as soon as
no job references it, so a canon stored only there would evaporate the day an employer's last
posting closes, and the next variant spelling would start a fresh company.

`reason` SHALL be recorded so that a later reversal can target one class of merge without
touching the other.

#### Scenario: A merge records both directions it will be read in

- **WHEN** `dollartree` is merged into `dollar-tree`
- **THEN** a row exists with `alias_slug = 'dollartree'`, `canonical_slug = 'dollar-tree'`,
  `folded_key = 'dollartree'` and `reason = 'spelling'`

#### Scenario: The canon survives the company going quiet

- **WHEN** every open job of a canonical company closes and `DeleteOrphanCompanies` removes its
  `companies` row
- **THEN** the `company_slug_aliases` rows pointing at that canonical slug are unaffected, and a
  later posting under a variant spelling still resolves to the same canonical slug

### Requirement: A new posting's slug SHALL resolve through the alias registry before it is written

The system SHALL resolve each ingested posting's company slug through the alias registry by
folding `normalize.CompanySlug(name)` and looking the folded value up against `folded_key`,
writing the registry's `canonical_slug` when one is found and the derived slug otherwise. The
lookup SHALL be performed ONCE per board run as a single batched query over the run's distinct
slugs, and the resulting map SHALL be the sole source of company slugs for both the
aggregator-coverage gate and the upsert.

`internal/jobderive` SHALL remain a pure function with no context and no database access; it
supplies the derived slug, and the pipeline applies the registry.

#### Scenario: A never-before-seen variant spelling joins the existing company

- **WHEN** a board yields a posting for `DollarTree` and `dollartree` is registered as an alias
  of `dollar-tree`
- **THEN** the posting is stored with `company_slug = 'dollar-tree'`, even though this exact
  spelling was never itself merged

#### Scenario: The gate and the upsert cannot disagree

- **WHEN** a board run resolves its distinct company slugs
- **THEN** the aggregator-coverage gate and the upsert read the company slug from the same
  resolved map, so no code path can compare a pre-resolution slug against a stored
  post-resolution one

#### Scenario: One lookup per run, not one per posting

- **WHEN** a board run yields N postings across M distinct companies
- **THEN** the registry is queried once for the M distinct folded keys, not N times

#### Scenario: jobderive stays pure

- **WHEN** `jobderive.Derive` is called
- **THEN** it takes no context and performs no database access, and its `CompanySlug` output is
  `normalize.CompanySlug(in.Company)` — unresolved by the registry

### Requirement: The merge worker SHALL default to a dry run and roll in waves

The system SHALL provide `cmd/merge-companies`, which groups companies that have at least one
open job by `Fold(CompanySlug(name))`, elects the variant with the highest `job_count` as the
canonical slug, and reports the plan. Writing SHALL require an explicit `--apply` flag; without
it the worker only reports. A `--min-jobs N` bound SHALL restrict a run to groups whose total
open jobs reach N, so the catalogue can be collapsed in reviewed waves.

The canonical election SHALL run only against groups the current run touches, and a slug already
present in `company_slug_aliases` as a `canonical_slug` SHALL NOT be re-elected against — the
canon is frozen at first merge.

An apply run SHALL rewrite `jobs.company_slug` and `jobs.company_slug_folded` together, in
chunks, guarded by `IS DISTINCT FROM`, so a re-run writes nothing and stopping mid-way is free.

#### Scenario: The default run writes nothing

- **WHEN** `cmd/merge-companies` runs without `--apply`
- **THEN** it prints the merge plan and no row in `jobs`, `companies` or `company_slug_aliases`
  is modified

#### Scenario: The larger variant wins

- **WHEN** a folded group holds `dominos` with 14,396 open jobs and `domino-s` with 1
- **THEN** `dominos` is elected canonical and `domino-s` becomes its alias

#### Scenario: A wave is bounded by job volume

- **WHEN** the worker runs with `--min-jobs 1000`
- **THEN** only folded groups whose combined open jobs reach 1000 are merged, and smaller groups
  are left for a later wave

#### Scenario: An apply run is idempotent

- **WHEN** an apply run completes and is then run again with the same bounds
- **THEN** the second run updates zero rows

#### Scenario: The folded column is maintained with the slug

- **WHEN** the worker rewrites a job's `company_slug`
- **THEN** it writes `company_slug_folded` in the same statement, as every write path that sets
  `company_slug` must
