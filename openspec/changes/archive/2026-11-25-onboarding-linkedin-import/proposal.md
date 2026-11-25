## Why

The `/onboarding` wizard's first step asks for a CV, and a CV is the only way to pre-fill the steps behind it. A user who has no PDF to hand — the common case for someone who keeps their history on LinkedIn and nowhere else — has nothing to give it, so they skip step 1 and hand-pick role, skills, level and location from empty pickers. Pasting a profile URL is the one thing that user always can do.

A feasibility spike (2026-09-03) established what a public LinkedIn profile actually yields to an anonymous fetch, and the answer is narrower than it looks: the page returns 200 with an `application/ld+json` `Person` node, but LinkedIn masks most of its text with asterisks that preserve string length — 479 masked strings on the profile tested, covering **every job title and every experience description**. What survives unmasked is the headline, the address, the languages, the photo, the first company and the first school. Fetching with a Googlebot user-agent, from a country subdomain, with an explicit locale, or at `/details/experience/` returns the identical 479 masks, so the masking is the product, not an obstacle to route around.

That is enough, because the headline is conventionally "level + role + stack" and this repository already turns exactly that string into exactly the fields this wizard collects. Measured against the spike's real headline, `classify.Parse` resolved `{senior, backend}`, `skilltag.Parse` resolved `[nodejs, python, typescript]`, and `location.Parse` turned `"Florianópolis, Santa Catarina, Brazil"` into `{countries: [br], regions: [latam], cities: [Florianópolis]}`. All four wizard fields, from deterministic dictionaries, with no LLM call.

## What Changes

- Add a server-side LinkedIn public-profile reader: given a profile URL, fetch the public page over `safehttp`, parse the `ld+json` `Person` node, and derive role/skills/level (from the headline, via `classify` + `skilltag`) and a location preference (from the address, via `location`). Anonymous fetch only — no cookies, no session, no credential of any kind is sent or stored.
- Add `POST /api/v1/me/linkedin/import`, cookie-authenticated, taking a profile URL and returning the same `ResumeProfile` shape the CV extraction already returns (`skills` / `categories` / `seniority`), plus the derived location preference and the display fields (name, headline, current company) needed to show the user what was recognised.
- Offer the URL import as a **second, co-equal entry point on the wizard's CV step**, alongside the existing PDF dropzone. A successful import pre-fills the confirm and location steps exactly as a CV extraction does — merged into, not replacing, whatever is already staged.
- The import step SHALL state plainly that LinkedIn does not release work history to it, and SHALL point the user at LinkedIn's own `More → Save to PDF` as the way to get the full history in — which lands back on the existing PDF dropzone and the existing extraction path. This is a product requirement, not a nicety: without it a user reasonably reads "Import from LinkedIn" as "imports my LinkedIn", and the missing experience looks like a defect.
- An import does **not** mark the account as having a CV. The wizard's redirect gate is CV presence, and a URL import produces no CV; the user stays eligible for the prompt on a later visit, which is correct — they still have no CV.
- **BREAKING**: none. New endpoint, new package, additive UI on an existing step.

## Capabilities

### New Capabilities
- `onboarding-linkedin-import`: importing a public LinkedIn profile by URL as an alternative to CV upload — what is fetched, what is derived from it, what is deliberately not obtainable and how that is disclosed, how the result pre-fills the wizard, and how failures degrade.

### Modified Capabilities

None. The wizard's own requirements (gating, step order, skip semantics, the single commit through `PUT /api/v1/me/profile`) are unchanged — this adds a second source that feeds the same staging the CV step already feeds. Those requirements currently live in the unarchived `onboarding-cv-wizard` change rather than in `openspec/specs/`, so they are referenced here, not re-stated or deltaed.

## Impact

- **Backend**: new package `internal/candidate/linkedinprofile` (block `candidate`, layer 4 — imports `dict/classify`, `dict/skilltag`, `dict/location` at layer 2 and `platform/safehttp` at layer 1, all strictly below it). **Must be added to the `candidate` list in `internal/platform/arch/layering/blocks.go`** or both layering guards fail. New handler in `internal/api/handler` registering `POST /me/linkedin/import` next to the existing `/me/resume/extract`.
- **Frontend**: `web/src/routes/onboarding/+page.svelte` (the CV step gains the URL input, the disclosure and the PDF hint); `web/src/lib/api.ts` (new client call); `web/src/lib/types.ts` (the import response type).
- **No database changes.** Nothing about the import is persisted on its own — it feeds the same locally-staged wizard values that the existing single `PUT /api/v1/me/profile` commits.
- **Outbound network on a request path**: this is the first user-triggered fetch of an arbitrary third-party URL from a `/me` handler. It needs `safehttp` (SSRF guard), a host allowlist restricted to LinkedIn profile URLs, a hard timeout, and a response size cap — the page measured ~600 KB, so an unbounded read is not acceptable.
- **`web/static/openapi.yaml`** and `docs/API.md` — the new endpoint is part of the published contract, and the generated docs carry no ratchet.
- **Out of scope**: the experience bank. No employment records or evidence atoms are created — the spike established the source data does not exist. Reading the profile through `extension/` with the user's own session (which would yield the full history) is a separate, later change.
