# CV preview issue highlights — design

Sub-project 2 of the match/tailor merge work (see
`2026-08-08-match-tailor-merge-design.md`). That change deferred this one explicitly.

## Problem

The candidate wants problems in their CV called out visually, right where they are —
not just as a text list. Earlier in the same brainstorming thread we already decided:
underline only problems in **existing** text (not missing skills, which have no text to
underline), and color = severity, not category.

## Feasibility research (this session)

The anchor mechanism already exists and needs no new plumbing:
`CvHtmlPreview.svelte`'s `highlightPaths: string[]` prop + `.cv-lit` dotted-underline
CSS, currently used only for revision-diff highlighting (paths like
`experience[2].bullets[1]`, matching `internal/cv.Document`'s JSON shape).

What's missing is upstream: **none of the three issue-producing analyses currently know
which `Document` field a finding is about.**

- `internal/cvmatch` (live deterministic score): matches skills as flat sets
  (`owned[skill]` lookups). By the time a `LineItem` exists, "where in the text" is
  already gone. Fixing this is a different architecture, not an added field.
- `internal/matchanalysis` (AI fit): scores the **base structured résumé**, not the
  tailored `Document` the Tailor workspace edits. There is no code anywhere that maps a
  base-résumé location to a tailored-CV path, and no guarantee the two even have the
  same shape (different lengths, reordered roles, edited bullets).
- `internal/atscheck` (ATS-readability delta): mostly the same flat-text problem as
  cvmatch — **except** two checks (`chronologyItem`, `bulletsPerRoleItem`) already scan
  the Experience section role-by-role in document order (`roleBlocks`) to do their job.
  They already know "this is the i-th role block encountered" — that ordinal index is
  cheap to keep and pair with `Document.Experience[i]`.

## Scope

**In scope:** only `atscheck`'s `chronologyItem` and `bulletsPerRoleItem` checks get
path-anchored underlines, in this pass.

**Out of scope, later passes:** every other `atscheck` check (format, content, keyword —
flat text, no positional memory), `cvmatch` (needs re-architecture to retain match
provenance), `matchanalysis` (needs a base-résumé-to-tailored-Document mapping that
doesn't exist and isn't guaranteed to be meaningful). Each is a separate, larger change;
none is bundled here.

## Design

**Backend (`internal/atscheck`):** `roleBlocks`' scan already visits role blocks in
document order. Thread that ordinal index through to `chronologyItem`/`bulletsPerRoleItem`
so a failing check can attach a `Path` (e.g. `"experience[1]"`) to its `LineItem`. Every
other check's `LineItem` simply carries an empty `Path` — unaffected, backward compatible.
Regenerate the TS contract so the SPA sees the new field.

**Frontend:**
- `AtsDelta.svelte` (Score tab) passes every line item's non-empty `path`, tagged with
  its severity (`fail`/`warn`, already how `atscheck` grades a check — no new vocabulary),
  into `CvHtmlPreview`'s `highlightPaths`.
- `CvHtmlPreview.svelte`: extend `.cv-lit` to take a severity modifier — `.cv-lit-fail`
  (red dotted underline) / `.cv-lit-warn` (yellow). Revision-diff highlighting is a
  different call site with its own (unaffected) neutral color.
- Underlines appear as soon as the delta is computed — the workspace already fetches it
  unprompted on open and after an autopilot run (existing behavior, `tailor-ats-delta`
  spec's "surfaces the delta without being asked") — regardless of whether the Score tab
  is open. The preview is the center column, always visible; the underline shouldn't wait
  on the candidate opening a specific tab to notice it.
- Hover reveals the check's message in a small dark tooltip (validated live in the visual
  mockup) — not the browser's native `title` tooltip, to match the app's tone.

## Risks / open items for the implementation plan

- Confirm `roleBlocks`' scan order truly matches `Document.Experience` order 1:1 after
  the renderer's blank-entry filtering (the same `keepIndex` concern `revisions.ts`
  already solves for revision highlighting — reuse that reasoning, don't re-derive it).
- Decide the exact tooltip component: reuse an existing primitive under `$lib/ui` if one
  exists, else a small new one — a task-list-level decision, not a design-level one.
