## 1. The mapping dictionary

- [x] 1.1 Add the curated domain→industry table to `internal/industrytag` (17 pairs;
      `other`, `media`, `mobility` deliberately absent) and `DomainsForIndustries`,
      which returns the sorted, de-duplicated domain values the requested industries
      can also be recognised by, inverting the table once at init
- [x] 1.2 Invariant tests: every mapped target is a canonical industry, every key is a
      `vocab.DomainValues` member, `media`/`mobility`/`other` map to nothing, an
      unknown input (`saas`, `""`) yields nothing, and the result never contains a
      value absent from `vocab.DomainValues`

## 2. Meilisearch path

- [x] 2.1 Widen the `industries` group in `CompanyFilterFromValues` with `Eq` fragments
      over the `domains` attribute for the mapped domains, keeping the group a plain OR
      and every other facet untouched
- [x] 2.2 Tests: an `industries` filter emits fragments over both attributes; an
      industry that maps to no domain emits only the curated fragment; the facets that
      do not translate produce the filter they produced before

## 3. Postgres path

- [x] 3.1 Widen the `industries` predicate in `ListCompanies` and `CountCompanies` to
      match either column, taking the mapped domains as a second argument, then
      `make sqlc`
- [x] 3.2 Resolve the mapped domains in the companies handler and pass them to both
      queries
- [x] 3.3 Integration tests (`-tags=integration`): a company matches on its curated
      industry alone; on its domain alone; a company whose only domain is unmapped
      (`other`/`media`/`saas`) matches nothing; `meta.total` counts the same set the
      list returns

## 4. The two paths agree

- [x] 4.1 A test that runs the same `industries` filter through the Meilisearch path
      and the Postgres path over one fixture set and asserts the matched sets are equal

## 5. Web

- [x] 5.1 Remove the `domains` facet from `COMPANY_FACETS` and its pane from
      `COMPANY_RAIL_GROUPS`; the rail-coverage test added in #2071 must stay green
- [x] 5.2 Update the company facet tests to assert Industry is the catalogue's only
      industry control, and that the `domains` API parameter is untouched

## 6. Documentation

- [x] 6.1 Update `docs/agents/company-facets.md` — the industry facet now reads a
      job-derived column without owning it, which is the one crossing the ownership
      rule permits and therefore worth stating explicitly
- [x] 6.2 Record the retired Domain control in `docs/API.md` if it names the companies
      filter set, leaving the `domains` parameter documented as still accepted
