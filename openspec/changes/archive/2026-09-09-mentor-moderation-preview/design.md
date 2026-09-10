## Context

See proposal.md - Why. Relevant existing shapes:

- `mentorResponse` (`internal/api/handler/mentorship.go`) is the one wire shape shared by
  the public read, the owner's cabinet read, and the moderator's queue read —
  `toMentorResponse` → `toModeratorMentorResponse` (+status/paused) →
  `toOwnMentorResponse` (+meeting link/session logistics). `CreatedAt` is on
  `mentorship.Profile` and selected by the SQL, but no `to*Response` function sets it.
- `GetMentorPhoto` resolves the profile via `Service.PublicProfile`, which reads through
  `PublishedProfileBySlug` — `status = 'approved' AND NOT paused` in the query itself.
  There is no existing by-id, any-status profile read on `Service` (only on
  `Repository`, used internally by booking/withdrawal code paths).
- The moderation queue (`ListPendingMentorProfiles`) already returns everything a
  preview needs except the photo: name, headline, company, bio, topics, languages,
  timezone, session_minutes, show_photo.

## Goals / Non-Goals

**Goals:**
- A moderator can see when a profile was submitted and preview its opted-in photo.
- The public photo/profile routes keep refusing an unapproved profile exactly as today.

**Non-Goals:**
- No shared component with the public `/mentors/[slug]` page — the moderation preview is
  a separate, purpose-built rendering (no booking widget, no rating display worth
  showing on a profile with zero reviews).
- No change to what the public directory, public profile read, or public photo route
  return — this only adds a moderator-scoped path.
- No pagination/history of past decisions on a profile — out of scope for "what to
  review right now."

## Decisions

**`created_at` is added to the shared `mentorResponse` struct field, populated only by
`toModeratorMentorResponse` and `toOwnMentorResponse` — never by the plain
`toMentorResponse` the public routes use.**
Adding the field to the shared struct (rather than a queue-specific wrapper type) mirrors
how `status`/`paused` already work: one struct, `omitempty`, populated selectively by the
function that builds a given audience's view. `toMentorResponse` never sets it, so the
public directory and public profile read are unaffected without needing a guard.

**A new `Service.ProfileForModeration(ctx, id)` wraps `Repository.ProfileByID`
unconditionally — no status filter, no ownership check.**
The route it backs is already behind `mw.moderator`, which is the only access control
this needs; duplicating a check the middleware already makes would be the kind of second
copy that drifts. This mirrors `Service.Decide`, which also takes a bare id under the
same middleware gate.

**The new photo route lives beside the existing moderator routes:
`GET /mentorship/profiles/:id/photo`, not `/me/mentorship/...`.**
The `/me/...` prefix names routes under the CALLER's own cabinet; `/mentorship/profiles`
is already the moderator queue's own namespace (`GET /mentorship/profiles`, `POST
/mentorship/profiles/:id/decide`), keyed by the numeric row id a moderator already reads
off the queue, not by slug (a slug the moderator would have to know is not yet public).

**The frontend preview reuses the queue's already-fetched `PendingMentorProfile`; only
the photo needs a network call.**
Every other field the preview shows (name, headline, company, topics, languages, bio) is
already in the list response — building a second "read one profile" endpoint just to
re-fetch what's already in hand would be a round trip for nothing.

## Risks / Trade-offs

- [A moderator's own photo route duplicates `GetMentorPhoto`'s shape] → Both call the
  same `mentorPhoto(ctx, profile, photos)` helper; only the profile lookup differs
  (`ProfileForModeration` vs `PublicProfile`), so the duplication is the handler
  boilerplate around one shared helper, not the photo-serving logic itself.
- [Two visually different "how does this mentor look" renderings (public page, moderator
  preview) could drift in what fields they show] → Accepted for now per Non-Goals; the
  moderator's job is deciding whether to publish, not proofreading the exact public
  layout, and unifying them would touch the already-shipped public route for a benefit
  this change does not need.

## Migration Plan

No schema change. `created_at` already exists and is already selected by
`ListPendingMentorProfiles`; this only stops the handler from discarding it. Deploys
without any coordination.
