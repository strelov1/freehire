## MODIFIED Requirements

### Requirement: The YC directory importer enriches existing companies and adds the rest

The system SHALL provide a run-once worker (`cmd/import-yc`) that fetches the
yc-oss directory, maps each entry, and resolves it to a company by its current-name
slug **or any former-name slug** — the first that matches an existing company — and
upserts there: an existing company has its company-info columns and curated YC
facets refreshed, and an entry matching no existing company (by any name) is inserted
as a reference row (`is_reference = true`) with no jobs under its current-name slug,
so the full YC directory is held.

The upsert SHALL NOT overwrite company-info values another source has already
stored: it SHALL write `tagline` only when the stored one is NULL or empty, merge
`company_info` JSONB key-wise with the stored value winning collisions, and union
`industries` with the stored values rather than replacing them. The industries it
contributes SHALL be resolved through the curated industry dictionary first, so the
importer cannot introduce a second spelling of an industry.

To avoid homonym collisions (a well-known
non-YC company sharing a normalized name with a small YC startup), the worker SHALL
NOT enrich a matched **existing** company when that company plainly dwarfs the YC
entry — specifically when the company's open-job count exceeds the YC entry's team
size (above a small floor) — and SHALL count such skips separately; reference-row
inserts are never guarded. The upsert SHALL NOT modify a company's `job_count`,
`collections`, or job-derived facet arrays. The worker SHALL be idempotent —
re-running rewrites the same values — and SHALL report matched vs inserted vs
skipped-collision counts.

#### Scenario: Existing company is enriched

- **WHEN** the worker processes an entry whose normalized name matches a company row
  whose open-job count does not exceed the YC entry's team size
- **THEN** that company's employee count, founding year, HQ country,
  `company_info.description`, and curated YC facets are set; its industries gain the
  entry's canonical industries; and its `job_count`/`collections`/job-derived facets
  are unchanged

#### Scenario: An existing tagline outranks the YC one-liner

- **WHEN** the worker enriches a company that already stores a non-empty tagline
- **THEN** the stored tagline is unchanged and the entry's `one_liner` is discarded

#### Scenario: A former name matches an existing company instead of inserting a duplicate

- **WHEN** the worker processes an entry whose current name matches no company but
  whose `former_names` slug matches an existing company
- **THEN** that existing company is enriched (no new reference row is inserted), and
  its display `name` is left unchanged

#### Scenario: A homonym collision is skipped, not enriched

- **WHEN** the worker matches an entry to an existing company whose open-job count
  exceeds the (known, non-zero) YC entry team size above the floor — e.g. a company
  with 620 open jobs matching a YC startup with 11 employees
- **THEN** the company's YC facets are left untouched and the entry is counted as a
  skipped collision, not applied

#### Scenario: Unmatched entry is inserted as a reference row

- **WHEN** the worker processes an entry whose current name and every former name
  match no company
- **THEN** a `companies` row is inserted with `is_reference = true` under the
  current-name slug, carrying the mapped company-info and yc facets, and `job_count = 0`

#### Scenario: Re-running is idempotent

- **WHEN** the worker runs twice over the same directory
- **THEN** the second run writes the same company-info and yc facet values as the first

### Requirement: yc-oss entries map to company-info fields

The system SHALL provide a pure mapping from a yc-oss directory entry
(`yc-oss.github.io/api/companies/all.json`) to company-info fields: `one_liner` →
tagline, `long_description` → a `company_info.description`, the union of `industry`,
`industries`, `subindustry`, and `tags` (de-duplicated) → industries, `team_size` →
employee count, `launched_at` → founding year, and `all_locations` → an HQ country.

The industries produced by this mapping SHALL be resolved through the curated
industry dictionary, so entries outside the vocabulary contribute nothing rather
than a directory-specific spelling.

#### Scenario: Directory industry fields resolve to canonical values

- **WHEN** an entry's industry fields are mapped
- **THEN** the resulting industries are canonical values of the curated vocabulary,
  and fields outside it contribute nothing
