## Why

A candidate mid-tailoring-session who wants to check, correct, or confirm what the assistant has
recorded about them has to leave the Tailor workspace for `/my/profile` and lose their place — the
experience bank is invisible from Tailor even though it's what every tailored CV is built from.
Separately, confirming an assistant-inferred claim today requires opening its edit field and
re-saving the same text; there's no one-click "yes, this is right," even though the backend
already re-stamps provenance to `manual` on any update call regardless of whether the text
changed.

## What Changes

- A new "Experience" tab in Tailor's left sidebar, positioned between Editor and Templates
  (`Chat, Editor, Experience, Templates, Settings`), embedding the existing
  `ExperienceBankView.svelte` component unmodified — it already fetches its own data and needs no
  new props.
- A one-click Confirm button on every `agent_inferred` (unconfirmed) atom in
  `ExperienceBankView.svelte`, beside the existing Edit/Remove buttons — calls the same update
  path the edit-save flow already uses, without opening the textarea first. Because the
  component is shared, this button also appears on `/my/profile`.
- **No backend changes.** `UpdateAtom` (`internal/handler/me_experience.go:197-206`) already
  forces `provenance: manual` on every call; the new button is a frontend-only shortcut to a call
  the UI already makes elsewhere.

## Capabilities

### New Capabilities

(none — this extends two existing capabilities rather than introducing a new one)

### Modified Capabilities

- `tailor-workspace`: the left-sidebar tab set gains a fifth tab, `experience`, and the existing
  tab order changes (`Chat` moves first).
- `experience-bank`: gains a one-click confirm path for an `agent_inferred` atom, alongside the
  existing edit-to-confirm path — same outcome (provenance becomes `manual`), a second way to
  reach it without editing the claim text.

## Impact

- Frontend: `web/src/lib/components/ExperienceBankView.svelte` (new Confirm button + handler),
  `web/src/routes/tailor/[slug]/+page.svelte` (new `LeftTab` value, reordered `leftTabs`, new
  panel mounting `<ExperienceBankView />`).
- Unaffected: `internal/handler/me_experience.go` and every other backend path — no API changes,
  no new endpoint, no wire-shape change to `ExperienceAtom`.
- Unaffected: `/my/profile`'s own layout and entry point (same component, now with one more
  button, otherwise unchanged).
