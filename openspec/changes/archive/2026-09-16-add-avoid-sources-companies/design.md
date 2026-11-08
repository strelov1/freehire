## Context

`internal/identity/userprofile` already stores `excluded_skills` as a `text[]`
column on `user_profiles`, normalized and capped by `Service.Save`, and served
through `GET`/`PUT /api/v1/me/profile` (`internal/api/handler/me_profile.go`). The
frontend renders it as a "Skills to avoid" control inside `SkillsPicker.svelte`,
which is embedded on the "Skills" tab of `/my/profile`
(`web/src/routes/my/profile/skills/+page.svelte`,
`web/src/routes/my/profile/+layout.svelte` for the tab nav). See proposal.md for
why this is being extended to sources and companies.

`Repository.Upsert`/`UpsertIfUnchanged` and `Service.Save` already take six
positional parameters each; this change adds two more to each rather than
introducing an options struct, matching the file's existing style (surgical
change — no unrelated refactor).

## Goals / Non-Goals

**Goals:**

- Persist `excluded_sources` and `excluded_companies` on the profile with the same
  normalize/cap/optional semantics `excluded_skills` already has.
- Give the user one place (`/my/profile/avoid`) to manage all three exclusion
  lists, removing the skills-to-avoid control from the Skills tab.
- Reuse existing dictionaries for the new pickers: the `source` facet vocabulary
  and label map already used by job search, and the existing company search
  (`api.listCompanies`) already used by the job-search company filter.

**Non-Goals:**

- No change to how `excluded_skills` (or the new fields) are consumed — the digest
  exclusion, the match-analysis avoid action, and the assistant profile tool keep
  their current scope. Wiring `excluded_sources`/`excluded_companies` into those
  consumers, or into general job search filtering, is a separate future change.
- No new API endpoints — the existing `/api/v1/me/profile` resource grows two
  fields.
- No change to the `jobs.source` or `company_slug` vocabularies themselves.

## Decisions

**One shared normalization helper, not per-field duplication.** Add
`normalizeExcludedSet(values []string, maxLen, maxCount int) ([]string, error)` in
`internal/identity/userprofile` and use it for both `excluded_sources` and
`excluded_companies`. `excluded_skills` keeps its existing `normalizeSkillList`
untouched (it also needs the `subtractSkills` wanted/avoided exclusivity step that
sources/companies don't have) — sharing further would require reshaping working,
tested code for no behavioral gain. Both new fields use `maxCount = 200` (mirrors
`maxSkills`, for consistency rather than a measured need — the closed `source`
vocabulary is much smaller, but a shared constant avoids inventing a second number
with no data behind it either) and `maxLen = 100` (company slugs run longer than
skill tokens; source values are short adapter names, so 100 is generous headroom
for both without a separate constant per field).

**No wanted/avoided exclusivity for sources or companies.** `excluded_skills`
drops a skill that's also in the wanted `skills` set, because a skill can be
wanted or avoided but not both. Sources and companies have no "wanted" list to
conflict with, so this rule simply doesn't apply — `Save` calls
`normalizeExcludedSet` directly for these two fields with no subtraction step.

**Positional params over an options struct.** `Repository.Upsert`/
`UpsertIfUnchanged` and `Service.Save` each gain two more `[]string` parameters
in place, growing from 6 to 8. An options struct would be a cleaner shape at this
size, but it's an unrelated refactor of working call sites (including
`MergeSkills`'s two `Upsert`/`UpsertIfUnchanged` calls) that the proposal doesn't
ask for — AGENTS.md's "surgical changes" guidance applies here.

**Avoid tab is a new sibling route, not a modal.** The existing "Profile
management UI" requirement in `search-profiles/spec.md` describes an edit modal,
but the shipped frontend already uses tabbed sub-routes under `/my/profile`
(`/my/profile/skills` etc.) — that requirement is stale against the current
implementation and reconciling it is out of scope here. The new Avoid tab follows
the actual shipped pattern: a thin `+page.svelte` under
`/my/profile/avoid` rendering a new `AvoidCard.svelte`, registered in
`+layout.svelte`'s tab list next to Skills.

**Sources picker reuses the search filter's source vocabulary, not a new
dictionary.** `internal/dict` has a canonical dictionary for skills
(`skilltag`) but not for `source` or `company_slug` — those are populated
dynamically from Meilisearch facet distribution / the companies SQL lookup
respectively. The new pickers call the same frontend helpers
(`sourceLabel()` / the dynamic source facet fetch, and `companySearch()`) the
job-search filters already use, rather than inventing a second source of truth.

## Risks / Trade-offs

- **`Save`/`Upsert` signatures grow to 8 positional `[]string`-ish params** →
  mitigated by keeping every new param the same shape as its neighbors and by
  the existing convention already being positional; a future change that adds a
  fourth exclusion list should reconsider the options-struct trade-off, not this
  one.
- **Existing "Profile management UI" spec requirement describes a modal that no
  longer matches the shipped UI** → not fixed here (out of scope); the new Avoid
  tab requirement is written against the real, shipped tab structure so it
  doesn't compound the drift.
- **`excluded_sources`/`excluded_companies` are stored but not yet enforced
  anywhere** → deliberate (see Non-Goals); documented in the requirement text so
  it isn't mistaken for an oversight during review.

## Migration Plan

1. Add migration `NNNN_user_profile_excluded_sources_companies.sql` (new columns,
   `NOT NULL DEFAULT '{}'::text[]`, mirroring migration 0039).
2. Update `internal/platform/db/queries/user_profile.sql`, run `make sqlc`.
3. Extend `userprofile.Profile`/`Repository`/`Service.Save` and
   `me_profile.go`'s DTOs, in that order, each with its own tests.
4. Frontend: new route + `AvoidCard.svelte`, remove the avoid block from
   `SkillsPicker.svelte`, extend `profile.svelte.ts`/`types.ts`/`api.ts`.
5. Deploy is a single ordinary release — no backfill needed (`DEFAULT '{}'`
   covers every existing row) and no reindex (`internal/search` is untouched).
   No rollback concern beyond the ordinary migration-then-deploy order this repo
   already follows.
