## Context

See proposal.md for why. The constraints that actually shape the approach:

- **`AssistantChat` is already a reusable, host-laid-out component.** `/my/assistant` and `/tailor/[slug]` both mount it; the tailoring host passes `showSessionRail={false}`, `onTurnComplete`, and a bounded-height flex parent. A second host is the established pattern, not new machinery.
- **`kickoff` is captured once and spent.** `const arrival = { preset, kickoff }` is a plain object read at component construction, and `boot()` clears `arrival.kickoff` after the first dispatch. Changing the `kickoff` prop on a **mounted** chat does nothing. Opening the interviewer on a *different* selection therefore requires a remount, not a prop update.
- **`preset !== 'chat'` with no `session` always mints a new conversation.** `boot()` calls `createAndOpen()` in that branch precisely so `?preset=profile` starts an interview rather than resuming the newest chat. The panel gets the behaviour it needs for free by passing `preset="profile"` and no `session`.
- **Unmounting cancels an in-flight turn.** The `/my/assistant` route node exists as one node specifically because swapping components aborted a streaming opening turn. A close control that unmounts would kill a turn mid-answer.
- **The site header is `sticky top-0 z-40`, `h-14`.** The current action bar is `sticky top-0 z-10`, so it pins *behind* the header. This is the whole of the reported "buttons are not fixed" defect — sticky is working, the offset and stacking order are wrong.
- **Sticky works here.** No ancestor between the action bar and the viewport sets `overflow` (`my/+layout` → `div.min-w-0.flex-1` → `div.flex.gap-6` → `main.flex-1` → tabpanel). Nothing needs restructuring to make pinning work.
- **Width is genuinely tight.** The account shell is `max-w-6xl` (1152px) with a `w-56` (224px) nav plus a `gap-8` (32px) at `lg`+, leaving ~896px for content however wide the viewport is.
- **The design system has no Sheet primitive.** `design-system/AGENTS.md` names this as a known gap with four hand-built call sites (`JobDrawer`, `FilterModalShell`, `OnboardingWizard`, `CookieConsent`). `Dialog` is a centred modal that cannot be repositioned from the call site.

## Goals / Non-Goals

**Goals:**

- One panel component hosting `AssistantChat`, docked beside the bank and non-modal where there is room for two columns.
- One kickoff builder shared by the panel and `entryFromQuery`, so the in-place and addressable entries cannot drift.
- A bank that refreshes from the server when a turn changes it, keeping the selection the candidate still has.
- An action bar pinned clear of the site header.

**Non-Goals:**

- Extracting a `Sheet` primitive, or migrating the four existing hand-built drawers. Confirmed with the user; the seam stays noted in `design-system/AGENTS.md`.
- A resizable / collapsible panel like the tailoring workspace's left column. One fixed width until someone asks.
- A session rail inside the panel, or any change to session listing, presets, tools, or the server.
- Removing `/my/assistant?preset=profile&atoms=…`. It stays a first-class address.

## Decisions

### The panel is a sticky flex column, not an overlay, at `xl` and up

Docked form: a `sticky top-14` column with `h-[calc(100dvh-3.5rem)]`, ~360px wide, placed **before** the bank in the Experience tab's flex row. Sticky rather than fixed because it must sit inside the content column's flow — the account nav already owns the left edge of the page, and a `fixed` panel would have to hard-code an offset that changes when the nav collapses.

The breakpoint is `xl`, not `lg`. At `lg` the content column is ~896px; 360px of panel plus a 24px gap leaves ~510px of bank, which is where the two-line achievement rows with their metric chips and flag pills start wrapping badly. Below `xl` the panel becomes a full overlay (`fixed inset-0 z-50`) with an explicit close control, the same shape `JobDrawer` already uses.

*Alternative considered:* letting the Experience tab break out of `max-w-6xl` when the panel is open. Rejected — the width would jump as the panel opens and closes, and the tab strip above it would visibly reflow.

*Alternative considered:* a `fixed` right-hand drawer. Rejected — the user asked for the left, and it matches the tailoring workspace, where the chat is also the left column.

### Non-modal means genuinely non-modal

No `showModal()`, no focus trap, no `inert`, no scroll lock, no backdrop, on the docked form. The spec requires the bank stay selectable and editable while the conversation is open, and every one of those mechanisms exists to prevent exactly that. This is why `Dialog` is not the vehicle even though it is the primitive in reach: it is `<dialog>.showModal()` by construction.

The overlay form below `xl` covers the bank, so it takes `role="dialog"`, `aria-modal="true"` and a close control — matching `JobDrawer`.

### A launch token forces the remount that a new subject requires

The panel holds `launch = { id: number, kickoff: string }`. `AssistantChat` is wrapped in `{#key launch.id}`. Pressing "Add an achievement" or "Tailor with assistant" increments the id, which remounts the chat, which mints a fresh `profile` session and sends that launch's kickoff.

This is forced by `arrival.kickoff` being spent at construction — a mounted chat cannot be re-aimed. It is also the right product behaviour: asking about a *different* pair of achievements is a different conversation, not a new message in the old one.

Closing the panel **hides** it (`class:hidden`) rather than unmounting, so a turn that is still streaming survives being dismissed and is still there on reopen. Unmount happens only on a new launch. This mirrors the tailoring workspace, which keeps the chat mounted across tab switches for the same reason.

*Alternative considered:* making `kickoff` reactive inside `AssistantChat`. Rejected — `arrival` is deliberately non-reactive, and the comment on it records that reactive kickoff put the same words in the caller's mouth twice. Changing it would risk every host.

### One kickoff builder, two entries

`profileKickoff(ids: string[]): string` moves next to `PROFILE_KICKOFF` in `presets.ts`. `entryFromQuery` calls it after validating ids from the query; the panel calls it with the current selection. The spec requires both entries produce the same opening message, and a shared function is the only way that stays true without a test that compares two string literals.

Id validation stays where it is: the panel's ids come from rendered rows and are already the candidate's own, but the builder is fed through the same UUID filter so a single rule covers both.

### The bank refreshes on `onTurnComplete`, and the selection is pruned rather than cleared

The panel passes `onTurnComplete` straight to the existing bank reload — the same seam the tailoring workspace uses to refresh its CV preview after a turn.

Today `load()` ends with `selected = []`. That is wrong once a turn can change the bank underneath an open selection: a merge deletes one of two selected ids and clearing both throws away a selection the candidate did not touch. The reload keeps the selection intersected with the ids that still exist. A refresh the *candidate* triggered by merging from the action bar still clears, because that selection has been consumed.

Turn-complete is a coarse trigger — it refetches after every turn, including ones that only talked. `GET /me/experience` is one owner-scoped query and this is a single open tab; a narrower signal would mean the chat telling the host which tools ran, which is a new contract across two components to save one cheap request.

### The action bar pins below the header

`sticky top-14 z-30`: `top-14` clears the `h-14` header, `z-30` sits under its `z-40` and above the list. The two in-flow banners (`EmailVerificationBanner`, `ProductHuntBanner`) scroll away and are irrelevant once the list is scrolled at all, so no dynamic offset is needed.

*Alternative considered:* a measured offset from the header's real height. Rejected — the header is a fixed `h-14` in one file, and a resize observer for a constant is machinery in place of a number.

### The panel names the way to the full assistant

With no session rail, the panel cannot reach the candidate's other conversations. The panel captures the live session id via `onSessionChange` (without navigating, unlike the `/my/assistant` host) and renders a link to `/my/assistant/<id>`, so a conversation that outgrows a 360px column can be opened full-width without losing it.

## Risks / Trade-offs

- [~510px of bank beside the panel is cramped at `xl`, and the `xl` gate means many laptops get the overlay instead of the side-by-side the user asked for] → Accepted for a first cut, and the breakpoint is one class to move once it has been used on real hardware. Going wider means letting the tab break its container, which reflows the whole page as the panel opens.
- [Refetching the bank after every turn, including conversational ones] → One indexed owner-scoped read on an open tab. Revisit only if the transcript grows tool-run reporting for another reason.
- [A launch remount abandons the previous conversation mid-turn if the candidate re-launches while one is streaming] → Disable the launch actions while `onTurnStateChange` reports a turn active, the same signal the tailoring workspace already consumes.
- [Two sticky elements (panel column and action bar) at the same offset] → They are in different columns and never overlap horizontally; the action bar belongs to the bank column only.
- [A fifth hand-built drawer deepens the documented Sheet gap] → Deliberate and confirmed with the user. The overlay form is deliberately shaped like `JobDrawer` so a future Sheet extraction has one more consistent call site rather than one more novel one.
- [`onTurnComplete` fires for turns in a conversation about something else entirely, since the panel's chat can be steered anywhere] → Harmless: the refresh is idempotent and the panel is only ever open on the Experience tab.

## Migration Plan

Frontend-only, no schema and no API change, so this ships and rolls back with the web bundle alone. Nothing to migrate, no flag: the previous behaviour (navigate to `/my/assistant`) remains reachable at its own URL, which is also the rollback target.

## Open Questions

- Whether the docked panel should remember its open/closed state across visits to the tab. Deferred — it does not change the specs, the approach, or the task list, and is a one-line `localStorage` read next to the account nav's existing `hire.myNavCollapsed` if it turns out to be wanted.
