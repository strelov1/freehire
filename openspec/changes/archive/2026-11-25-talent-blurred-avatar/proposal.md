## Why

The Talent Network public card page (`/talent/<handle>`) currently renders a generic
person-silhouette icon for every member, by design — the public projection carries no
photo. A candidate's uploaded headshot exists in the system (`internal/candidate/headshot`,
already used on their private profile) but the anonymous card never shows anything
resembling them. Showing a heavily, irreversibly blurred rendering of that headshot makes
the anonymous card feel like it belongs to an actual person, without exposing a
recognizable likeness — approved by the user (this is a UI/product decision, not an
engineering-driven one) after being shown the existing spec conflict and the weaker
"CSS blur" alternative.

## What Changes

- New public endpoint `GET /api/v1/talent/:handle/photo`: re-verifies membership against
  the database on every request (same freshness guarantee as `GET /talent/:handle`, never
  the cached catalogue snapshot), fetches the member's stored headshot, and serves it
  through a strong, irreversible server-side Gaussian blur. The original image bytes are
  never sent to the client under this route.
- The blur uses a new dependency (`github.com/disintegration/imaging`) for a true Gaussian
  kernel, applied with a large, fixed sigma. Blurring happens on every request, from the
  stored original — no blurred derivative is persisted, since the source image is small
  (512×512 JPEG) and the operation is cheap.
- Every failure reason (handle isn't a current member, member has no headshot uploaded,
  headshot storage unconfigured) collapses to a single 404 — the response must not reveal
  whether a member who happens not to have a headshot differs from a non-member, matching
  the existing `/mentors/:slug/photo` precedent.
- The talent card page (`web/src/routes/talent/[handle]/+page.svelte`) renders an `<img>`
  pointed at the new endpoint in place of the generic icon, falling back to the icon via
  `onerror` — the same try-then-fallback pattern already used for company logos
  (`EntityLogo`). No new field is added to the public `CandidateCard`/`CatalogueMember`
  wire shape; presence of a photo is discovered by the browser, not signaled by the API.
- The page's footer disclaimer is updated to state plainly that an uploaded photo, when
  present, is shown heavily blurred and cannot be recovered in its original form.
- **BREAKING (spec-level, not wire-level)**: amends the `talent-network-catalog`
  capability's requirement that the public projection withholds photo entirely. It still
  withholds the *original* — no route, cached or not, ever serves an unblurred headshot
  publicly — but a new requirement describes the blurred-photo exception explicitly.

## Capabilities

### New Capabilities

None.

### Modified Capabilities

- `talent-network-catalog`: the "public response carries no free text from a CV"
  requirement's photo-withholding clause is narrowed — the original photo is still
  withheld, but a new requirement is added describing that a member's photo, when
  uploaded, is served publicly ONLY as an irreversibly blurred derivative, computed
  server-side on every request, never cached as a separate stored artifact, and gated by
  the same per-request membership re-verification the card route already uses.

## Impact

- `internal/candidate/headshot/` — new `Blur` function (decode → Gaussian blur via
  `imaging.Blur` → re-encode JPEG), reusing the package's existing image pipeline
  conventions (`normalize.go`).
- `internal/candidate/talentnetwork/catalogue.go` — new method resolving a handle straight
  to its owner's user id (for the photo route only), without building the full public
  `CandidateCard` projection.
- `internal/api/handler/talent_catalog.go` — new `GET /talent/:handle/photo` route, same
  rate limiter instance as the other two talent routes.
- `go.mod` — new dependency `github.com/disintegration/imaging`.
- `web/src/routes/talent/[handle]/+page.svelte` — render the photo with icon fallback;
  update the footer disclaimer copy.
- `openspec/specs/talent-network-catalog/spec.md` — delta spec narrowing the photo
  withholding requirement and adding the blurred-photo requirement.
