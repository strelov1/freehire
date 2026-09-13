## Why

Every mentor profile today must name an existing `companies.slug` row — the whole
capability was built around "an insider at a company the catalogue already carries."
That excludes a real population of would-be mentors: an independent consultant, someone
between jobs, or someone whose employer simply isn't in the catalogue yet. Since the
company field is create-time-only (it never changes on an existing profile), the fix has
to happen at submission, not as an editable toggle later.

## What Changes

- A mentor profile MAY be submitted with no company at all. `company_slug` becomes an
  optional column (`ALTER TABLE mentors ALTER COLUMN company_slug DROP NOT NULL`) rather
  than a required one.
- Submitting a company that names an EXISTING catalogue row still works exactly as today
  (unchanged validation, unchanged FK-violation-to-`ErrCompanyNotFound` mapping for a
  typo'd/unknown slug). Only the "must supply *something*" rule is removed — a slug that
  IS supplied is still checked against the catalogue.
- The directory, its company filter, and the vacancy/company-page mentor entry point are
  unaffected by construction: every read query already `LEFT JOIN`s `companies`, and the
  company filter can never match a NULL `company_slug`, so a company-less mentor already
  appears in the unfiltered directory and never wrongly appears on a company's own page.
- The directory card and the owner's own profile view stop assuming a company is always
  present — both currently render `headline · company_name`/`company_name || company_slug`
  unconditionally, which would show a dangling separator or empty string for a
  company-less mentor.
- The create form's company picker becomes explicitly optional, with a way to say "no
  company" (the profile-editor's existing `{#if profile}...{:else}...{/if}` create-mode
  branch already treats company as its own labelled field — see design.md).

## Capabilities

### Modified Capabilities
- `mentor-profile`: "A mentor profile is owned by one account and names one company" no
  longer requires a company; "A profile naming an unknown company is refused" gains a
  companion scenario for the no-company case; the public directory/entry-point
  requirements gain scenarios confirming a company-less mentor is listed and never
  misattributed to a company page.

## Impact

- **Schema**: one additive migration dropping `mentors.company_slug`'s `NOT NULL`.
- **Domain** (`internal/engage/mentorship/profile.go`, `repository.go`): remove the
  "a company is required" check from `validateProfile`'s `creating` branch; `CompanySlug`
  moves through the existing `optionalText`/`pgconv.TextString` idiom already used for
  `company_name` and `seniority` in this same package — no new pattern.
- **SQL** (`internal/platform/db/queries/mentorship.sql`): no query rewrites — every read
  already `LEFT JOIN`s `companies` and the company filter is already NULL-safe. `make
  sqlc` regenerates `CompanySlug` as `pgtype.Text` after the migration.
- **HTTP** (`internal/api/handler/mentorship*.go`): no changes — the wire fields are
  already plain, non-`omitempty` strings that tolerate empty in both directions.
- **Frontend**: `MentorsView.svelte`'s directory card and `MentorProfileEditor.svelte`'s
  existing-profile company display both need a company-less fallback; the create form's
  company field gains an explicit "no company" affordance and updated copy.
