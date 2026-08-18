## 1. Precedence on both backends

- [x] 1.1 Postgres: guard the derived arm of the `industries` predicate with
      `cardinality(industries) = 0` in `ListCompanies` and `CountCompanies`, then
      `make sqlc`
- [x] 1.2 Meilisearch: make the derived fragment a conjunct with `industries IS EMPTY`
      in `CompanyFilterFromValues`, keeping the group a plain OR of the two arms
- [x] 1.3 Integration tests: a company whose curated industry does not match is absent
      even when its domains map to the requested value; the derived-only case still
      matches; the backend-agreement subtest covers a curated-and-domains company

## 2. The two remaining domains

- [x] 2.1 Map `media` → `entertainment` and `mobility` → `transportation`, leaving
      `other` the only deliberate omission, and record in the table's comment that
      contested placements are settled against NAICS/Crunchbase rather than by argument
- [x] 2.2 Add the missing aliases: `media`, `publishing`, `digital-publishing`,
      `social-media`, `creator-economy` → `entertainment`; `mobility`,
      `urban-mobility`, `ride-hailing`, `mobility-as-a-service` → `transportation`
- [x] 2.3 Update the dictionary tests: the deliberate-omission list is now `other`
      alone, and the two new pairs resolve

## 3. Verify against the reported defect

- [x] 3.1 A test reproducing the production case: a company shaped like Uber (curated
      `{ai, data-analytics, logistics}`, domains including `gamedev`) must not match
      `industries=gaming`
- [ ] 3.2 After deploy, confirm on production that `?industries=gaming&q=uber` no
      longer returns Uber, and that a derived-only company still resolves
