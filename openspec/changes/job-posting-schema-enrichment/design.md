## Context

`jobPostingJsonLd(job, origin)` in `web/src/lib/seo.ts` builds the `JobPosting`
JSON-LD from the job wire shape already loaded on `/jobs/:slug`. It emits baseline
fields conditionally (salary, location/remote, `validThrough`). The job carries a
top-level dictionary `skills` array and an `enrichment` blob with
`experience_years_min` and `education_level`; the company logo is derivable from
`job.company` via the existing `logoDevUrl` helper. None of this reaches the
`JobPosting` yet.

## Goals / Non-Goals

**Goals:**
- Enrich `JobPosting` with `hiringOrganization.logo`, `skills`,
  `experienceRequirements`, `educationRequirements`, each strictly conditional.
- First unit coverage for `jobPostingJsonLd` (currently untested).

**Non-Goals:**
- `directApply`, `identifier`, `jobBenefits` — deferred.
- No backend, data, or route changes.

## Decisions

- **Logo from the company name.** `hiringOrganization.logo = logoDevUrl(job.company)`
  when `job.company` is non-empty. logo.dev's `fallback=404` means unknown
  companies yield a URL that 404s; Google silently ignores a non-resolving logo
  (it is an optional field, not a validation error), and this reuses the exact
  source the SPA and OG cards already trust — consistent behavior over a
  per-company availability check we cannot make server-side.
- **Skills from the dictionary facet.** `skills = job.skills.join(', ')` when the
  array is non-empty. `job.skills` is the production dict facet (canonical names),
  not the raw `enrichment.skills`, matching the dict-only serving convention.
  Emitted as Google `Text` (comma-joined), the shape Google's examples use.
- **Experience as months, positive-only.** `experienceRequirements =
  { '@type': 'OccupationalExperienceRequirements', monthsOfExperience:
  experience_years_min * 12 }` only when `experience_years_min > 0`. A `0`
  (explicit entry-level) carries no SEO signal, so it is omitted rather than
  emitting `monthsOfExperience: 0`.
- **Education via a small enum map.** `education_level` → `credentialCategory`:
  `bachelor`→"bachelor degree", `master`/`phd`→"postgraduate degree"; `none` and
  any unmapped value omit `educationRequirements`. Emitted as
  `EducationalOccupationalCredential`, Google's accepted type.
- **TDD on the pure function.** `seo.test.ts` gains `jobPostingJsonLd` cases:
  each new field present, and the omission cases (no skills, zero experience,
  `education_level: none`, missing company). All output still flows through the
  unchanged `jsonLdScript` escaping.

## Risks / Trade-offs

- **404 logos.** Some emitted logo URLs will 404 for companies logo.dev doesn't
  know. Accepted: optional field, ignored by Google, no validation impact.
- **Skills as Text vs DefinedTerm.** Comma-joined Text is the simpler,
  Google-documented form; DefinedTerm would add structure with no ranking benefit
  here (YAGNI).
- **Enrichment absence.** Unenriched jobs simply omit the new fields — the
  baseline `JobPosting` is unchanged, so nothing regresses.
