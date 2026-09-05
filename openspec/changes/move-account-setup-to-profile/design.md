## Context

See proposal.md - Why/What Changes for motivation. Current state:

- `AccountSetupCard.svelte` renders the checklist from `accountCompleteness.ts`'s `accountSteps()`, and is only mounted in `web/src/routes/my/tracking/+layout.svelte`.
- Each `CompletenessStep.href` is one of a closed literal union (`SetupHref`) checked against SvelteKit's `resolve()`, so a step can never link to a route that doesn't exist. Three of five steps (`role`, `skills`, `location`) already resolve to `/my/profile`.
- `/my/profile/+page.svelte` keeps its selected section in a local `$state<ViewId>` variable (`view`), read from `page.url.searchParams.get('tab')` exactly once, at the `let view = $state(...)` initializer. It never re-reads the URL after that.

## Goals / Non-Goals

**Goals:**
- Move the checklist to `/my/profile`, rendered only once a profile exists.
- Make the `role`/`skills`/`location` steps land on their own tab instead of the default one.
- Make that work both as a fresh navigation to `/my/profile?tab=<id>` and as an in-page link click while already on `/my/profile` (a same-route navigation that only changes the query string).

**Non-Goals:**
- Two-way sync of `view` back into the URL on manual tab clicks (e.g. via `replaceState`) — out of scope; the existing tabs already work without it, and this change only needs the URL → tab direction.
- Any change to which fields count as "done" (`accountSteps`'s predicates) — untouched.
- A generic/typed coupling between `accountCompleteness.ts` and the profile page's `ViewId` — see Decisions.

## Decisions

**Add an opaque `tab?: string` field to `CompletenessStep`, not a typed one.** `accountCompleteness.ts` is deliberately free of any SvelteKit import so it stays unit-testable in plain Node (see its own header comment). Importing the profile page's `ViewId` type would couple a shared, page-agnostic module to one page's local tab enum. A plain optional string is forwarded verbatim by `AccountSetupCard` as a query value; correctness of the three tab names used (`profile`, `skills`, `location`) is covered by the design's manual verification and by keeping the literal strings identical to `VIEWS[].id` in the profile page.

**Build the link by appending `?tab=` after `resolve(step.href)`, matching the existing `ReferralsLandingView.svelte` pattern** (`` `${resolve('/my/referrals')}?tab=offers` ``) rather than extending `SetupHref` itself with query strings — `resolve()` validates pathnames against the route table, not arbitrary query combinations, so composing the tab suffix outside of `resolve()` keeps that build-time safety for the path while still allowing a tab suffix.

**Give the `role` step an explicit `tab: 'profile'` even though `'profile'` is already the page's default.** Without it, a user already on a non-profile tab who clicks "Say what you do, and at what level" would navigate to the same pathname+query they might already be missing a change from (if no `?tab=` is present yet), but more importantly an explicit tab keeps all three profile-bound steps uniform and correct regardless of what tab is currently open — omitting it would make correctness depend on which tab happens to be selected when the link is clicked.

**Make `view` reactive to `page.url.searchParams.get('tab')` via `$effect`, instead of only reading it once at initialization.** This is the only way a same-page link click (checklist and profile page are now the same route) can change what's visible: SvelteKit does not remount `+page.svelte` for a navigation that only changes the query string on the same route, so the one-time `$state` initializer never re-runs.

The read of `view` inside the effect's own condition must go through `untrack()`. Svelte 5 effects track every reactive value they read, not only the one meant as the trigger — an untracked read of `view` would make the effect re-run on every ordinary tab click too (since it just wrote `view`), and it would then re-read the same, unchanged `page.url` and revert the click back to whatever tab the URL still names. `web/src/lib/urlSynced.svelte.ts`'s `syncOnNavigation` already carries this exact shape of fix, for the identical reason (see its doc comment). With `untrack`, the effect's only real dependency is `page.url`, so it fires on initial load, the checklist's own links, and browser back/forward — not on tab clicks or arrow-key moves, which never touch the URL.

## Risks / Trade-offs

- [The `tab` values in `accountCompleteness.ts` and the `VIEWS` ids in `/my/profile/+page.svelte` are two hand-kept string literals with no type-level link between them] → A typo silently lands on the default tab instead of failing a build; mitigated by keeping exactly three call sites, verifying manually per proposal, and the existing `accountCompleteness.test.ts` assertion that every step's `href` is non-empty (extended, if useful, to also check `tab` is one of the known ids as a lightweight guard — final call left to the task).
- [Making `view` reactive to the URL could, in principle, clash with a future feature that writes `?tab=` for a different reason on this page] → None exists today; if one is added later it must go through the same query param and will naturally stay in sync since both would drive the same effect.
