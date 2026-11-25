## Why

Skills live in three places that must currently be kept in sync by hand: the CV document's
skills section, the experience bank's evidence atoms, and the profile's search-filter skill
list (`userprofile.skills`). Only the third is stale by default — it advances only when the
candidate notices and clicks the tailor page's "Add N skills to profile" button, which diffs
one open tailored CV against the profile. A skill the bank already holds evidence for (and
that the tailoring agent could therefore write into a CV) does not reach search filters or
match scoring until that manual step happens, so a candidate can be under-matched against
their own proven skills with no visible cause.

## What Changes

- The experience bank's atom write path (create and update) upserts the atom's canonical
  skills into the owner's `userprofile.skills`, respecting the existing `PROFILE_MAX_SKILLS`
  cap and excluding anything already in `excluded_skills` — the same merge semantics
  `withSkill`/`withSkills` already apply for a manual claim. This fires regardless of how the
  atom entered the bank (CV import, chat, future manual entry), from one choke point in
  `internal/experience`'s write path.
- **BREAKING (internal only, no external API change):** `internal/experience`'s `Store` gains
  a dependency on a small interface for upserting profile skills, implemented by
  `internal/userprofile` — mirroring how `internal/cvedit` depends on an `EvidenceGate`
  interface rather than importing `internal/experience` directly.
- **Removal:** the tailor page's "Add N skills to profile" control and its supporting logic
  (`web/src/lib/tailor/skillDiff.ts`, the compute/confirm/apply block and button in
  `web/src/routes/tailor/[slug]/+page.svelte`) are deleted. By the time the tailoring agent
  writes a skill into a tailored CV, `cvedit.EvidenceGate` already required it to cite an
  existing atom — so that atom's skills were already synced into the profile at write time,
  ahead of and independent of any specific CV. The button has nothing left to add.
- No change to: CV-upload-to-bank import, bank-to-CV relevance retrieval
  (`experience.Retrieve`), the job-match "I have it" / "avoid" claim UI
  (`job-profile-match`), `excluded_skills`, or any wire/API shape.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `experience-bank`: writing an atom (create or update) now also upserts the atom's
  canonical skills into the owner's profile skill list, so the profile reflects bank
  evidence without a separate manual step.

## Impact

- **Go:** `internal/experience` (new upsert-on-write behavior, new small interface),
  `internal/userprofile` (implements the interface; repository upsert already exists in
  spirit via the existing `Upsert` used by the profile handler).
- **Web:** `web/src/lib/tailor/skillDiff.ts` deleted; `web/src/routes/tailor/[slug]/+page.svelte`
  loses the "Add N skills to profile" control and its state.
- **Accepted limitation, not addressed by this change:** a skill a candidate hand-types
  directly into their own CV's skills section (permitted without evidence — `cvedit` exempts
  the human actor from the citation requirement) has no backing atom and will not
  auto-sync to the profile. The candidate can still add such a skill to their search profile
  directly through the existing job-match or profile-editing UI.
