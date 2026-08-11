## 1. Overlay panel component

- [x] 1.1 Create a new component (e.g. `web/src/lib/components/TalentNetworkPanel.svelte`)
      hosting the current settings logic. Follow the existing overlay
      pattern already used in this codebase — read
      `web/src/lib/components/FollowUpDialog.svelte` for the exact shape:
      a full-screen backdrop `<button>` (click-to-close) plus a
      `role="dialog"` panel, not a new design-system primitive.
- [x] 1.2 Move the GET/PUT `/me/talent-network` fetch logic and state
      (currently in `TalentNetworkSettings.svelte`) into this new
      component. Keep using `getTalentNetwork`/`setTalentNetworkVisibility`
      from `web/src/lib/api.ts` — no API-layer changes.
- [x] 1.3 Panel layout, top to bottom: (a) a public-link card — always
      rendered, even when visibility is `off`, showing the profile URL
      (built the same way the current settings component already builds
      it) and a "View" link/action; (b) the Off/Public/Anonymous picker,
      each option now showing an icon (🚫 / 🌐 / 🕶️) beside its existing
      label + description text.
- [x] 1.4 Preserve existing behavior carried over from
      `TalentNetworkSettings.svelte`: echoed-value updates from the PUT
      response (no optimistic assumption), the trade-off copy near the
      picker, and error surfacing through the parent's existing
      `actionError` pattern (via an `onerror` prop, matching the current
      component's contract).

## 2. Status-aware entry button

- [x] 2.1 On `web/src/routes/my/profile/+page.svelte`, add a button near
      the top of the page (next to the page heading) that opens
      `TalentNetworkPanel`. Fetch the current visibility (reuse
      `getTalentNetwork`) on page load to decide the button's initial
      appearance — check whether the page already loads a value it can
      reuse, or add a lightweight fetch alongside the existing ones.
- [x] 2.2 Button rendering: `off` → solid filled CTA, "Join Talent
      Network". `public`/`anonymous` → outlined pill with the mode's icon
      + "Talent Network: <Mode>". Update the button's own displayed state
      when the panel closes with a changed value (the panel's echoed PUT
      response is the source of truth — thread it back up via a callback
      prop, don't re-fetch).
- [x] 2.3 Remove the old inline `TalentNetworkSettings.svelte` render from
      the Settings tab — the panel is now the only entry point. Delete
      `TalentNetworkSettings.svelte` if `TalentNetworkPanel.svelte` fully
      supersedes it (confirm no other importer first).

## 3. Public profile page redesign

- [ ] 3.1 Restyle `web/src/routes/talent-network/[publicId]/+page.svelte`:
      header block (avatar-or-initials circle — initials from name when
      present, a generic icon when absent in anonymous mode; headline;
      location; one-line summary; skill chips inline) followed by a
      single-column experience list (small logo/initial placeholder box,
      title, company, date range, description) then education. No new
      data — every field is already returned by the existing endpoint;
      this task is template/CSS only.
- [ ] 3.2 Confirm existing behavior is preserved through the restyle: name
      omitted in anonymous mode, masked-employer label rendering, empty-CV
      graceful rendering (existing empty-state message), 404 page
      unaffected (out of scope for this task — it's a separate SvelteKit
      error page, not touched by the template restyle).

## 4. Verification

- [ ] 4.1 `pnpm run check`, `pnpm run lint`, `pnpm run build` all pass.
- [ ] 4.2 Manual check (this repo has no component-level test runner):
      button shows correct state in all three visibility modes; panel's
      link card is visible and correct when `off`; all three mode-picker
      icons render; public page renders correctly for `public` and
      `anonymous` profiles, including an empty-CV profile.
