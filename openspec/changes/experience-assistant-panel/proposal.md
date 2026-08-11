## Why

The Experience tab's two ways into the interviewer — "Add an achievement" and the
selection's "Tailor with assistant" — are both full navigations to `/my/assistant`.
The candidate leaves the list they were reasoning about: the selection is gone, the
near-duplicate pair they had just spotted is no longer on screen, and answering the
agent's question means remembering what the two atoms said. The whole point of
selecting a cluster is that the agent and the candidate are looking at the same rows,
and today only one of them is.

The selection action bar has a second, smaller problem that makes the first one worse.
It is `sticky top-0` while the site header is `sticky top-0 z-40` — so on any bank long
enough to scroll, the bar pins *underneath* the header and disappears exactly when it
is needed. Selecting two atoms far down the list currently leaves no visible way to
merge them.

## What Changes

- The experience view gains a **docked assistant panel** on the left of the bank.
  Opening it no longer navigates: the bank stays mounted, visible and interactive
  beside the conversation, so the candidate can keep selecting and correcting rows
  while the agent asks about them.
- The panel is **non-modal on desktop** — no focus trap, no inert background, the list
  behind stays live. Below `lg`, where there is no room for two columns, it becomes a
  full overlay with a close control.
- Both existing entries — the header action and the selection's tailor action — open
  the panel in place instead of navigating. The selection's atom ids become the panel's
  opening message rather than a query string.
- Bank writes made **inside** the conversation (merge, add, update) refresh the list
  behind it without a page reload, so the two halves cannot disagree.
- The selection action bar is pinned **below** the site header rather than under it, so
  Merge / Tailor / Clear stay reachable however far the bank is scrolled.
- `/my/assistant?preset=profile&atoms=<ids>` keeps working unchanged as an address. It
  is a bookmarkable deep link and the panel is not a replacement for it; the two share
  one kickoff builder so they cannot drift.
- **No** Sheet primitive is extracted. The design system's documented Sheet gap is real
  but has four other call sites and is not this change's job — the seam is noted in
  `design-system/AGENTS.md`, and this panel is built locally against it.

## Capabilities

### New Capabilities

None. This changes how an existing surface is reached and laid out, not what the
product can do.

### Modified Capabilities

- `experience-bank`: the way into the interviewer becomes in-place rather than a
  navigation, the bank stays live beside the conversation and reflects what the
  conversation writes, and the selection action bar remains reachable while scrolling.

## Impact

Frontend only — no API, schema, or worker change, and no new endpoint. `AssistantChat`
is already a reusable component with a host-supplied layout (the tailoring workspace
embeds it the same way, with `showSessionRail={false}`), so this adds a second host
rather than new chat machinery.

- `web/src/lib/components/ExperienceBankView.svelte` — panel host, pinned action bar,
  in-place open in both entries.
- A new panel component under `web/src/lib/components/` wrapping `AssistantChat`.
- `web/src/lib/assistant/presets.ts` — the kickoff text for a set of atom ids becomes a
  shared builder used by both `entryFromQuery` and the panel.
- `web/src/routes/my/profile/+page.svelte` — only if the panel needs width the tab body
  does not currently give it.
- `web/AGENTS.md` — record that the Experience tab hosts the assistant in place.

Risk is contained to one tab. The chat itself is unchanged: same preset, same tools,
same session list, same server.
