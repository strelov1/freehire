## Context

`mentors.company_slug` is `NOT NULL REFERENCES companies(slug) ON DELETE CASCADE`
(`migrations/0145_mentorship.sql`), and `company_slug` is create-time-only — it never
appears in `UpdateMentorProfile`'s SET list, so a profile's company is fixed forever once
submitted. Every READ query already `LEFT JOIN`s `companies` (the join exists purely
because `company_name` is denormalised for display, not because company itself was ever
optional), and the directory's `company` filter is already a `sqlc.narg` equality check
that a NULL column value can never satisfy. See proposal.md for the "why."

## Goals / Non-Goals

**Goals:**
- Let a profile be submitted with no company, with the existing company-supplied path
  (validation, catalogue check, display, filtering) completely unchanged.
- Reuse the exact `optionalText`/`pgconv.TextString` idiom this same package already uses
  for `company_name` and `seniority` — no new nullable-string pattern.

**Non-Goals:**
- No way to ADD or CHANGE a company on an existing profile. Company is, and remains,
  create-time-only; retrofitting an edit path is a separate, larger change (it would need
  to decide what happens to `mentors_company_published_idx` re-indexing, moderation
  re-review, etc.) and nothing about this change requires it.
- No new UI affordance (checkbox, radio, etc.) for "I have no company." `CompanyPicker`
  already emits `onSelect(null)` when its query is cleared or never filled in — leaving it
  untouched and simply not picking anything IS the "no company" path once the backend
  stops requiring one.
- No change to the moderation queue's, the public profile read's, or the "vacancy leads to
  a mentor" entry point's own LOGIC — all three already tolerate a NULL/empty company by
  construction (see proposal.md - Impact) and need no new query or validation code. Their
  DISPLAY does need the same `companyLabel` fallback the directory card gets — the
  moderation queue in particular renders a profile's company twice (`MentorReviewView.svelte`)
  and both sites are in scope alongside `MentorsView.svelte`/`MentorProfileEditor.svelte`.

## Decisions

**Migration: `ALTER TABLE mentors ALTER COLUMN company_slug DROP NOT NULL`, not a new
column or a sentinel value.** A separate "no_company boolean" or a sentinel slug like
`'__none__'` would need its own validation to keep it out of the `companies` FK's reach
and its own case in every query — NULL already means "no company" everywhere else a
company_slug-shaped column exists in this schema (see `jobs.company_slug`, itself
nullable), so this follows the codebase's own established vocabulary for the concept
rather than inventing a second one. Purely additive: existing rows are all non-NULL
already, so nothing to backfill. Filed as `migrations/0161_mentors_company_optional.sql`.

**Validation: delete the "a company is required" check, don't relax it to a softer
rule.** `validateProfile`'s `creating` branch currently refuses an empty
`strings.TrimSpace(in.CompanySlug)`. The company-EXISTS check (the FK violation mapped to
`ErrCompanyNotFound` in `repository.go`) is untouched — a typo'd slug is still refused
exactly as before. Only the "must supply *something*" gate goes away.

**Frontend field: relabel and drop the requirement, don't restructure the form.** The
create-mode company field becomes `"Your company — optional"` with a one-line hint
("Leave blank if you're independent or your employer isn't listed") instead of gaining a
toggle. This is the minimal change that matches how `MentorProfileEditor.svelte` already
treats every other optional field (seniority, meeting link) — a plain optional control
with explanatory copy, not a structurally different widget.

**Display fallback: an "Independent" label, not blank space, wherever company would
render.** `MentorsView.svelte`'s directory card (`{mentor.headline} · {mentor.company_name}`)
and `MentorProfileEditor.svelte`'s existing-profile view (`{profile.company_name ||
profile.company_slug}`) both currently assume a non-empty value. Alternative considered:
omit the company segment entirely for a company-less mentor (no dangling separator, but
also no separator — `{headline}` alone, with nothing marking that "no company" was a
deliberate choice rather than a data gap). Rejected: the sync target for this survey
already found `MentorsView.svelte` uses ` · ` unconditionally, and a reader comparing two
cards side by side — one with a company, one blank — should see that the blank one is
INTENTIONALLY independent, not a profile with a missing company. "Independent" says that
in one word.

## Risks / Trade-offs

- **A company-less mentor is indistinguishable from one whose profile predates this
  change and simply never got a company backfilled** → Moot: the migration adds no new
  rows and never NULLs an existing `company_slug`, so every row that exists today keeps
  its company. Every company-less mentor from this point on chose it.
- **A moderator reviewing the pending queue loses the "which company vouches for this
  person" signal for a company-less submission** → Accepted, not mitigated: that signal
  was always advisory (see the existing "approved referral offer... marks the
  corroborating referral offer as evidence" scenario, which already handles "no
  corroboration" by simply not marking anything) and an independent mentor has no
  employer to vouch via in the first place.

## Migration Plan

Single additive migration, no code deploy ordering constraint beyond the repo's usual
"migrate before code that reads new schema" rule — and here the OLD constraint (NOT NULL)
is only relaxed, so old code reading the column continues to work unchanged even if the
migration runs first. Rollback is the same `ALTER TABLE ... SET NOT NULL` in reverse, safe
only once no company-less row exists (true until this change's frontend ships).

**Code rollback, not just schema rollback, has a narrow exposure worth naming.** The
pre-change binary's generated code declares `CompanySlug` as a plain, non-nullable Go
`string` on `Mentor`, `GetMentorBookingRow`, and `ListBookingsBySeekerRow`. If that binary
is rolled back (an ordinary incident-response move, independent of any schema rollback)
after at least one company-less mentor row exists, scanning that row's NULL
`company_slug` into a non-nullable `string` is a pgx scan error — which fails the whole
query it occurs in, not just that one row's display. In practice this means the directory
listing, the pending queue, or a booking list that happens to include a company-less
mentor would 500 entirely under the old binary until either it rolls forward again or the
schema rollback (above) also runs. Not mitigated here — the window is short and self-heals
on the next forward roll — but worth knowing before reaching for a binary rollback as the
first response to an unrelated incident while this change is live.
