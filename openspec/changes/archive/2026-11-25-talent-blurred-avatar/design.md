## Context

Three product decisions were made by the user before this design, and are not
reopened here: (1) show a blurred photo, not the generic icon, when one exists; (2)
automatic for every member with a stored headshot — no per-member opt-in, unlike the
Mentors directory's `ShowPhoto` flag; (3) blur server-side, computed per request from
the original, never a stored second derivative; and a fourth, on the blur mechanism:
(4) a true Gaussian blur via a new dependency, not the reuse-what-exists
downsample/upsample trick.

`internal/candidate/headshot.Store.Get(ctx, userID int64) ([]byte, error)` already
returns a stored headshot's raw bytes — always a 512×512 JPEG, since `Normalize`
re-encodes every upload on write (`normalize.go:47-80`). `internal/candidate/talentnetwork.Catalogue.ByHandle`
already re-verifies membership against Postgres on every call, via
`GetTalentNetworkMemberByHandle`, whose `WHERE` clause is the definition of "current
member" this whole capability uses: `talent_handle = <handle> AND
talent_network_visibility <> 'off' AND resume_uploaded_at IS NOT NULL AND
resume_structured_uploaded_at = resume_uploaded_at` (`users.sql:534-547`). That query
does not select `u.id`, and `ByHandle` only ever builds a public `CatalogueMember` —
neither is reusable as-is for a route that needs a bare user id to look up a headshot.

`internal/api/handler/mentorship.go`'s `GET /mentors/:slug/photo` is the one existing
precedent for a public, unauthenticated route serving a stored headshot: it resolves
through the same predicate its sibling profile route uses (`PublicProfile`), collapses
every failure to one 404, and shares the profile route's rate limiter rather than
getting its own.

## Goals / Non-Goals

**Goals:**
- Serve a heavily, irreversibly blurred rendering of a member's stored headshot from a
  new public route, computed fresh on every request.
- Never let the original image bytes reach any public route or cache.
- Keep the existing `CandidateCard`/`CatalogueMember` wire contract untouched — no new
  field, no OpenAPI/contract regeneration for the card shape.
- Match this capability's existing freshness and privacy conventions (DB
  re-verification per request, indistinguishable-404 failure collapsing, shared rate
  budget) rather than inventing new ones.

**Non-Goals:**
- No per-member opt-in/opt-out control (explicit user decision).
- No stored blurred derivative, no new blobstore key, no backfill.
- No change to the private `/me/photo/image` route or the Mentors directory's photo
  route.
- No avatar upload/crop UX changes — this only concerns how an existing headshot is
  *read* publicly.

## Decisions

**A new minimal sqlc query, not a reuse of `GetTalentNetworkMemberByHandle`.** The
existing query does not select `u.id` and additionally joins `user_profiles` for
`specializations`, which the photo route has no use for. Adding
`GetTalentNetworkMemberUserIDByHandle` — same `WHERE` predicate, `SELECT u.id` only, no
join — keeps the photo path from ever touching or constructing a `CatalogueMember`, so
the "whitelist projection" package (`talentnetwork/AGENTS.md`) never gains a code path
that assembles a public struct for a purpose other than the public JSON response.
Alternative considered: extend `ByHandle` to also return the row's user id — rejected
because it would put an internal-use id on a method whose only current callers want the
public projection, inviting a future caller to leak it by accident.

**New `Catalogue` method, not a handler-local query call.** `internal/api/handler`
already imports `internal/candidate/talentnetwork`; the membership predicate belongs in
the package that owns the concept of "current member," not duplicated in the handler.
`func (c *Catalogue) HeadshotOwner(ctx context.Context, handle string) (userID int64,
err error)` — returns `ErrNotFound` (the same sentinel `ByHandle` uses) for a malformed
handle or a no-rows result, so the handler's error mapping is identical for both the
card route and the photo route.

**Blur lives in `headshot`, not in a new package.** `headshot.Blur(data []byte) ([]byte,
error)` — decode (the stored bytes are always JPEG, but decoding via the standard
`image.Decode` dispatch costs nothing and avoids a second assumption about the input),
apply `imaging.Blur(img, sigma)`, re-encode with the package's existing
`jpeg.Encode(..., &jpeg.Options{Quality: jpegQuality})` call. This keeps every
image-pixel operation on a headshot in the one package that already owns
`Normalize`, rather than splitting "how we transform a headshot" across two packages.
`sigma = 30` on a 512×512 image: large enough that no facial feature (eyes, mouth,
hairline edge) survives as a distinct shape — verified visually during implementation,
not just chosen by formula, since "unrecognizable" is a visual judgment, not a
numerically derivable threshold. Kept as an unexported package constant, not an env var
— there is no deployment reason to tune this per environment, and a configurable
"how anonymous is this" knob is the wrong kind of flexibility to offer.

**New dependency: `github.com/disintegration/imaging`.** Chosen over hand-rolling a
Gaussian kernel (more code, more surface to get subtly wrong on edge pixels) and over
`github.com/anthonynsimon/bild` (less widely used, similar API surface, no material
advantage here). `imaging.Blur` operates on `image.Image` via `image/draw`-compatible
conversion, so it composes directly with the existing decode/encode calls without
adapting either side.

**On-the-fly, no stored derivative** (per the user's explicit decision). The operation
is cheap — decode a ~40 KB 512×512 JPEG, blur, re-encode — comfortably sub-request-budget
even under the shared `talentCatalogLimiter` (120/min/IP), and avoids: a new blobstore
key scheme, a backfill for existing headshots, and a second place a stored image can
drift from the pointer that names it.

**Route and failure collapsing.** `GET /api/v1/talent/:handle/photo`, registered in
`talentCatalogHandlers.register` alongside `/talent` and `/talent/:handle`, sharing the
same `limiter := talentCatalogLimiter(mw.throttler)` instance (matching how
`mentorship.go` shares one limiter across all four of its public routes). Handler flow:
`HeadshotOwner(handle)` → on `ErrNotFound`, `fiber.NewError(404)`; on success,
`headshot.Get(userID)` → on `ErrNotStored` or `ErrStorageDisabled`, the SAME
`fiber.NewError(404)` (never a 501, unlike the private `/me/photo/image` route — a
public caller must not learn "storage is down" as distinct from "this member has no
photo," the same collapsing `mentorship.go`'s `errMentorPhotoNotFound` already performs)
→ `headshot.Blur(data)` → `image/jpeg`, `X-Robots-Tag: noindex`,
`Cache-Control: private, max-age=60` (matching the card route's own header choice,
deliberately `private` rather than `public` so a shared/CDN cache never retains a copy
past a member's possible departure, even though the origin re-verifies every time).

**Frontend: `<img>` + `onerror` fallback, no `has_photo` field.** The talent page tries
`<img src="/api/v1/talent/{handle}/photo">` unconditionally and falls back to today's
generic-icon `<div>` on load failure — the same try-then-fallback shape `EntityLogo`
already uses for company logos elsewhere in this codebase. This was chosen over adding
a boolean to `CatalogueMember` specifically to avoid growing the public card contract
for a concern the browser can resolve itself with one request.

## Risks / Trade-offs

- **A page view for a photo-less member (likely most members, initially) always costs
  one extra request that 404s.** Mitigated: the DB lookup for a 404 is a single indexed
  query, no blob fetch, no blur — cheap relative to the request budget, and consistent
  with the existing logo-fallback pattern's own cost profile.
- **Gaussian blur is theoretically not information-destroying the way a heavy
  downsample would be** (a large enough compute budget could attempt approximate
  deconvolution against a *known* fixed kernel). Accepted trade-off: the user explicitly
  chose this mechanism after the downsample/upsample alternative was presented as the
  more theoretically robust option. `sigma = 30` on a photo that started at only
  512×512 (itself already a lossy, resampled derivative of whatever was uploaded, per
  `Normalize`) leaves very little signal for any reconstruction attempt to work with in
  practice.
- **`private, max-age=60` still allows the requesting browser itself to cache the image
  for up to a minute.** Matches the card route's own choice; a tighter value was not
  requested and would only add requests for the same real-world exposure window.

## Migration Plan

Additive: new route, new sqlc query (regenerated via `make sqlc`), new package
function, new dependency, one frontend file changed, one spec file amended. No data
migration — every stored headshot is already the exact bytes this route will blur.
Rollback is reverting the branch; nothing is written that must be undone.
