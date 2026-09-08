## Context

`internal/engage/mentorship` (layer 7) already sits above `internal/candidate` (layer 4)
and `internal/identity` (layer 3) in the layering table, so it could import either
directly. It does not today, and its `Profile`/`ProfileInput`/`Service` know nothing
about résumés, experience or user profiles. `internal/api/handler` (layer 8) already
composes exactly this shape of read for an unrelated feature: `me_profile.go`'s
`profileHandlers` reads `userprofile.Service`, a narrow `structuredResumeReader` slice
of `*resume.Store`, and a `candidateProfiler` slice of `*experience.Store`, each
consulted best-effort and independently, to answer `GET /me/profile`. Company existence
is a single `db.Queries.CompanyExists(ctx, slug) (bool, error)` call — the create flow
today instead relies on the `company_slug` foreign key rejecting an unknown company at
insert time (`internal/engage/mentorship/repository.go`), which the read-only
suggestions endpoint cannot reuse. Headshot bytes come from
`headshot.Store.Get(ctx, userID) ([]byte, error)`, already used by
`internal/api/handler/photo.go`'s `GetPhotoImage` for the caller's own photo.

## Goals / Non-Goals

**Goals:**
- Compose mentor-profile suggestions the same way `me_profile.go` composes its `cv`
  block: narrow reader interfaces, best-effort, independent per field.
- Serve a mentor's own headshot publicly only through their own explicit,
  per-profile opt-in, reusing the account's single stored copy.

**Non-Goals:**
- No new photo storage, resizing, or caching layer — `show_photo` gates read access to
  the existing `headshot.Store` object, nothing else.
- No fuzzy or LLM-assisted company matching — only the exact slug match the create flow
  already enforces via the foreign key.
- No change to how `internal/engage/mentorship` validates or stores a profile beyond
  the one new `show_photo` column; the domain package still knows nothing about résumés
  or experience.

## Decisions

**Composition lives in `internal/api/handler`, not in `internal/engage/mentorship`.**
Mirrors `me_profile.go` exactly: a new `mentorship_suggestions.go` defines a narrow
`mentorSuggestionSources` interface (résumé, user profile, account timezone,
experience-bank current employment, company-existence check) so the handler is
unit-testable without a database, and keeps the mentorship domain package free of
`candidate`/`identity` imports it would otherwise never need. Alternative considered:
add a `SuggestProfile` method on `mentorship.Service` that takes those stores as
arguments — rejected because it would make the domain package's constructor depend on
four unrelated blocks' types for a feature that is pure read composition, exactly the
shape `me_profile.go` already chose to keep out of `userprofile.Service`.

**The company suggestion resolves through `normalize.CompanySlug` + `CompanyExists`,
not a new lookup path.** `normalize.CompanySlug` is the one legal-form vocabulary the
codebase uses for this; running a bank's free-text current employer through it and then
checking `db.Queries.CompanyExists` matches exactly what the create flow's foreign key
would accept, so a suggested company is guaranteed to submit successfully. No new
company-matching logic is introduced.

**`show_photo` is a profile column, not a derived flag.** It needs to survive
independent of whether a headshot happens to exist at the moment — a mentor who opts in
before uploading a CV photo, or removes their CV photo later, keeps their stated
preference rather than having it silently reset. The public photo route computes "serve
or 404" at request time from `show_photo AND status == approved AND !paused AND
headshot exists`, so no denormalized "has photo" flag needs to stay in sync.

**The photo route reuses `GetMentor`'s resolution and predicate, not a copy of it.**
`GetMentor` already turns a slug into a `Profile` and enforces
`approved && !paused`; the new `GET /mentors/:slug/photo` calls the same resolution
before checking `ShowPhoto`, so the two routes cannot drift on what "publicly readable"
means (the existing `mentor-profile` spec already calls this out as a reason the
directory-entry-point feature reuses the directory query instead of a second
"has a mentor?" endpoint — the same reasoning applies here).

**Every non-serving reason for the photo route maps to the same 404** (no photo, not
approved/paused, opted out, storage unconfigured) rather than distinguishing them in
the response. A mentor's opt-out and a mentor's absent photo must be indistinguishable
from outside — otherwise the response itself would leak whether a mentor who opted out
has a CV photo at all, a narrower but real version of the privacy concern the opt-in
exists to address.

**Suggestions are a separate endpoint, not a query parameter on `GET
/me/mentorship/profile`.** That existing route already means "my profile, or null" one
way; overloading it with "or, if null, suggestions" would make the response shape
depend on whether a profile exists, which the frontend would need to branch on anyway.
A dedicated `GET /me/mentorship/profile/suggestions` keeps `GetMyMentorProfile`
unchanged and the new endpoint independently cacheable/skippable by a caller that
doesn't need it (e.g., the edit path never calls it).

## Risks / Trade-offs

- **A résumé/experience-bank/user-profile read failure must not block profile
  creation.** → Every source is read independently inside the aggregator and a failing
  or absent source simply omits its field(s), exactly as `me_profile.go`'s
  `structuredCV`/`derivedLocation` degrade; the suggestions endpoint itself has no
  non-2xx path tied to source data.
- **`CompanyExists` adds one query to a form-load path.** → It is a single indexed
  lookup on an already-normalized slug, called at most once per suggestions request,
  which itself only fires once (on mount, before a profile exists) — negligible next to
  the résumé/experience-bank reads already in the same request.
- **A mentor who opts in, then deletes their CV headshot, leaves `show_photo` stuck
  true with nothing to serve.** → Accepted: the photo route already 404s cleanly on a
  missing headshot regardless of the flag, so this degrades to "no photo shown," not an
  error, and re-uploading a headshot makes it reappear with no further action.
