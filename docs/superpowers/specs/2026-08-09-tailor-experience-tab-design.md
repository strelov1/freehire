# Tailor workspace: an Experience tab, and a one-click confirm — design

## Problem

The candidate's experience bank already has a full view — `ExperienceBankView.svelte`, mounted
on `/my/profile` — that lists every banked achievement, flags the ones the assistant inferred
(`agent_inferred` provenance) as not yet confirmed, and lets the owner edit or remove any entry.
An unconfirmed entry never appears on a CV: `tailor-autopilot`'s evidence gate only writes
claims the candidate themselves asserted (`cv_import` / `stated_in_chat` / `manual`), never the
model's own reading.

Two gaps, both raised live while testing the just-shipped cold-start autopilot change:

1. **The bank is invisible from Tailor.** A candidate mid-tailoring-session who wants to check,
   correct, or confirm what the assistant has on file has to leave the workspace for
   `/my/profile` and lose their place.
2. **Confirming a claim requires editing it.** The only way today to move an `agent_inferred`
   atom to `manual` is to open its edit field and re-save the same text — there is no one-click
   "yes, this is right" action, even though the backend already re-stamps provenance to `manual`
   on any update call regardless of whether the text changed
   (`internal/handler/me_experience.go:206`, `in.Provenance = experience.ProvenanceManual`
   unconditionally).

## Decision

Both gaps are small, and the fix is embedding + one small addition, not new backend work.

**1. A new "Experience" tab in Tailor's left sidebar.** `web/src/routes/tailor/[slug]/+page.svelte`
gains a fifth `LeftTab` value, `'experience'`, rendering `<ExperienceBankView />` unmodified — the
component takes no props and fetches its own data (`api.getExperience()`), so this is a mount, not
a rewrite. Tab order becomes `Chat, Editor, Experience, Templates, Settings` (the candidate's
explicit ask, moving Chat first since it's the tab a fresh session opens into).

**2. A one-click Confirm action on `ExperienceBankView.svelte`.** Next to the existing Edit/Remove
buttons on every `agent_inferred` atom, a new button calls the same
`api.updateExperienceAtom(atom.id, { ...atom, claim: atom.claim })` the edit-save path already
calls, just without opening the textarea first. No backend change: `UpdateAtom`
(`internal/handler/me_experience.go:197-206`) already forces `provenance: manual` on every call.
Because the component is shared, this button appears on `/my/profile` too — intentional, not a
scope leak: the same claim deserves the same one-click confirmation wherever it's reviewed.

## Non-goals

- No new backend endpoint, no change to the `Atom`/`ExperienceAtom` wire shape.
- Confirming an atom does not interact with the current tailoring session or turn — it writes to
  the bank, which any future autopilot run (this one or a later one) reads fresh. No live
  refresh of the open CV is triggered by a confirm.
- No change to `/my/profile`'s own layout or entry point — it keeps using the same component,
  now with the new button, unchanged otherwise.

## Implementation sketch

- `web/src/lib/components/ExperienceBankView.svelte`: add a `Confirm` button (reuse the `Check`
  icon already imported — it currently only renders inside the edit-mode row, so there's no
  visual collision with a second, non-editing-state use of the same icon) beside Edit/Remove,
  visible only when `unconfirmed(atom)` and not currently `editing`. Calls a new `confirmAtom`
  function mirroring `saveEdit`'s call shape but skipping the draft/textarea state.
- `web/src/routes/tailor/[slug]/+page.svelte`: add `'experience'` to the `LeftTab` union and the
  `leftTabs` array in the new order; add the panel `<div class="h-full overflow-auto p-4"
  class:hidden={leftTab !== 'experience'}><ExperienceBankView /></div>` alongside the existing
  Editor/Templates/Settings panels.
- No OpenSpec capability changes beyond `tailor-workspace` (tab set) and possibly a light touch to
  `experience-bank`'s existing confirm-flow requirement, if one names the edit-only path
  explicitly — check `openspec/specs/experience-bank/spec.md` during spec-writing.

## Testing

- `ExperienceBankView`'s new confirm action: no existing test harness for this component found in
  a quick pass; verify manually and via `pnpm check`/`pnpm test` as the rest of `web/` does — the
  repo does not run Svelte component-level automated tests today.
- Tab reorder and new panel: `pnpm check`, then a live browser check (per this repo's own
  practice — no headless component test harness) that the tab appears, orders correctly, and
  `ExperienceBankView` loads inside it.
