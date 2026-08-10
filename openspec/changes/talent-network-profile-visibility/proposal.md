## Why

freehire wants to eventually let recruiters discover candidates instead of
candidates only applying job-by-job. Before building the recruiter-facing half
(employer accounts, search, intro requests), candidates need a way to opt in
and control how much of their identity is exposed. This change ships that
standalone first slice: a visibility toggle and a shareable public profile
page, independently useful (a candidate can already send the link to a
recruiter themselves) and small enough to ship without the rest of the system
existing yet.

## What Changes

- New per-candidate setting: `talent_network_visibility` — `off` (default),
  `public`, or `anonymous`.
- New toggle control on the existing `my/profile` page to set this.
- New public, unauthenticated page at a stable, unguessable URL per candidate,
  rendering their profile according to the selected mode:
  - `off` → 404 for everyone but the owner (who manages it from `my/profile`).
  - `public` → name, photo, work history, skills. No email/phone/links.
  - `anonymous` → same as `public` minus name and photo, plus the most recent
    `experience` entry's company name is masked (older entries are shown
    as-is).
- No new parsing: page content is sourced entirely from the existing
  `users.resume_structured` (via `internal/resumeextract`) and `user_profiles`.

Explicitly deferred (not in this change): employer accounts, employer-side
search/browse, the intro-request flow, per-company visibility rules, any
monetization.

## Capabilities

### New Capabilities
- `talent-network-profile`: candidate opt-in visibility setting and the
  public, unauthenticated profile page that renders according to it.

### Modified Capabilities
(none — purely additive; no existing capability's requirements change)

## Impact

- **DB**: new migration adding `talent_network_visibility` enum column to
  `users` (or a dedicated table, per design.md) plus whatever opaque-id scheme
  the design settles on for the public URL.
- **Backend**: new unauthenticated handler/route serving the public profile
  page; extension of the existing `me_profile` handler (or a new sibling) to
  read/write the visibility setting.
- **Frontend**: new toggle UI in `web/src/routes/my/profile`; new public route
  rendering the profile page.
- **No changes** to `internal/pii`, `internal/cv`, employer-facing surfaces
  (none exist yet), or any existing public route.
