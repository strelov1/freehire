## 1. Enrich JobPosting JSON-LD

- [x] 1.1 RED: add `seo.test.ts` cases for `jobPostingJsonLd` — a fully-enriched
  job (company name, `skills`, `experience_years_min > 0`, `education_level`
  bachelor/master/phd) emits `hiringOrganization.logo`, `skills` (joined),
  `experienceRequirements.monthsOfExperience` = years×12, and
  `educationRequirements.credentialCategory` (correct mapping); an omission job
  (no skills, `experience_years_min` 0, `education_level` none, no company) emits
  none of the four. Watch them fail.
- [x] 1.2 GREEN: extend `jobPostingJsonLd` in `web/src/lib/seo.ts` (import
  `logoDevUrl`, add the four conditional fields + the education map) until tests
  pass.

## 2. Verify

- [x] 2.1 Run `vitest run`, `svelte-check`, and lint on changed files locally;
  all green.
- [x] 2.2 `simplify` pass over the diff; re-run tests green.
