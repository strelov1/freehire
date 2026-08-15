## Context

See proposal.md - Why. Three "Tailor CV" entry points exist today, each navigating directly:

- Extension side panel: `extension/entrypoints/sidepanel/MatchCard.svelte`'s "Tailor my CV"
  button, currently `<Button href={tailorUrl} target="_blank">`. The card already holds a
  `JobMatch` (matched/missing/coverage) fetched by its parent — `blockers` is already on the
  wire (`GET /jobs/:slug/match`) but not yet in the extension's hand-written `JobMatch`
  TypeScript interface.
- Web application drawer: `web/src/lib/components/JobDrawer.svelte`'s `startTailoring()`,
  currently `goto(resolve('/tailor/[slug]', ...))` directly. This component has no match data
  loaded (a sibling `JobMatch.svelte` fetches its own, scoped to its own tab).
- Web job-page sidebar: `web/src/lib/components/MatchSummary.svelte`'s primary CTA, currently a
  plain `href` link (guest viewers are routed to sign-in instead, untouched by this change).
  Also match-data-less — it only receives `slug` and fetches the LLM analysis, not the
  deterministic match.

Both web call sites lack match/blocker data locally; the extension card already has it. The web
app already has the exact shared building blocks this needs: `JobMatchResult` (`matched`,
`missing`, `blockers`) in `web/src/lib/types.ts`, `api.getJobMatch(slug)`, and
`partitionBlockers()` in `web/src/lib/jobMatch.ts`. `toneText()` (severity → Tailwind class) and
the skill-chip class constants currently live as private consts inside
`web/src/lib/components/JobMatch.svelte` — this change is the second consumer, so they move to
`jobMatch.ts` as shared exports rather than being duplicated a second time.

The design system's `ConfirmDialog` (`design-system/src/confirm-dialog.svelte`) already supports
a `children` Snippet for rich body content alongside its plain `title`/`description` strings,
and is already used from both `web/` and (once wired) `extension/` via the shared
`freehire-design-system` package.

## Goals / Non-Goals

**Goals:**
- One confirmation UX, phrased and structured the same way, across extension and web.
- Zero new backend surface — read only what `GET /jobs/:slug/match` already returns.
- Extension: zero added network calls (reuse the card's already-loaded match).
- Web: one call site pattern (a singleton "ask" dialog, mirroring the existing
  `cvRefreshDialog.svelte.ts` / `CvRefreshDialog.svelte` pair) usable from both web entry
  points without threading match data through component props.

**Non-Goals:**
- Changing what `/tailor/[slug]` itself does once reached (its own "Tailor it for me" / "Walk
  me through it" choice is untouched).
- Gating the "View full analysis" revisit links — those stay direct navigation.
- Any backend/API change — `blockers` is already served.
- A shared component/type between the extension and web implementations — the two apps have no
  shared source import path (only the compiled `freehire-design-system` package), so each gets
  its own small, parallel implementation, consistent with the extension's existing precedent of
  porting rather than importing small pure helpers from web (`extension/lib/labels.ts`).

## Decisions

**Extension: dialog stays local to `MatchCard.svelte`, driven by the existing `match` prop.**
No new fetch, no state lifted to `App.svelte`. `ConfirmDialog`'s underlying `Dialog` uses native
`<dialog>`/`showModal()`, which promotes to the browser's top layer regardless of the
`.match-scroll` container's `overflow-y: auto` — confirmed by reading `dialog.svelte` and the
sidepanel's mount (`main.ts` → plain `mount(App, {target: document.getElementById('app')})`, no
shadow DOM/iframe boundary to cross). Alternative considered: lifting confirm state to
`App.svelte` — rejected, since it would only be needed if the confirmation had to survive a
`MatchCard` unmount (tab switch), which it does not (a cancelled or abandoned confirmation losing
its state on tab-away is fine).

**Extension: `partitionBlockers` duplicated locally rather than imported from web.** The
extension is a separate npm-managed package with no import path into `web/src`; small pure
helpers are ported rather than shared today, same precedent as `extension/lib/labels.ts`'s
existing "a port, not a shared import" comment.

**Web: a new singleton dialog controller (`askConfirmTailor`), not props threaded through
JobDrawer/MatchSummary.** Both call sites currently hold only a slug (or a slug plus item), not
a fetched `JobMatchResult`. Options considered:
1. *(chosen)* Singleton controller self-fetches `api.getJobMatch(slug)` when opened, mirroring
   `askCvRefresh`'s exact state/resolver/settle shape. Each call site becomes one `await
   askConfirmTailor(slug, optionalLabel)`. Cost: one extra (cheap, deterministic) fetch per
   click; acceptable since tailoring itself is already about to mint/fetch a CV server-side.
2. Lift `JobMatch.svelte`'s already-fetched `match` up through props to `MatchSummary`, and have
   `JobDrawer` fetch match itself for its own button. Rejected: couples an unrelated component's
   internal state to a new modal's needs, and still leaves `JobDrawer` needing its own fetch
   anyway — no consistency gained for the added coupling.

**Web: `toneText` and skill-chip classes move from `JobMatch.svelte` into `jobMatch.ts`.** They
become genuinely shared (a second component now needs identical severity→tone and chip styling)
rather than duplicated a second time; `JobMatch.svelte`'s own rendering is unchanged, only its
import source for those two pieces moves.

**Modal always shows, content adapts.** Per product decision (confirmed with the user): no
"skip the modal when everything matches" shortcut — the check should be a habit, not a surprise
that only appears when something is wrong.

## Risks / Trade-offs

- [Extra network round-trip before every web tailoring click] → The fetch is against an
  existing, cheap, deterministic, already-authenticated endpoint the app calls elsewhere on
  every job-page view; the click already leads to a page that performs its own (heavier)
  server round-trip immediately after, so the added latency is marginal.
- [`getJobMatch` can fail (e.g. no profile yet) at the moment the dialog opens] → The dialog
  degrades to a plain "we couldn't check your fit — add a CV to see it next time" message and
  still lets the candidate proceed; it does not block tailoring on the check succeeding.
- [Two independent implementations (extension, web) can drift in copy/behavior over time] →
  Accepted per Non-Goals; the two apps have no shared source today for this class of UI logic.
