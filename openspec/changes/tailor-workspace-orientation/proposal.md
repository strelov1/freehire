## Why

Three things reported from using the tailoring workspace on production:

1. **A CV opened by id showed an empty chat with no way in.** The two opening actions were keyed
   on "this is a fresh workspace", but a CV re-opened by `?cv=` can carry a conversation bound at
   bootstrap that nobody ever spoke to. That case rendered "Ask the agent anything to get started"
   and nothing else — indistinguishable from a chat whose history had been lost.
2. **The active tab was not readable as active.** Editor / Settings / Chat marked the current one
   with `bg-muted`, which against the panel's own background is nearly invisible: you cannot tell
   which pane you are looking at.
3. **The account section was still called "CV builder"**, and the agent and the tailoring list were
   reachable only from inside the account shell, while the inbox was duplicated in the header menu.

## What Changes

- The workspace offers its two actions whenever the conversation is EMPTY, not only when it was
  just bootstrapped. The chat already renders them only while there are no messages, so the
  "resuming" condition was a second, wrong gate on the same thing.
- The active tab in both the left panel and the artifact panel is marked with the brand tint
  (the palette's olive) and a heavier weight, rather than a barely-visible grey.
- The account section is renamed **Tailor** — every CV in it is aimed at one vacancy, and
  "CV builder" named the tool it grew out of.
- The header menu duplicates **Agent** and **Tailor** beside the inbox it already carried, so the
  three things opened from anywhere are reachable from anywhere.

## Capabilities

### New Capabilities
<!-- none -->

### Modified Capabilities
- `tailor-workspace`: the opening actions follow an empty conversation rather than a fresh
  workspace, and the active tab is legible.
- `account-navigation`: the tailoring section's name, and which sections the header menu repeats.

## Impact

- **Web only:** `web/src/lib/tailor/autopilot.ts` (`openingFor` → `openingActions`),
  `web/src/routes/tailor/[slug]/+page.svelte`, `web/src/lib/tailor/ArtifactPanel.svelte`,
  `web/src/lib/accountNav.ts`, `web/src/lib/components/HeaderMenu.svelte`.
- No API, schema or backend change.
