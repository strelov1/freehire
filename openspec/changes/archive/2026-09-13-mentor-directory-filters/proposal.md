## Why

The public mentor directory narrows only by company, topic and language. A seeker who
already knows roughly who they want — a specific name, a seniority level, or simply
someone who hasn't been booked out yet — has no way to say so and has to scan the whole
list by hand. The directory is small today, but it grows by hand-onboarding and the gap
only widens.

## What Changes

- **Free-text search**: a new `q` directory parameter matches a mentor's name or
  headline (case-insensitive substring), reusing the ILIKE pattern the company search
  and mail inbox search already use. No schema change.
- **Seniority**: a mentor may optionally state their seniority level on their profile,
  using the platform's existing closed vocabulary (`internal/dict/vocab.SeniorityValues`
  — junior through c-level, intern included) rather than a new one. The directory gains
  a `seniority` parameter to narrow by it. A profile that leaves it unset is excluded by
  the filter but never excluded from the unfiltered directory.
- **A "no reviews yet" toggle**: a `no_reviews` directory parameter narrows to mentors
  whose review count is zero, from the rating aggregate the directory query already
  joins. No new join or column.
- All three follow the directory's existing "empty means unfiltered" rule and the
  dropped-filter convention: a parameter the endpoint does not read is reported in
  `meta.ignored_params`, never silently applied or silently dropped.

## Capabilities

### Modified Capabilities
- `mentor-profile`: adds the optional `seniority` field to what a profile carries, and
  extends "The public directory lists approved mentors and can be narrowed" with the
  three new parameters (`q`, `seniority`, `no_reviews`) alongside the existing three.

## Impact

- **Backend**: `internal/engage/mentorship/profile.go` (`Profile.Seniority`,
  `ProfileInput.Seniority`, `DirectoryFilter.Query`/`Seniority`/`NoReviewsOnly`,
  validation against `vocab.SeniorityValues`), `internal/engage/mentorship/repository.go`
  (threading the new fields through `CreateProfile`/`UpdateProfile`/
  `ListPublishedProfiles`/`profileFromRow`), `internal/platform/db/queries/mentorship.sql`
  (a new `mentors.seniority` column read/written by `CreateMentorProfile`/
  `UpdateMentorProfile`, and the three new `ListPublishedMentors` predicates), a new
  additive migration, `internal/api/handler/mentorship.go` (`mentorResponse.Seniority`,
  the three new query params, `knownMentorParams`),
  `internal/api/handler/mentorship_write.go` (`profileRequest.Seniority`).
- **Frontend**: `web/src/lib/mentorship.ts` (`MentorFilters`/`MENTOR_FILTER_KEYS`
  extended, `mentorFilterOptions` derives the seniority dropdown's values the same way
  it already derives topics/languages), `web/src/lib/types.ts` (`Mentor.seniority?`,
  `MentorProfileInput.seniority`), `web/src/lib/components/MentorsView.svelte` (a search
  input, a seniority `<select>`, a "no reviews yet" checkbox, alongside the three
  existing selects), `web/src/lib/components/MentorProfileEditor.svelte` (an optional
  seniority `<select>` on the create/edit form, mirroring the existing plain-`<select>`
  pattern the form already uses — no design-system dropdown component is used here
  today).
- **No changes** to booking, availability, moderation, ranking/sorting of the directory,
  or pricing (this platform's mentorship is free).
