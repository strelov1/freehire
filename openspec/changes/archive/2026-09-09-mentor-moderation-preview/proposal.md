## Why

A moderator deciding on a pending mentor profile currently sees only name, headline,
company, bio, topics, languages and a referral-offer flag (`GET
/api/v1/mentorship/profiles`) — no submission date, and no way to see how the profile
will actually look once published. The public mentor page cannot be reused for this: it
fetches through the public `GET /api/v1/mentors/:slug` endpoint, which 404s for any
non-approved profile by design (so a visitor cannot enumerate the moderation queue), and
its photo endpoint has the same gate. A moderator is left approving or rejecting a
profile they cannot actually preview.

## What Changes

- The moderator-facing response for a pending profile now carries `created_at`, already
  read by the SQL query and carried on the domain type, but previously dropped before
  serialization.
- A new moderator-only route, `GET /mentorship/profiles/:id/photo`, serves a pending
  mentor's opted-in photo by resolving the profile through a new
  `Service.ProfileForModeration` (unconditional lookup by id) instead of the public-only
  `PublicProfile`.
- The moderation queue UI shows each profile's submission date and offers a "Preview"
  toggle rendering a public-card-style view (photo, name, headline @ company,
  topics/languages, bio) from data the queue already holds, plus the new photo route.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `mentor-profile`: a moderator's read of a pending profile now includes its submission
  date and a way to preview its would-be-public photo.

## Impact

- `internal/api/handler/mentorship.go`: `mentorResponse`/`toModeratorMentorResponse`
  gains `created_at`; new `GetPendingMentorPhoto` handler + route.
- `internal/engage/mentorship/{profile,service}.go`: new `Service.ProfileForModeration`.
- `web/src/lib/api.ts`, `web/src/lib/components/MentorReviewView.svelte`: submission
  date display, preview toggle, moderator photo fetch.
- No schema change, no migration: `created_at` already exists and is already selected by
  `ListPendingMentorProfiles`.
