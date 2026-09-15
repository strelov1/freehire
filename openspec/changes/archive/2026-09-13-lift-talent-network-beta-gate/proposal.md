## Why

The Talent Network shipped gated to the beta group so a working catalogue would not open
onto four candidates. It has since had time to accumulate a real membership, and the
gate now only prevents wider adoption: search engines are told not to index the list
(`noindex WHILE JOINING IS BETA-ONLY`), and every non-beta candidate sees no invitation
at all. There is no measurement gate here — the feature is simply ready to open, and the
longer it stays beta-only the longer the catalogue understates itself to both visitors
and search engines.

Separately, the account navigation and the profile page have carried two entry points
into membership since launch (`accountNav.ts`'s own comment: "the feature previously
shipped with a working page and no way to reach it, which is indistinguishable from not
having shipped"). With the nav grown to fifteen sections, a second entry point earns its
keep only while the first is unreliable; the profile page's `TalentNetworkInvite` card
already states standing and links to the control, so it can carry that job alone.

## What Changes

- Remove the beta-tester restriction on JOINING the Talent Network: the server no longer
  refuses a join from an account outside the beta group (`internal/api/handler/me_talent_network.go`).
- Remove the `beta` gate on the profile invitation (`TalentNetworkInvite.svelte`) — every
  signed-in candidate sees it, not only beta testers.
- Lift `noindex` on the `/talent` catalogue list page now that its membership is not
  beta-only.
- **BREAKING** (spec-level, not wire-level): retire the account-navigation entry point.
  Remove the `betaOnly` Talent Network item from `web/src/lib/accountNav.ts` — the
  profile page's invitation card becomes the sole way to reach `/my/talent-network`
  short of a direct URL, replacing the "reachable from account navigation" requirement
  with "reachable from the profile page."
- Sweep stale beta-gate comments left in the touched files (`accountNav.ts`,
  `TalentNetworkInvite.svelte`, `talent/+page.svelte`) so they describe the shipped
  state rather than the lifted one.
- Leave `users.beta_tester`, the Mentorship `betaOnly` nav entry, and every other
  consumer of the beta-tester mechanism untouched — only the Talent Network gate and its
  nav entry are in scope.

## Capabilities

### New Capabilities

(none)

### Modified Capabilities

- `talent-network-membership`: retires "Joining is limited to the beta group while the
  feature settles" (the join refusal, the forced `noindex`, and the "entry points
  hidden while the gate stands" scenarios all go with it), and narrows "The control is
  reachable without a direct URL" to a single entry point — the profile page — instead
  of both the account navigation and the profile page.

## Impact

- **Backend**: `internal/api/handler/me_talent_network.go` (join handler loses its
  `IsBetaTester` check and the 403 branch); `me_talent_network_test.go` loses the
  now-untestable refusal case and gains a case confirming a non-beta account can join.
- **Frontend**: `web/src/lib/accountNav.ts` (drop the Talent Network item),
  `web/src/lib/accountNav.test.ts` (drop assertions about it), `web/src/lib/components/profile/TalentNetworkInvite.svelte`
  (drop the beta gate), `web/src/routes/talent/+page.svelte` (drop the forced
  `noindex`), and any test fixture asserting on the current nav list or on the
  invitation's beta-gated visibility.
- **No migration, no new route, no new dictionary.** `users.beta_tester` and the
  Mentorship nav entry are untouched.
