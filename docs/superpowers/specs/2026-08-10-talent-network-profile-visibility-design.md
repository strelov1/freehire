# Talent Network — profile visibility toggle + public page (slice 1)

## Context

Long-term idea: a "talent network" where recruiters can discover candidates and
request an introduction, instead of candidates only applying job-by-job. Before
committing to that full flow, this slice ships the smallest independently useful
piece: a candidate can opt in, pick how much of their identity to expose, and get
a shareable public page. No employer accounts, no search, no intro-request flow —
those are deliberately deferred to a later change.

## Competitor research (summary)

Surveyed: hiring.cafe, lemon.io, Toptal, Arc.dev, Braintrust, Hired.com (dead),
honeypot.io (dead), Wellfound, interviewing.io, Pallet/Gem/Beamery, Himalayas.app.

Two findings shaped this design:

1. **Identity handling falls into two families.** Either fully open profiles
   (Himalayas, Wellfound, Braintrust, honeypot.io — a visibility toggle at most,
   no redaction) or redact-then-reveal-on-consent (hiring.cafe, interviewing.io —
   identity hidden until the candidate consents to a specific introduction).
   No competitor combines both as a candidate-chosen mode; that combination is
   this project's actual idea.
2. **Revenue model predicts survival, independent of the identity model.**
   Platforms whose revenue was 100% contingent on a completed hire (honeypot.io,
   Hired.com, Pallet's original model, interviewing.io pre-2020) died or pivoted
   when hiring demand contracted, because reaching out via a talent network is
   discretionary spend for an employer — the first thing cut in a downturn.
   Platforms with revenue independent of hire completion (Wellfound seats,
   Himalayas candidate subscriptions, hiring.cafe's undisclosed but manually-sold
   pricing) survived. This project defers monetization entirely (free for
   everyone at launch) rather than bake in a per-hire fee — consistent with
   `hire-monetization-strategy`, which already ranked a full marketplace as the
   least defensible rung.

## Scope of this slice

In scope:
- A per-candidate visibility setting: **Off** (default) / **Public** / **Anonymous**.
- A public, unauthenticated page at a stable, unguessable URL rendering the
  profile according to the selected mode.
- Toggle UI on the existing `my/profile` surface.

Out of scope (future slices, not designed here):
- Employer accounts, employer-side search/browse.
- Intro-request flow (candidate accept/decline, contact reveal on consent).
- Per-company visibility (e.g. hiring.cafe's "Hidden Companies" blocklist) —
  this slice is one global mode for the whole profile, not per-viewer.
- Any monetization.

## Data model

New enum column, `users.talent_network_visibility`, one of `off` / `public` /
`anonymous`, default `off`. No new table — this is a single flag on the existing
user row, alongside the existing `resume_*` columns.

The page needs a stable opaque identifier, consistent with the existing pattern
in `hire-opaque-ids-uuid-swap` — never expose the sequential `users.id`. Reuse
whatever opaque-id scheme other public routes (`companies/[slug]`, `jobs/[slug]`)
already use for this entity, or mint one if users don't already have one.

## Page content per mode

Source data: `users.resume_structured` (via `internal/resumeextract`) and
`user_profiles` (specializations, skills, location_preferences) — both already
populated by existing flows; nothing new to parse or store.

**Off** — the route returns 404 regardless of who asks (including the owner
signed out; the owner sees their own page while signed in via `my/profile`,
not this route).

**Public** — renders `resumeextract.Structured` with `email`, `phone`, and
`links` fields stripped (the existing `Professional()` projection already
does exactly this minus the name). Name and photo shown. No raw contact
info anywhere on the page, by design — this is a public URL, crawlable by
bots/scrapers, and initiating contact is explicitly deferred to a future
intro-request flow rather than exposed here.

**Anonymous** — renders `resumeextract.Structured.Professional()` (already
strips name/email/phone/links) plus one additional masking step this slice
adds: the most recent entry in `experience` has its company name replaced
with a generic label (e.g. "Current employer") instead of shown as-is. Older
`experience` entries are shown unmasked — hiding the whole history was
considered and rejected as over-broad; the risk this addresses is specifically
a candidate's current employer recognizing them, not their career history in
general.

## Error handling

- Visibility `off`, or user/profile not found → 404, identical response shape
  to any other missing public resource (no leak of "exists but hidden" vs
  "doesn't exist").
- Malformed/missing `resume_structured` for a user who has turned the toggle
  on → the page renders with an empty experience section rather than erroring;
  a user can enable Talent Network before finishing their CV.

## Testing

- Unit: the masking function for anonymous mode (most-recent-experience-entry
  redaction) — verify only the newest entry is touched, older ones pass through.
- Handler test: all three visibility states return the expected HTTP status and
  field set (off → 404; public → no email/phone/links present in response body;
  anonymous → no name/email/phone/links present, most recent company masked).
- No integration/DB test changes anticipated beyond a new migration for the
  enum column — follows the existing migration conventions in
  `internal/db/AGENTS.md`.
