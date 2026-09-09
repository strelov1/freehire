## Why

Creating a mentor profile at `/my/mentorship/profile` starts from a completely blank
form — name, headline, bio, topics, languages, timezone and company are all typed from
scratch, even though most of that already exists on the candidate's account (résumé,
user profile, experience bank). That friction is a plausible reason the mentor pool
stays small. Separately, a published mentor's public card and page carry no picture at
all — the `mentor-profile` spec deliberately deferred an avatar, noting the account's
CV headshot is a job-search photo a mentor may not want reused here without asking.
Both gaps are addressed together because solving the second is now cheap once we are
already willing to compose the candidate's stored data for prefill — but the photo
still needs the mentor's own explicit consent, since the original deferral's privacy
concern still holds.

## What Changes

- New endpoint `GET /me/mentorship/profile/suggestions`, read-only, composed solely
  from the candidate's résumé, user profile, account and experience bank — never from
  the mentor profile itself, so it answers the same way whether or not one exists. Only
  the create form calls it, and only before a profile exists. Best-effort, per field:
  name, headline,
  bio and languages from the résumé; topics from the user profile's specializations;
  timezone from the account; and a current-employer company suggestion, included only
  on an exact, verified company-catalog match. A missing or unmatched source simply
  omits that field — nothing about this endpoint can fail the create flow.
- The mentor-profile **create** form seeds itself from those suggestions instead of
  blank defaults. Every field stays an ordinary, independently editable input — this is
  a one-time starting point, not a live sync. Editing an already-existing profile is
  unchanged.
- A new `show_photo` field on the mentor profile — an explicit, off-by-default opt-in,
  editable like any other profile field. A mentor who turns it on has their account's
  stored CV headshot served on their public directory card and profile page, through a
  new narrow public endpoint scoped to that one approved, opted-in profile. A mentor who
  never opts in changes nothing about their existing CV-photo privacy, and nobody's
  headshot becomes reachable through this endpoint without that mentor's own opt-in.
- Public directory card and profile page render the photo when `show_photo` is true and
  a photo exists, falling back cleanly (no broken image) otherwise.

## Capabilities

### New Capabilities
- `mentor-profile-prefill`: read-only, best-effort suggestions for a mentor's own
  not-yet-created profile, composed from their résumé, user profile, account and
  experience bank.

### Modified Capabilities
- `mentor-profile`: adds the `show_photo` opt-in field and the public photo endpoint
  the spec's "An avatar is NOT part of this requirement" section explicitly deferred,
  now resolved with mentor-controlled consent rather than automatic reuse.

## Impact

- **Backend**: `internal/engage/mentorship` (profile fields, validation, repository
  params), a migration adding `mentors.show_photo`, `internal/api/handler`
  (`mentorship_write.go`/`mentorship.go` for the new field and the new public photo
  route; a new narrow aggregator, likely `mentorship_suggestions.go`, composing
  `internal/candidate/resume`, `internal/candidate/experience`,
  `internal/identity/userprofile`, `internal/identity/accounts`, and the existing
  company-lookup behind mentor-profile creation).
- **Frontend**: `web/src/lib/components/MentorProfileEditor.svelte` (prefill + opt-in
  checkbox), `web/src/lib/components/MentorsView.svelte` and
  `web/src/routes/mentors/[slug]/+page.svelte` (avatar rendering), `web/src/lib/types.ts`
  and `web/src/lib/api.ts` (new wire types and the suggestions call).
- **No changes** to booking, availability, moderation, or the withdraw/pause flows.
