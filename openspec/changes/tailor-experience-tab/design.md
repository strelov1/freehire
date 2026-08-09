## Context

`ExperienceBankView.svelte` is a fully self-contained component (no props; fetches
`api.getExperience()` itself, writes via `api.updateExperienceAtom`/`deleteExperienceAtom`),
already mounted on `/my/profile`. `internal/handler/me_experience.go`'s `UpdateAtom` already
forces `provenance: manual` on every call it handles, regardless of whether the submitted claim
text differs from the stored one — confirming an atom is already, structurally, just "call update
with the atom's own current claim." This is a small change: embed an existing component in a new
tab, and add a thin frontend shortcut to a call path that already exists.

## Goals / Non-Goals

**Goals:**
- The experience bank is reachable and actionable from inside the Tailor workspace, without a
  navigation away from the session.
- Confirming an assistant-inferred claim is one click, not edit-then-save.

**Non-Goals:**
- No backend changes — this design deliberately avoids touching `internal/handler/me_experience.go`
  or `internal/experience` at all.
- No live interaction between confirming an atom and the currently-open tailoring session or
  turn — the bank is a shared resource read fresh by any future autopilot run, not something this
  change wires into the CV preview or the chat.
- No redesign of `ExperienceBankView`'s existing edit/remove flows — the confirm button is
  additive.

## Decisions

**1. Embed `ExperienceBankView` as-is, mounted in a new tab.** Considered building a
tailor-scoped, narrower variant instead (the sidebar column is ~350-720px vs. the profile page's
full width) — rejected: the component's layout (`flex flex-col gap-6`, wrapping sections) already
reflows for narrow containers used elsewhere in the app, and a second near-duplicate view is a
maintenance liability the moment one changes without the other. If the narrow column reveals a
real layout problem once built, fix it in the shared component, not by forking it.

**2. Confirm calls `updateExperienceAtom` with the atom's own unchanged `claim`, not a new
endpoint.** Considered adding a dedicated `POST /me/experience/atoms/:id/confirm` — rejected as
unnecessary indirection: the existing `PATCH`-style update already does exactly what confirming
needs (re-stamp provenance to `manual`), and a second endpoint doing the same mutation through a
different name is a second thing to keep in sync for no behavioral gain.

**3. Tab order becomes `Chat, Editor, Experience, Templates, Settings`.** The candidate's own
ask, and it also puts Chat — the tab a fresh cold-start session opens into (per
`tailor-coldstart-autopilot`) — first, ahead of Editor.

## Risks / Trade-offs

- **[Risk]** The `Confirm` button reuses the `Check` icon already imported for the edit-mode
  save action, in the same file. → **Mitigation**: the two uses are mutually exclusive by row
  state (edit-mode save only renders while `editing === atom.id`; Confirm only renders while an
  atom is `agent_inferred` and NOT being edited) — no row ever shows both, so there's no visual
  ambiguity despite the shared icon.
- **[Risk]** No automated test coverage for Svelte component behavior exists in this repo today
  (per `web/`'s own conventions — vitest covers logic-only `.ts` modules). → **Mitigation**:
  verify via `pnpm check` (types) and a live browser check, matching how the rest of `web/`'s
  UI-only changes are already verified in this codebase.

## Migration Plan

None — no data migration, no API version change. Deploy order doesn't matter (frontend-only
change); rollback is a plain revert.
