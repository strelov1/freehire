## Context

Full design background and the competitor research behind it lives in
`docs/superpowers/specs/2026-08-10-talent-network-profile-visibility-design.md`
(approved). This document translates that into implementation decisions.

freehire has no employer-facing surfaces today — no employer accounts, no
recruiter search. This change only builds the candidate-facing half: a
visibility setting and a public page. Source data already exists and needs no
new parsing: `users.resume_structured` (JSONB, shape defined in
`internal/resumeextract/structured.go` as `Structured`, with an existing
contact-stripped `Structured.Professional()` projection) and `user_profiles`
(specializations, skills, location_preferences).

`internal/pii` was considered and rejected as a building block: it redacts
spans in prose text, not structured JSON fields, so it doesn't apply to
rendering a structured CV.

## Goals / Non-Goals

**Goals:**
- Let a candidate opt into three states — Off / Public / Anonymous — from
  `my/profile`.
- Serve a public, unauthenticated page per candidate at a stable, unguessable
  URL, rendering content per the selected state.
- Anonymous mode specifically protects against the candidate's current
  employer recognizing them (the stated primary fear), not just general
  identity.

**Non-Goals:**
- Employer accounts, search, or browse (no consumer of this page exists yet
  beyond a candidate manually sharing their own link).
- Intro-request flow, contact reveal, or any consent-gated disclosure.
- Per-viewer / per-company visibility rules (e.g. a blocklist of specific
  companies) — the mode is one global setting for the whole profile.
- Any monetization.
- Showing raw email/phone anywhere on the public page, in either mode — the
  page is scrapeable by definition, and contact initiation is deferred to a
  future intro-request flow.

## Decisions

**Single enum column vs. new table.** Add `talent_network_visibility` as an
enum column (`off` / `public` / `anonymous`, default `off`) directly on
`users`, alongside the existing `resume_*` columns, rather than a new table.
Rationale: it's a single scalar flag with no independent lifecycle, identical
in shape to how the codebase already treats resume-related state as columns
on `users`. Alternative considered: a `talent_network_profiles` table —
rejected as premature; nothing else needs to hang off a separate row yet, and
one can be introduced later (e.g. when intro-requests need their own table)
without migrating this column.

**Opaque ID for the public URL.** Reuse the existing opaque-ID convention
used by other public routes (see `hire-opaque-ids-uuid-swap` precedent) —
never expose the sequential `users.id` in the URL. If users don't already
carry a suitable opaque identifier, mint one (e.g. a UUID column) scoped to
this feature rather than repurposing an unrelated identifier.

**Anonymous-mode masking is `Professional()` plus one extra step.**
`resumeextract.Structured.Professional()` already strips name, email, phone,
and links. This change adds exactly one more transform on top: the newest
entry in the `experience` slice has its `company` field replaced with a
generic label (e.g. `"Current employer"`); older entries pass through
unmodified. Rationale: masking the entire work history was considered and
rejected as over-broad — the risk being mitigated is specifically the current
employer recognizing the candidate, not the candidate's career history in
general, which is useful signal for anyone the candidate does share the link
with.

**Public mode still strips contact info.** Even though `public` mode shows
name and photo, it uses the same contact-stripped base as `anonymous`
(no email/phone/links) — because the page is unauthenticated and publicly
reachable, and contact info displayed there would be scraped by bots
regardless of the candidate's intent to be identifiable.

**404, not 403, for `off` and not-found.** A disabled profile and a
nonexistent one return the identical 404 response — the route must not leak
whether an ID corresponds to a real, currently-hidden profile.

**Missing/empty CV does not block enabling the toggle.** A candidate can
switch visibility on before finishing their CV; the page renders with an
empty experience/skills section rather than erroring. Rationale: decoupling
"opted in" from "has a complete profile" avoids a forced ordering that isn't
otherwise enforced elsewhere in the product.

## Risks / Trade-offs

- **[Risk]** A candidate mistakenly leaves visibility on `public` and later
  wants it hidden retroactively from someone who already saved the link →
  **Mitigation**: none possible once a URL has been shared (inherent to any
  shareable-link design); document this clearly in the toggle's UI copy so
  the trade-off is visible before the candidate enables it. No mitigation
  work is in scope for this change beyond that copy.
- **[Risk]** Anonymous mode's single "mask most-recent employer" rule doesn't
  cover a candidate with multiple concurrent roles or a very recently-ended
  role still readable as "current" by context → **Mitigation**: accept this
  limitation for slice 1; the design spec explicitly scoped the masking rule
  to the newest `experience` entry rather than attempting to infer "current"
  semantically.
- **[Risk]** Public unauthenticated pages are an SEO/scraping surface with no
  rate limiting by default → **Mitigation**: out of scope for this change;
  flag for follow-up if abuse is observed after launch (consistent with the
  project's "no MVP shortcuts, but no speculative infra either" principle —
  don't build rate limiting for a threat that hasn't materialized).

## Migration Plan

- New forward-only migration adding the `talent_network_visibility` enum
  column (and opaque-ID column, if one doesn't already exist for `users`)
  to `users`, default `off`. No backfill needed — every existing user
  defaults to `off`, matching current behavior (no such page exists today).
- No rollback complexity: the column and route are additive; reverting is a
  follow-up migration dropping the column, safe at any time since nothing
  else depends on it within this change's scope.

## Open Questions

- Exact opaque-ID scheme to reuse (or mint) — resolved during task breakdown
  by inspecting how `companies`/`jobs` slugs are generated, per
  `internal/db/AGENTS.md` conventions.
